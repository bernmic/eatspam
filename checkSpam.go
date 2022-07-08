package main

import (
	"fmt"
	"github.com/emersion/go-imap"
	log "github.com/sirupsen/logrus"
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
			log.Errorf("error checking mail on %s: %v", ic.Host, err)
		}
	}
	return nil
}

func (ic *ImapConfiguration) checkSpam(conf *Configuration) error {
	log.Infof("start checking mail for account %s on host %s", ic.Name, ic.Host)
	err := ic.connect()
	if err != nil {
		return fmt.Errorf("error: imap connect to %s for account %s failed: %v", ic.Host, ic.Name, err)
	}
	defer func() {
		err := ic.client.Logout()
		if err != nil {
			log.Errorf("error logging out: %v", err)
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
		return fmt.Errorf("error selecting INBOX %s for fetching: %v", ic.Inbox, err)
	}
	if mbox.Messages == 0 {
		return nil
	}
	ids, err := ic.searchMails()
	if err != nil {
		return fmt.Errorf("error searching mails to process: %v", err)
	}
	ic.UnreadMails = len(ids)
	ids = reverseSort(ids)
	for _, id := range ids {
		msg, s, err := ic.getMessage(id)
		spamdChan := make(chan checkSpamResult)
		rspamdChan := make(chan checkSpamResult)
		if conf.Spamd.Use {
			go conf.Spamd.spamdCheckIfSpam(s, conf.Actions, spamdChan)
		}
		if conf.Rspamd.Use {
			go conf.Rspamd.rspamdCheckIfSpam(s, rspamdChan)
		}
		result := conf.overallResult(msg, spamdChan, rspamdChan)
		if result.err == nil {
			conf.pushAction(result.action)
			err = ic.doAction(id, result, conf)
			if err != nil {
				continue
			}
			if ic.InboxBehaviour == behaviourEatspam &&
				result.action != spamActionReject &&
				result.action != spamActionAddHeader &&
				result.action != spamActionRewriteSubject {
				err = ic.markAsEatspamSeen(id)
				if err != nil {
					log.Errorf("error adding flag %s to mail in account %s: %v", eatspamSeenFlag, ic.Name, err)
				}
			}
		}
	}
	log.Infof("end checking mail for account %s on host %s", ic.Name, ic.Host)
	return nil
}

func (ic *ImapConfiguration) doAction(id uint32, result checkSpamResult, conf *Configuration) error {
	var err error
	switch result.action {
	case spamActionReject:
		log.Infof("action for message %d is %s. Move to spam folder", id, result.action)
		err = ic.moveToSpam(id)
		if err != nil {
			log.Errorf("error moving spam %d to spam folder: %v", id, err)
		}
	case spamActionAddHeader:
		log.Infof("action for message %d is %s", id, result.action)
		err = ic.markSpamInHeader(result.score, true, id)
		if err != nil {
			log.Errorf("error adding header to spam mail %d: %v", id, err)
		}
	case spamActionRewriteSubject:
		log.Infof("action for message %d is %s", id, result.action)
		err = ic.markSpamInSubject(conf.SpamPrefix, id)
		if err != nil {
			log.Errorf("error rewriting subject of spam mail %d: %v", id, err)
		}
	case spamActionGreylist, spamActionNoAction:
		log.Debugf("action for message %d is %s. Skip action", id, result.action)
	default:
		log.Warnf("unknown action %s", result.action)
	}
	return err
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
			log.Errorf("spamd error: %v", spamdResult.err)
		} else if conf.Spamd.Use {
			log.Debugf("spamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, spamdResult.score, spamdResult.action)
			averageResult = spamdResult
		}
		if rspamdResult.err != nil {
			averageResult.err = rspamdResult.err
			log.Errorf("rspamd error: %v", rspamdResult.err)
		} else if conf.Rspamd.Use {
			log.Debugf("rspamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, rspamdResult.score, rspamdResult.action)
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
		log.Debugf("spamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, spamdResult.score, spamdResult.action)
		return spamdResult
	case strategyRspamd:
		if !conf.Rspamd.Use {
			log.Fatal("stategy rspamd is set but rspamd is not configured for use")
		}
		log.Debugf("rspamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, rspamdResult.score, rspamdResult.action)
		return rspamdResult
	case strategyLowest:
		if conf.Spamd.Use && conf.Rspamd.Use {
			if spamdResult.score < rspamdResult.score {
				log.Debugf("spamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, spamdResult.score, spamdResult.action)
				return spamdResult
			}
			log.Debugf("rspamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, rspamdResult.score, rspamdResult.action)
			return rspamdResult
		} else if !conf.Spamd.Use && !conf.Rspamd.Use {
			return checkSpamResult{
				score:  0.0,
				action: spamActionNoAction,
				err:    fmt.Errorf("spamd and rspamd are noch configured for use"),
			}
		} else if conf.Spamd.Use {
			log.Debugf("spamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, spamdResult.score, spamdResult.action)
			return spamdResult
		}
		log.Debugf("rspamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, rspamdResult.score, rspamdResult.action)
		return rspamdResult
	case strategyHighest:
		if conf.Spamd.Use && conf.Rspamd.Use {
			if spamdResult.score > rspamdResult.score {
				log.Debugf("spamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, spamdResult.score, spamdResult.action)
				return spamdResult
			}
			log.Debugf("rspamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, rspamdResult.score, rspamdResult.action)
			return rspamdResult
		} else if !conf.Spamd.Use && !conf.Rspamd.Use {
			return checkSpamResult{
				score:  0.0,
				action: spamActionNoAction,
				err:    fmt.Errorf("spamd and rspamd are noch configured for use"),
			}
		} else if conf.Spamd.Use {
			log.Debugf("spamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, spamdResult.score, spamdResult.action)
			return spamdResult
		}
		log.Debugf("rspamd score for '%s'(%d) is %0.1f with action=%s", msg.Envelope.Subject, msg.SeqNum, rspamdResult.score, rspamdResult.action)
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

func reverseSort(ids []uint32) []uint32 {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] > ids[j]
	})
	return ids
}
