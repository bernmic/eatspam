package main

import (
	"fmt"
	"github.com/emersion/go-imap"
	"log"
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
	log.Printf("start checking mail on %s\n", ic.Host)
	err := ic.connect()
	if err != nil {
		return fmt.Errorf("error: imap connect to %s for account %s failed: %v", ic.Host, ic.Name, err)
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
	mbox, err := ic.client.Select(ic.Inbox, false)
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
		actions := make(map[uint32]checkSpamResult, 0)
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
			}
			if conf.Rspamd.Use {
				go conf.Rspamd.rspamdCheckIfSpam(s, rspamdChan)
			}
			result := conf.overallResult(msg, spamdChan, rspamdChan)
			if result.err == nil && result.action != spamActionNoAction {
				log.Printf("action for message %d is %s\n", msg.SeqNum, result.action)
				actions[msg.SeqNum] = result
			}
		}
		if len(actions) > 0 {
			actionIds := make([]uint32, 0)
			for k, _ := range actions {
				actionIds = append(actionIds, k)
			}
			for i := len(actionIds) - 1; i >= 0; i-- {
				cr := actions[actionIds[i]]
				switch cr.action {
				case spamActionReject:
					err = ic.moveToSpam(actionIds[i])
					if err != nil {
						log.Printf("error moving spams to spam folder: %v", err)
					}
				case spamActionAddHeader:
					err = ic.markSpamInHeader(cr.score, true, actionIds[i])
					if err != nil {
						log.Printf("error adding header to spam mails: %v", err)
					}
				case spamActionRewriteSubject:
					err = ic.markSpamInSubject(conf.SpamPrefix, actionIds[i])
					if err != nil {
						log.Printf("error rewriting subject of spam mails: %v", err)
					}
				}
			}
		}
	}
	log.Printf("end checking mail on %s\n", ic.Host)
	return nil
}

func (conf *Configuration) overallResult(msg *imap.Message, spamdChan chan checkSpamResult, rspamdChan chan checkSpamResult) checkSpamResult {
	var spamdResult, rspamdResult checkSpamResult
	if conf.Spamd.Use {
		spamdResult = <-spamdChan
	}
	if conf.Rspamd.Use {
		rspamdResult = <-rspamdChan
	}
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
		log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
		return spamdResult
	case strategyRspamd:
		if !conf.Rspamd.Use {
			log.Fatal("stategy rspamd is set but rspamd is not configured for use")
		}
		log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
		return rspamdResult
	case strategyLowest:
		if conf.Spamd.Use && conf.Rspamd.Use {
			if spamdResult.score < rspamdResult.score {
				log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
				return spamdResult
			}
			log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
			return rspamdResult
		} else if !conf.Spamd.Use && !conf.Rspamd.Use {
			return checkSpamResult{
				score:  0.0,
				action: spamActionNoAction,
				err:    fmt.Errorf("spamd and rspamd are noch configured for use"),
			}
		} else if conf.Spamd.Use {
			log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
			return spamdResult
		}
		log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
		return rspamdResult
	case strategyHighest:
		if conf.Spamd.Use && conf.Rspamd.Use {
			if spamdResult.score > rspamdResult.score {
				log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
				return spamdResult
			}
			log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
			return rspamdResult
		} else if !conf.Spamd.Use && !conf.Rspamd.Use {
			return checkSpamResult{
				score:  0.0,
				action: spamActionNoAction,
				err:    fmt.Errorf("spamd and rspamd are noch configured for use"),
			}
		} else if conf.Spamd.Use {
			log.Printf("spamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, spamdResult.score, spamdResult.action)
			return spamdResult
		}
		log.Printf("rspamd score for '%s' is %0.1f with action=%s\n", msg.Envelope.Subject, rspamdResult.score, rspamdResult.action)
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
