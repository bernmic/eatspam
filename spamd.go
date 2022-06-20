package main

import (
	"context"
	"fmt"
	"github.com/Teamwork/spamc"
	"net"
	"strings"
	"time"
)

func (conf *SpamdConfiguration) spamdCheckIfSpam(s string, threshold float64, result chan checkSpamResult) {
	c := spamc.New(fmt.Sprintf("%s:%d", conf.Host, conf.Port), &net.Dialer{
		Timeout: 20 * time.Second,
	})
	ctx := context.Background()

	msg := strings.NewReader(s)

	// Check if a message is spam.
	check, err := c.Check(ctx, msg, nil)
	if err != nil {
		result <- checkSpamResult{score: 0.0, action: spamActionNoAction, err: err}
	} else {
		a := spamActionNoAction
		if check.Score >= threshold {
			a = spamActionReject
		}
		result <- checkSpamResult{score: check.Score, action: a, err: nil}
	}

	// Report ham for training.
	/*	tell, err := c.Tell(ctx, msg, spamc.Header{}.
			Set("Message-class", "ham").
			Set("Set", "local"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(tell)*/
}
