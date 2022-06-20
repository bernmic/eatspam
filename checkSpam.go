package main

import (
	"fmt"
	"github.com/emersion/go-imap"
	"log"
	"math"
	"sort"
)

type checkSpamResult struct {
	score  float64
	action string
	err    error
}

const (
	spamActionNoAction       = "no action"
	spamActionSoftReject     = "soft reject"
	spamActionReject         = "reject"
	spamActionRewriteSubject = "rewrite subject"
	spamActionAddHeader      = "add header"
	spamActionGreylist       = "greylist"
)

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
	log.Printf("start checking mail on %s\n", ic.Host)

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
				go conf.Spamd.spamdCheckIfSpam(s, conf.SpamThreshold, spamdChan)
			} else {
				spamdChan <- checkSpamResult{score: math.MaxFloat64, action: spamActionNoAction, err: nil}
			}
			if conf.Rspamd.Use {
				go conf.Rspamd.rspamdCheckIfSpam(s, rspamdChan)
			} else {
				rspamdChan <- checkSpamResult{score: math.MaxFloat64, action: spamActionNoAction, err: nil}
			}
			spamdResult := <-spamdChan
			rspamdResult := <-rspamdChan
			var averageResult float64
			if spamdResult.err != nil {
				log.Printf("spamd error: %v", err)
			} else if conf.Spamd.Use {
				log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
				averageResult = spamdResult.score
			}
			if rspamdResult.err != nil {
				log.Printf("rspamd error: %v", err)
			} else if conf.Rspamd.Use {
				log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
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
	log.Printf("end checking mail on %s\n", ic.Host)
	return nil
}

func (conf *Configuration) overallResult(msg *imap.Message, spamdResult checkSpamResult, rspamdResult checkSpamResult) checkSpamResult {
	switch conf.Strategy {
	case strategyAverage:
		averageResult := checkSpamResult{
			score:  0.0,
			action: spamActionNoAction,
			err:    nil,
		}
		if spamdResult.err != nil {
			averageResult.err = spamdResult.err
			log.Printf("spamd error: %v", spamdResult.err)
		} else if conf.Spamd.Use {
			log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
			averageResult = spamdResult
		}
		if rspamdResult.err != nil {
			averageResult.err = rspamdResult.err
			log.Printf("rspamd error: %v", rspamdResult.err)
		} else if conf.Rspamd.Use {
			log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
			if conf.Spamd.Use {
				averageResult.score = (averageResult.score + rspamdResult.score) / 2
			} else {
				averageResult = rspamdResult
			}
		}
		return checkSpamResult{
			score:  averageResult.score,
			action: conf.averageAction(averageResult.score),
		}
	case strategySpamd:
		if !conf.Spamd.Use {
			log.Fatal("stategy spamd is set but spamd is not configured for use")
		}
		return spamdResult
	case strategyRspamd:
		if !conf.Rspamd.Use {
			log.Fatal("stategy rspamd is set but rspamd is not configured for use")
		}
		return spamdResult
	case strategyLowest:
		if conf.Spamd.Use && conf.Rspamd.Use {
			if spamdResult.score < rspamdResult.score {
				return spamdResult
			}
			return rspamdResult
		} else if !conf.Spamd.Use && !conf.Rspamd.Use {
			return checkSpamResult{
				score:  0.0,
				action: spamActionNoAction,
				err:    fmt.Errorf("spamd and rspamd are noch configured for use"),
			}
		} else if conf.Spamd.Use {
			return spamdResult
		}
		return rspamdResult
	case strategyHighest:
		if conf.Spamd.Use && conf.Rspamd.Use {
			if spamdResult.score > rspamdResult.score {
				return spamdResult
			}
			return rspamdResult
		} else if !conf.Spamd.Use && !conf.Rspamd.Use {
			return checkSpamResult{
				score:  0.0,
				action: spamActionNoAction,
				err:    fmt.Errorf("spamd and rspamd are noch configured for use"),
			}
		} else if conf.Spamd.Use {
			return spamdResult
		}
		return rspamdResult
	}
	return checkSpamResult{
		score:  0.0,
		action: spamActionNoAction,
		err:    fmt.Errorf("unknown strategy %s", conf.Strategy),
	}
}

func (conf *Configuration) averageAction(score float64) string {
	keys := make([]float64, 0)
	for k, _ := range conf.Actions {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	for i := len(keys) - 1; i >= 0; i-- {
		if score >= keys[i] {
			return conf.Actions[keys[i]]
		}
	}
	return spamActionNoAction
}
