package main

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"github.com/emersion/go-imap"
	"sync"
	"time"
)

type QueueElement struct {
	Id      string
	Score   string
	Action  string
	Class   string
	Account *ImapConfiguration
	Sender  string
	Subject string
	Body    string
	Date    string
}

type Queue struct {
	messages *list.List
	mu       sync.Mutex
}

const capacity = 50

var actionClassMap = map[string]string{
	spamActionNoAction:       "success",
	spamActionSoftReject:     "secondary",
	spamActionReject:         "danger",
	spamActionRewriteSubject: "warning",
	spamActionAddHeader:      "warning",
	spamActionGreylist:       "dark",
}

func NewQueue() *Queue {
	q := Queue{
		messages: list.New(),
	}
	return &q
}

func (q *Queue) queueMessage(ic *ImapConfiguration, c checkSpamResult, msg *imap.Message, body string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	sender := ""
	if msg != nil && msg.Envelope != nil || len(msg.Envelope.Sender) > 0 {
		sender = msg.Envelope.Sender[0].Address()
	}
	qe := QueueElement{
		Id:      fmt.Sprintf("%x", sha256.Sum256([]byte(body))),
		Score:   fmt.Sprintf("%0.1f", c.score),
		Action:  c.action,
		Class:   actionClassMap[c.action],
		Account: ic,
		Sender:  sender,
		Subject: msg.Envelope.Subject,
		Body:    body,
		Date:    msg.Envelope.Date.Format(time.RFC822),
	}
	q.messages.PushBack(&qe)
	if q.messages.Len() > capacity {
		e := q.messages.Front()
		q.messages.Remove(e)
	}
}

func (q *Queue) asList() []*QueueElement {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*QueueElement, 0)
	for e := q.messages.Front(); e != nil; e = e.Next() {
		result = append(result, e.Value.(*QueueElement))
	}
	return result
}

func (q *Queue) byId(id string) *QueueElement {
	for e := q.messages.Front(); e != nil; e = e.Next() {
		qe := e.Value.(*QueueElement)
		if qe.Id == id {
			return qe
		}
	}
	return nil
}
