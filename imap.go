package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"
)

func (ic *ImapConfiguration) connect() error {
	s := fmt.Sprintf("%s:%d", ic.Host, ic.Port)
	tlsConfig := tls.Config{InsecureSkipVerify: true}
	c, err := client.DialTLS(s, &tlsConfig)
	if err != nil {
		return err
	}
	ic.client = c
	return nil
}

func (ic *ImapConfiguration) mailboxes() (chan *imap.MailboxInfo, error) {
	mailboxList := make(chan *imap.MailboxInfo, 100)
	done := make(chan error, 1)
	go func() {
		done <- ic.client.List("", "*", mailboxList)
	}()

	if err := <-done; err != nil {
		return nil, fmt.Errorf("error getting mailboxlist: %v", err)
	}
	return mailboxList, nil
}

func (ic *ImapConfiguration) login(key string) error {
	pw, err := decrypt(ic.Password, key)
	if err != nil {
		return fmt.Errorf("error decrypting password for %s: %v", ic.Host, err)
	}
	if err := ic.client.Login(ic.Username, pw); err != nil {
		if err == client.ErrAlreadyLoggedIn {
			log.Warnf("warning: already logged in")
			return nil
		}
		return fmt.Errorf("error login to %s: %v", ic.Host, err)
	}
	return nil
}

func (ic *ImapConfiguration) logout() {
	err := ic.client.Logout()
	if err != nil {
		log.Errorf("error logging out: %v", err)
	}
}

func (ic *ImapConfiguration) lastNMessages(mbox *imap.MailboxStatus, n uint32) ([]*imap.Message, error) {
	// Get the last n messages
	from := uint32(1)
	to := mbox.Messages
	if mbox.Messages > n-1 {
		// We're using unsigned integers here, only subtract if the result is > 0
		from = mbox.Messages - (n - 1)
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	return ic.fetchMessages(seqset)
}

func body(m *imap.Message) (string, error) {
	b := []byte{}
	for name, literal := range m.Body {
		log.Tracef("%v = %v", name, literal)
		bb, err := ioutil.ReadAll(literal)
		if err != nil {
			log.Errorf("error reading body literal %v: %v", name, err)
		}
		b = append(b, bb...)
	}

	return string(b), nil
}

func (ic *ImapConfiguration) searchUnread() ([]uint32, error) {
	return ic.searchFlag(imap.SeenFlag)
}

func (ic *ImapConfiguration) searchEatspamUnread() ([]uint32, error) {
	return ic.searchFlag(eatspamSeenFlag)
}

func (ic *ImapConfiguration) searchFlag(flag string) ([]uint32, error) {
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{flag}

	return ic.client.Search(criteria)
}

func (ic *ImapConfiguration) fetchMessage(seqset *imap.SeqSet) (*imap.Message, error) {
	log.Debugf("fetching message %v", seqset)
	ch := make(chan *imap.Message, 1)
	err := ic.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchItem("BODY.PEEK[]")}, ch)
	if err != nil {
		return nil, fmt.Errorf("error fetching message %v: %v", seqset, err)
	}
	return <-ch, nil
}

func (ic *ImapConfiguration) fetchMessages(seqset *imap.SeqSet) ([]*imap.Message, error) {
	log.Debugf("fetching messages %v", seqset)
	msgs := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- ic.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchItem("BODY.PEEK[]")}, msgs)
	}()

	result := make([]*imap.Message, 0)
	for msg := range msgs {
		if msg != nil && msg.Envelope != nil {
			log.Debug("* " + msg.Envelope.Subject)
		}
		result = append(result, msg)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("error fetching messages: %v", err)
	}

	log.Debug("Done!")
	return result, nil
}

func (ic *ImapConfiguration) setMessagesUnread(seqset *imap.SeqSet) error {
	log.Debugf("set messages %v to unread", seqset)
	item := imap.FormatFlagsOp(imap.RemoveFlags, true)
	flags := []interface{}{imap.SeenFlag}
	return ic.client.Store(seqset, item, flags, nil)
}

func (ic *ImapConfiguration) moveToSpam(id ...uint32) error {
	sort.Slice(id, func(i, j int) bool {
		return id[i] > id[j]
	})
	seqset := new(imap.SeqSet)
	for _, i := range id {
		seqset.AddNum(i)
	}
	return ic.client.Move(seqset, ic.SpamFolder)
}

func (ic *ImapConfiguration) deleteMessages(id ...uint32) error {
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := ic.client.Store(reverseSeqSet(id...), item, flags, nil); err != nil {
		return fmt.Errorf("error deleting mails: %v", err)
	}

	// Then delete it
	if err := ic.client.Expunge(nil); err != nil {
		return fmt.Errorf("error expunging mails: %v", err)
	}
	return nil
}

