package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
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

func (ic *ImapConfiguration) messagesWithId(ids []uint32, unread bool) ([]*imap.Message, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)
	msgs, err := ic.fetchMessages(seqset)
	if err != nil {
		return nil, err
	}
	if unread {
		err = ic.setMessagesUnread(seqset)
	}
	return msgs, err
}

func body(m *imap.Message) (string, error) {
	result := ""
	var section imap.BodySectionName
	r := m.GetBody(&section)
	if r == nil {
		return result, fmt.Errorf("no body in mail")
	}
	b, err := ioutil.ReadAll(r)
	return string(b), err
}

func (ic *ImapConfiguration) searchUnread() ([]uint32, error) {
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}

	return ic.client.Search(criteria)
}

func (ic *ImapConfiguration) fetchMessages(seqset *imap.SeqSet) ([]*imap.Message, error) {
	log.Debugf("fetching messages %v", seqset)
	msgs := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	bs := &imap.BodySectionName{}
	go func() {
		done <- ic.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, bs.FetchItem()}, msgs)
	}()

	result := make([]*imap.Message, 0)
	for msg := range msgs {
		log.Debug("* " + msg.Envelope.Subject)
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
	err = ic.client.Append(ic.Inbox, flags, dt, b)
	if err != nil {
		return fmt.Errorf("error writing mail copy to server: %v", err)
	}
	return nil
}

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
	if strings.Contains(s, "X-Spam-Flag:") {
		// has already the header. stopping here
		return nil
	}
	err = ic.deleteMessages(id)
	if err != nil {
		return fmt.Errorf("error deleting message: %v", err)
	}
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("X-Spam-Flag: %t\r\n", isSpam))
	b.WriteString(fmt.Sprintf("X-Spam-Score: %0.3f\r\n", spamScore))
	b.WriteString(fmt.Sprintf("X-Spam-Level: %s\r\n", strings.Repeat("*", int(spamScore))))
	b.WriteString(s)
	flags := []string{}
	err = ic.client.Append(ic.Inbox, flags, dt, &b)
	if err != nil {
		return fmt.Errorf("error writing mail copy to server: %v", err)
	}
	return nil
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
