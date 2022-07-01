package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

var (
	requests *prometheus.CounterVec
	actions  *prometheus.CounterVec
)

func (c *Configuration) initMetrics() {
	if !c.CollectMetrics {
		return
	}
	actions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "actions",
		Help: "no. of actions on mails",
	}, []string{"action"})
	prometheus.MustRegister(actions)
	requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_total",
		Help: "The total number of http requests",
	}, []string{"method", "uri", "status"})
	prometheus.MustRegister(requests)
}

func (c *Configuration) pushRequests(r *http.Request, status int) {
	if c.CollectMetrics && requests != nil {
		requests.With(prometheus.Labels{"method": r.Method, "uri": r.RequestURI, "status": fmt.Sprintf("%d", status)}).Inc()
	}
}

func (c *Configuration) pushAction(action string) {
	if c.CollectMetrics && actions != nil {
		actions.With(prometheus.Labels{"action": action}).Inc()
	}
}
