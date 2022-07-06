package main

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const defaultHeaderTemplate = `X-Spam-Flag: {{.YesNo}}\r\nX-Spam-Score: {{.Score}}\r\nX-Spam-Level: {{.Level}}\r\nX-Spam-Bar: {{.Bar}}\r\nX-Spam-Status: {{.YesNoCap}}, score={{.Score}}\r\n`

var headerTemplate = defaultHeaderTemplate

type AddHeaderData struct {
	YesNo    string
	YesNoCap string
	Score    string
	Level    string
	Bar      string
}

func (c *Configuration) initAddHeaderTemplate() {
	headerTemplate = c.SpamHeader
}

func header(isSpam bool, score float64) ([]byte, error) {
	t, err := template.New("addHeader").Parse(headerTemplate)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer

	d := AddHeaderData{
		YesNo:    yesNo(isSpam),
		YesNoCap: yesNoCap(isSpam),
		Score:    fmt.Sprintf("%0.1f", score),
		Level:    strings.Repeat("*", int(score)),
		Bar:      strings.Repeat("+", int(score)),
	}

	err = t.Execute(&b, d)
	return b.Bytes(), err
}

func yesNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

func yesNoCap(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
