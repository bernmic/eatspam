package main

import (
	"github.com/emersion/go-imap"
	"testing"
)

func setupTestConfiguration() Configuration {
	c := Configuration{
		Strategy: strategyAverage,
		Actions: map[float64]string{
			4.0:  spamActionGreylist,
			6.0:  spamActionAddHeader,
			8.0:  spamActionRewriteSubject,
			10.0: spamActionReject,
		},
	}
	return c
}

func TestAverageAction(t *testing.T) {
	c := setupTestConfiguration()
	result := c.averageAction(0.0)
	if result != spamActionNoAction {
		t.Errorf("expected %s, got %s", spamActionNoAction, result)
	}
	result = c.averageAction(3.9)
	if result != spamActionNoAction {
		t.Errorf("expected %s, got %s", spamActionNoAction, result)
	}
	result = c.averageAction(5.0)
	if result != spamActionGreylist {
		t.Errorf("expected %s, got %s", spamActionGreylist, result)
	}
	result = c.averageAction(7.0)
	if result != spamActionAddHeader {
		t.Errorf("expected %s, got %s", spamActionAddHeader, result)
	}
	result = c.averageAction(9.0)
	if result != spamActionRewriteSubject {
		t.Errorf("expected %s, got %s", spamActionRewriteSubject, result)
	}
	result = c.averageAction(10.0)
	if result != spamActionReject {
		t.Errorf("expected %s, got %s", spamActionReject, result)
	}
}

func TestOverallResult(t *testing.T) {
	t.SkipNow()
	c := setupTestConfiguration()
	c.Strategy = strategyLowest
	c.Spamd = SpamdConfiguration{
		Use: true,
	}
	c.Rspamd = RspamdConfiguration{
		Use: true,
	}
	m := imap.Message{
		Envelope: &imap.Envelope{
			Subject: "internal test",
		},
	}
	spamdResult := checkSpamResult{
		score:  0.0,
		action: spamActionNoAction,
		err:    nil,
	}
	rspamdResult := checkSpamResult{
		score:  4.0,
		action: spamActionAddHeader,
		err:    nil,
	}

	spamdChan := make(chan checkSpamResult)
	rspamdChan := make(chan checkSpamResult)
	spamdChan <- spamdResult
	rspamdChan <- rspamdResult
	c.Strategy = strategyLowest
	r := c.overallResult(&m, spamdChan, rspamdChan)
	if r.score != 0.0 || r.action != spamActionNoAction {
		t.Errorf("expecting lowest result (%0.1f, %s), got (%0.1f, %s)", 0.0, spamActionNoAction, r.score, r.action)
	}
	c.Strategy = strategyHighest
	r = c.overallResult(&m, spamdChan, rspamdChan)
	if r.score != 4.0 || r.action != spamActionAddHeader {
		t.Errorf("expecting lowest result (%0.1f, %s), got (%0.1f, %s)", 4.0, spamActionAddHeader, r.score, r.action)
	}
	c.Strategy = strategySpamd
	r = c.overallResult(&m, spamdChan, rspamdChan)
	if r.score != 0.0 || r.action != spamActionNoAction {
		t.Errorf("expecting lowest result (%0.1f, %s), got (%0.1f, %s)", 0.0, spamActionNoAction, r.score, r.action)
	}
	c.Strategy = strategyRspamd
	r = c.overallResult(&m, spamdChan, rspamdChan)
	if r.score != 4.0 || r.action != spamActionAddHeader {
		t.Errorf("expecting lowest result (%0.1f, %s), got (%0.1f, %s)", 4.0, spamActionAddHeader, r.score, r.action)
	}
	c.Strategy = strategyAverage
	r = c.overallResult(&m, spamdChan, rspamdChan)
	if r.score != 2.0 || r.action != spamActionNoAction {
		t.Errorf("expecting lowest result (%0.1f, %s), got (%0.1f, %s)", 2.0, spamActionNoAction, r.score, r.action)
	}
}

func TestSort(t *testing.T) {
	ids := []uint32{10, 3, 5, 2, 20, 10, 1, 6, 4, 7, 8, 9, 19, 17, 18, 16, 15, 12, 13, 11, 14}
	expected := []uint32{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	result := reverseSort(ids)

	if len(result) != len(expected) {
		t.Errorf("length are not equal: %d vs %d", len(result), len(expected))
		return
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("unexprected result. exprected %v, got %v", expected, result)
			return
		}
	}

}
