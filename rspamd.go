package main

import (
	"context"
	"fmt"
	rspamd "github.com/Shopify/go-rspamd/v3"
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
