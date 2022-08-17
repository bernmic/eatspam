package main

import (
	"context"
	"fmt"
	rspamd "github.com/Shopify/go-rspamd/v3"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

func (conf *RspamdConfiguration) rspamdCheckIfSpam(s string, result chan checkSpamResult) {
	ctx := context.Background()
	c := rspamd.New(conf.url())
	req := &rspamd.CheckRequest{
		Message: strings.NewReader(s),
		Header:  http.Header{},
	}
	cr, err := c.Check(ctx, req)
	if err != nil {
		result <- checkSpamResult{score: 0.0, action: "", err: err}
	} else {
		result <- checkSpamResult{score: cr.Score, action: cr.Action, err: nil}
	}
}

func (conf *RspamdConfiguration) url() string {
	return fmt.Sprintf("http://%s:%d", conf.Host, conf.Port)
}

func (conf *RspamdConfiguration) learnHam(body string) error {
	ctx := context.Background()
	c := rspamd.New(conf.url())
	lr := rspamd.LearnRequest{
		Message: strings.NewReader(body),
		Header:  http.Header{},
	}
	l, err := c.LearnHam(ctx, &lr)
	if l.Success {
		log.Info("successfully learned ham")
	}
	return err
}

func (conf *RspamdConfiguration) learnSpam(body string) error {
	ctx := context.Background()
	c := rspamd.New(conf.url())
	lr := rspamd.LearnRequest{
		Message: strings.NewReader(body),
		Header:  http.Header{},
	}
	l, err := c.LearnSpam(ctx, &lr)
	if l.Success {
		log.Info("successfully learned spam")
	}
	return err
}