func (ic *ImapConfiguration) markSpamInSubject(spamPrefix string, id uint32) error {
	msgs, err := ic.fetchMessages(reverseSeqSet(id))
	if err != nil {
		return fmt.Errorf("error fetching mail: %v", err)
	}
	if len(msgs) != 1 {
		return fmt.Errorf("expect 1 mail got %d", len(msgs))
	}
	dt := msgs[0].Envelope.Date
	s, err := body(msgs[0])
	if err != nil {
		return fmt.Errorf("error getting mails body: %v", err)
	}
	if strings.Contains(s, fmt.Sprintf("Subject: %s ", spamPrefix)) {
		// has already the prefix. stopping here
		return nil
	}
	err = ic.deleteMessages(id)
	if err != nil {
		return fmt.Errorf("error deleting message: %v", err)
	}

	if strings.Contains(s, "Subject: ") {
		s = strings.Replace(s, "Subject: ", fmt.Sprintf("Subject: %s ", spamPrefix), 1)
	} else {
		if strings.Contains(s, "Subject:\n") {
			s = strings.Replace(s, "Subject:\n", fmt.Sprintf("Subject: %s\n", spamPrefix), 1)
		}
	}

	b := bytes.NewBufferString(s)
	flags := []string{}
	if ic.InboxBehaviour == behaviourEatspam {
		flags = append(flags, eatspamSeenFlag)
	}
	err = ic.client.Append(ic.Inbox, flags, dt, b)
	if err != nil {
		return fmt.Errorf("error writing mail copy to server: %v", err)
	}
	return nil
}

var regexpSpamHeader = regexp.MustCompile("(?m)^X-Spam-Flag: [NY][OE][S]*$")

func (ic *ImapConfiguration) markSpamInHeader(spamScore float64, isSpam bool, id uint32) error {
	msgs, err := ic.fetchMessages(reverseSeqSet(id))
	if err != nil {
		return fmt.Errorf("error fetching mail: %v", err)
	}
	if len(msgs) != 1 {
		return fmt.Errorf("exprect 1 mail got %d", len(msgs))
	}
	dt := msgs[0].Envelope.Date
	s, err := body(msgs[0])
	if err != nil {
		return fmt.Errorf("error getting mails body: %v", err)
	}
	if regexpSpamHeader.MatchString(s) {
		// Remove previous spam-flag
		regexpSpamHeader.ReplaceAllString(s, "")
	}
	err = ic.deleteMessages(id)
	if err != nil {
		return fmt.Errorf("error deleting message: %v", err)
	}
	var b bytes.Buffer
	hd, err := header(isSpam, spamScore)
	if err != nil {
		return fmt.Errorf("error creating header data: %v", err)
	}
	b.Write(hd)
	/*
		b.WriteString(fmt.Sprintf("X-Spam-Flag: %s\r\n", yesNo(isSpam)))
		b.WriteString(fmt.Sprintf("X-Spam-Score: %0.3f\r\n", spamScore))
		b.WriteString(fmt.Sprintf("X-Spam-Level: %s\r\n", strings.Repeat("*", int(spamScore))))
		b.WriteString(fmt.Sprintf("X-Spam-Bar: %s\r\n", strings.Repeat("+", int(spamScore))))
		b.WriteString(fmt.Sprintf("X-Spam-Status: %s, score=%0.1f\r\n", yesNoCap(isSpam), spamScore))
	*/
	b.WriteString(s)
	flags := []string{}
	if ic.InboxBehaviour == behaviourEatspam {
		flags = append(flags, eatspamSeenFlag)
	}
	err = ic.client.Append(ic.Inbox, flags, dt, &b)
	if err != nil {
		return fmt.Errorf("error writing mail copy to server: %v", err)
	}
	return nil
}

const eatspamSeenFlag = "$EatspamSeen"

func (ic *ImapConfiguration) markAsEatspamSeen(id uint32) error {
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{eatspamSeenFlag}
	if err := ic.client.Store(reverseSeqSet(id), item, flags, nil); err != nil {
		return fmt.Errorf("error adding flag to mail: %v", err)
	}
	return nil
}

func (ic *ImapConfiguration) getMessage(id uint32) (*imap.Message, string, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(id)
	msg, err := ic.fetchMessage(seqset)
	if err != nil {
		log.Errorf("error fetching message %d from account %s: %v", id, ic.Name, err)
		return nil, "", err
	}
	s, err := body(msg)
	if err != nil {
		log.Errorf("error getting mail body: %v", err)
		return msg, "", err
	}
	return msg, s, nil
}

func (ic *ImapConfiguration) searchMails() ([]uint32, error) {
	switch ic.InboxBehaviour {
	case behaviourUnseen:
		return ic.searchUnread()
	case behaviourEatspam:
		return ic.searchEatspamUnread()
	case behaviourAll:
		return nil, fmt.Errorf("inboxBehaviour 'all' is not implemented yes")
	default:
		return nil, fmt.Errorf("inboxBehaviour '%s' is not known", ic.InboxBehaviour)
	}
}

func reverseSeqSet(id ...uint32) *imap.SeqSet {
	sort.Slice(id, func(i, j int) bool {
		return id[i] > id[j]
	})
	seqset := new(imap.SeqSet)
	for _, i := range id {
		seqset.AddNum(i)
	}
	return seqset
}
