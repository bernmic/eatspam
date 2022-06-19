package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

func setup() (*Configuration, error) {
	c := Configuration{}
	configdata, err := ioutil.ReadFile(defaultConfigFile)
	if err == nil {
		err = yaml.Unmarshal([]byte(configdata), &c)
		if err != nil {
			return nil, fmt.Errorf("error unmarshal config file: %v", err)
		}
	} else {
		log.Fatalf("error reading config file: %v", err)
	}
	if len(c.ImapAccounts) == 0 {
		return nil, fmt.Errorf("No imap accounts configured. Stopping here.")
	}
	b, err := os.ReadFile(c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("error reading key file: %v", err)
	}
	c.key = string(b)
	c.SpamPrefix = "*** SPAM ***"
	c.SpamThreshold = 5.0
	return &c, nil
}

func connectLastAccount(c *Configuration) (*ImapConfiguration, error) {
	ic := c.ImapAccounts[len(c.ImapAccounts)-1]
	err := ic.connect()
	if err != nil {
		return nil, fmt.Errorf("imap login to %s failed: %v", ic.Host, err)
	}

	pw, err := decrypt(ic.Password, c.key)
	if err != nil {
		return nil, fmt.Errorf("error decrypting password: %v", err)
	}
	if err := ic.client.Login(ic.Username, pw); err != nil {
		return nil, fmt.Errorf("error logging into %s: %v", ic.Host, err)
	}

	ic.Ok = true
	return ic, nil
}

// move last message from last imap account to spam
func TestMove(t *testing.T) {
	t.SkipNow()
	c, err := setup()
	if err != nil {
		t.Fatalf("error setting up: %v", err)
	}

	ic, err := connectLastAccount(c)
	if err != nil {
		t.Fatalf("error connecting to imap:%v", err)
	}
	defer func() {
		err := ic.client.Logout()
		if err != nil {
			log.Fatalf("error logging out: %v", err)
		}
	}()

	mbox, err := ic.client.Select(ic.Inbox, false)
	if err != nil {
		t.Errorf("error selecting INBOX %s: %v\n", ic.Inbox, err)
	}
	if mbox.Messages > 0 {
		// move last message to spam folder
		err = ic.moveToSpam(mbox.Messages)
		if err != nil {
			t.Errorf("error moving mail to spam folder '%s': %v", ic.SpamFolder, err)
			mailboxList, err := ic.mailboxes()
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Mailboxes:")
			for m := range mailboxList {
				log.Println(m.Name)
			}
		}
	}
}

// delete last message from last imap account to spam
func TestDelete(t *testing.T) {
	t.SkipNow()
	c, err := setup()
	if err != nil {
		t.Fatalf("error setting up: %v", err)
	}

	ic, err := connectLastAccount(c)
	if err != nil {
		t.Fatalf("error connecting to imap:%v", err)
	}
	defer func() {
		err := ic.client.Logout()
		if err != nil {
			log.Fatalf("error logging out: %v", err)
		}
	}()

	mbox, err := ic.client.Select(ic.Inbox, false)
	if err != nil {
		t.Errorf("error selecting INBOX %s: %v\n", ic.Inbox, err)
	}
	if mbox.Messages > 0 {
		// move last message to spam folder
		err = ic.deleteMessages(mbox.Messages)
		if err != nil {
			t.Errorf("error deleting mail: %v", err)
		}
	}
}

func TestMarkSubject(t *testing.T) {
	c, err := setup()
	if err != nil {
		t.Fatalf("error setting up: %v", err)
	}

	ic, err := connectLastAccount(c)
	if err != nil {
		t.Fatalf("error connecting to imap:%v", err)
	}
	defer func() {
		err := ic.client.Logout()
		if err != nil {
			log.Fatalf("error logging out: %v", err)
		}
	}()

	mbox, err := ic.client.Select(ic.Inbox, false)
	if err != nil {
		t.Errorf("error selecting INBOX %s: %v\n", ic.Inbox, err)
	}
	if mbox.Messages == 0 {
		t.Error("no messages in mbox")
	}
	err = ic.markSpamInSubject(c.SpamPrefix, mbox.Messages)
	if err != nil {
		t.Errorf("error mark message: %v", err)
	}
}

func TestMarkHeader(t *testing.T) {
	c, err := setup()
	if err != nil {
		t.Fatalf("error setting up: %v", err)
	}

	ic, err := connectLastAccount(c)
	if err != nil {
		t.Fatalf("error connecting to imap:%v", err)
	}
	defer func() {
		err := ic.client.Logout()
		if err != nil {
			log.Fatalf("error logging out: %v", err)
		}
	}()

	mbox, err := ic.client.Select(ic.Inbox, false)
	if err != nil {
		t.Errorf("error selecting INBOX %s: %v\n", ic.Inbox, err)
	}
	if mbox.Messages == 0 {
		t.Error("no messages in mbox")
	}
	err = ic.markSpamInHeader(6.0, true, mbox.Messages)
	if err != nil {
		t.Errorf("error mark message: %v", err)
	}
}
