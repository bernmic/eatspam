package main

import (
	"context"
	"fmt"
	"github.com/Shopify/go-rspamd"
	"strings"
)

func (conf *RspamdConfiguration) rspamdCheckIfSpam(s string, result chan checkSpamResult) {
	ctx := context.Background()
	c := rspamd.New(conf.url())
	mail := rspamd.NewEmailFromReader(strings.NewReader(s))
	cr, err := c.Check(ctx, mail)
	if err != nil {
		result <- checkSpamResult{score: 0.0, err: err}
	}
	result <- checkSpamResult{score: cr.Score, err: nil}
}

func (conf *RspamdConfiguration) url() string {
	return fmt.Sprintf("http://%s:%d", conf.Host, conf.Port)
}
