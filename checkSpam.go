package main

import (
	"fmt"
	"log"
	"math"
)

type checkSpamResult struct {
	score float64
	err   error
}

func (conf *Configuration) spamChecker() error {
	for _, ic := range conf.ImapAccounts {
		err := ic.checkSpam(conf)
		if err != nil {
			log.Printf("error checking mail on %s: %v\n", ic.Host, err)
		}
	}
	return nil
}

func (ic *ImapConfiguration) checkSpam(conf *Configuration) error {
	err := ic.connect()
	if err != nil {
		log.Fatalf("imap login to %s failed: %v", ic.Host, err)
	}
	defer func() {
		err := ic.client.Logout()
		if err != nil {
			log.Printf("error logging out: %v", err)
		}
	}()

	pw, err := decrypt(ic.Password, conf.key)
	if err != nil {
		return fmt.Errorf("error decrypting password for %s: %v", ic.Host, err)
	}
	if err := ic.client.Login(ic.Username, pw); err != nil {
		return fmt.Errorf("error login to %s: %v", ic.Host, err)
	}

	ic.Ok = true
	log.Printf("checking mail on %s\n", ic.Host)
	/*
		mailboxList, err := ic.mailboxes()
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Mailboxes:")
		for m := range mailboxList {
			log.Println(m.Name)
		}
	*/
	mbox, err := ic.client.Select(ic.Inbox, true)
	if err != nil {
		return fmt.Errorf("error selecting INBOX %s: %v\n", ic.Inbox, err)
	}
	if mbox.Messages == 0 {
		return nil
	}
	ids, err := ic.searchUnread()
	if err != nil {
		return fmt.Errorf("error searching unread mails: %v", err)
	}
	ic.UnreadMails = len(ids)
	if len(ids) > 0 {
		log.Printf("%d unread messages: %v", len(ids), ids)
		spamIds := make([]uint32, 0)
		msgs, err := ic.messagesWithId(ids)
		if err != nil {
			return fmt.Errorf("error fetching unread messages: %v", err)
		}
		for _, msg := range msgs {
			s, err := body(msg)
			if err != nil {
				log.Printf("error getting mail body: %v\n", err)
				continue
			}
			spamdChan := make(chan checkSpamResult)
			rspamdChan := make(chan checkSpamResult)
			if conf.Spamd.Use {
				go conf.Spamd.spamdCheckIfSpam(s, spamdChan)
			} else {
				spamdChan <- checkSpamResult{score: math.MaxFloat64, err: nil}
			}
			if conf.Rspamd.Use {
				go conf.Rspamd.rspamdCheckIfSpam(s, rspamdChan)
			} else {
				rspamdChan <- checkSpamResult{score: math.MaxFloat64, err: nil}
			}
			spamdResult := <-spamdChan
			rspamdResult := <-rspamdChan
			var averageResult float64
			if spamdResult.err != nil {
				log.Printf("spamd error: %v", err)
			} else if conf.Spamd.Use {
				log.Printf("spamd score for '%s' is %0.1f\n", msg.Envelope.Subject, spamdResult.score)
				averageResult = spamdResult.score
			}
			if rspamdResult.err != nil {
				log.Printf("rspamd error: %v", err)
			} else if conf.Rspamd.Use {
				log.Printf("rspamd score for '%s' is %0.1f\n", msg.Envelope.Subject, rspamdResult.score)
				if conf.Spamd.Use {
					averageResult = (averageResult + rspamdResult.score) / 2
				} else {
					averageResult = rspamdResult.score
				}
			}
			if averageResult >= conf.SpamThreshold {
				log.Printf("Move it (%d) to spam folder %s\n", msg.SeqNum, ic.SpamFolder)
				spamIds = append(spamIds, msg.SeqNum)
			}
		}
		if len(spamIds) > 0 {
			err = ic.moveToSpam(spamIds...)
			if err != nil {
				log.Printf("error moving spams to spam folder: %v", err)
			}
		}
	}
	return nil
}
