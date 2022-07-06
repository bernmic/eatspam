package main

import (
	"fmt"
	"testing"
)

const header1 = `X-Spam-Flag: YES\r\nX-Spam-Score: 3.3\r\nX-Spam-Level: ***\r\nX-Spam-Bar: +++\r\nX-Spam-Status: Yes, score=3.3\r\n`

func TestTemplate(t *testing.T) {
	b, err := header(true, 3.333)
	if err != nil {
		t.Fatalf("error generating header: %v", err)
	}
	s := string(b)
	if s != header1 {
		t.Errorf("wrong result")
		fmt.Println(s)
		//fmt.Println(header1)
	}
}
