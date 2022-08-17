package main

import (
	"embed"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"html/template"
	"net/http"
	"strings"
)

const (
	templateDir = "templates"
	assetsDir   = "assets"
)

const (
	secretPhrase   = "secret"
	cookieLoggedIn = "loggedIn"
)

var (
	lastMessageText string
	lastMessageType string
)

//go:embed templates
var templates embed.FS

//go:embed assets
var assets embed.FS

func (conf *Configuration) startHttpListener() {
	http.HandleFunc("/", conf.handlerIndex)
	if conf.CollectMetrics {
		http.Handle("/metrics", promhttp.Handler())
	}
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%04d", conf.Http.Port), nil))
}

func (conf *Configuration) handlerIndex(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "" || r.RequestURI == "/" {
		http.Redirect(w, r, "/index.html", http.StatusMovedPermanently)
		conf.pushRequests(r, http.StatusMovedPermanently)
	} else if r.RequestURI == "/logout" {
		c := http.Cookie{
			Name:   cookieLoggedIn,
			Value:  "",
			MaxAge: 0,
		}
		http.SetCookie(w, &c)
		http.Redirect(w, r, "/login.html", http.StatusMovedPermanently)
		conf.pushRequests(r, http.StatusMovedPermanently)
	} else if r.RequestURI == "/login" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "ParseForm() err: %v", err)
			return
		}
		pw := r.FormValue("password")
		opw, err := decrypt(conf.Http.Password, conf.key)
		if err != nil || opw != pw {
			conf.renderUnauthorized(w, r)
			return
		}
		secret, err := encrypt(secretPhrase, conf.key)
		if err != nil {
			conf.renderServerError(w, r)
			return
		}
		c := http.Cookie{
			Name:   cookieLoggedIn,
			Value:  secret,
			MaxAge: 3600,
		}
		http.SetCookie(w, &c)
		http.Redirect(w, r, "/index.html", http.StatusMovedPermanently)
		conf.pushRequests(r, http.StatusMovedPermanently)

	} else if r.URL.Path == "/ham" {
		m := r.URL.Query().Get("m")
		log.Debugf("make %s to ham", m)
		qe := queue.byId(m)
		if qe != nil {
			err := conf.learnHam(qe)
			if err != nil {
				log.Errorf("error learning ham: %v", err)
			}
		}
		http.Redirect(w, r, "/mails.html", http.StatusFound)
	} else if r.URL.Path == "/spam" {
		m := r.URL.Query().Get("m")
		log.Debugf("make %s to spam", m)
		qe := queue.byId(m)
		if qe != nil {
			err := conf.learnSpam(qe)
			if err != nil {
				log.Errorf("error learning spam: %v", err)
			}
		}
		http.Redirect(w, r, "/mails.html", http.StatusFound)
	} else if f, err := templates.Open(templateDir + r.URL.Path); err == nil {
		f.Close()
		conf.handleTemplate(w, r)
	} else if f, err := assets.Open(assetsDir + r.URL.Path); err == nil {
		f.Close()
		conf.serveFile(w, r)
	} else {
		conf.renderNotFound(w, r)
	}
}

func (conf *Configuration) handleTemplate(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/index.html":
		if !conf.checkLoggedIn(w, r) {
			return
		}
		conf.renderIndex(w, r)
	case "/login.html":
		conf.renderLogin(w, r)
	case "/account.html":
		if !conf.checkLoggedIn(w, r) {
			return
		}
		conf.renderAccount(w, r)
	case "/mails.html":

	}
	accessLog(r, http.StatusOK, r.RequestURI)
	if !conf.checkLoggedIn(w, r) {
		return
	}
	conf.renderMails(w, r)
}

func (conf *Configuration) serveFile(w http.ResponseWriter, r *http.Request) {
	data, err := assets.ReadFile(assetsDir + r.RequestURI)
	if err != nil {
		accessLog(r, http.StatusInternalServerError, err.Error())
		conf.renderServerError(w, r)
		return
	}
	accessLog(r, 200, "")
	lc := strings.ToLower(r.RequestURI)
	switch {
	case strings.HasSuffix(lc, ".css"):
		w.Header().Add("Content-Type", "text/css")
	case strings.HasSuffix(lc, ".jpg"):
		w.Header().Add("Content-Type", "image/jpeg")
	case strings.HasSuffix(lc, ".jpeg"):
		w.Header().Add("Content-Type", "image/jpeg")
	case strings.HasSuffix(lc, ".png"):
		w.Header().Add("Content-Type", "image/png")
	case strings.HasSuffix(lc, ".gif"):
		w.Header().Add("Content-Type", "image/git")
	case strings.HasSuffix(lc, ".ico"):
		w.Header().Add("Content-Type", "image/x-icon")
	case strings.HasSuffix(lc, ".html"):
		w.Header().Add("Content-Type", "text/html")
	case strings.HasSuffix(lc, ".js"):
		w.Header().Add("Content-Type", "application/javascript")
	case strings.HasSuffix(lc, ".map"):
		w.Header().Add("Content-Type", "application/json")
	case strings.HasSuffix(lc, ".svg"):
		w.Header().Add("Content-Type", "image/svg+xml")
	}
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(data)
	if err != nil {
		accessLog(r, http.StatusInternalServerError, "write data")
		conf.pushRequests(r, http.StatusInternalServerError)
	} else {
		conf.pushRequests(r, http.StatusOK)
	}
}

type IndexData struct {
	Page          string
	MessageText   string
	MessageType   string
	Configuration *Configuration
}

func (conf *Configuration) renderIndex(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFS(templates, templateDir+r.URL.Path, templateDir+"/navbar.html")
	if err != nil {
		log.Errorf("Error parsing template %s: %v", r.URL.Path, err)
		conf.renderServerError(w, r)
		return
	}
	err = t.Execute(w, IndexData{Page: "index", Configuration: conf, MessageType: lastMessageType, MessageText: lastMessageText})
	if err != nil {
		log.Errorf("error executing template /index.html: %v", err)
		conf.renderServerError(w, r)
	} else {
		conf.pushRequests(r, http.StatusOK)
	}
	lastMessageType = ""
	lastMessageText = ""
}

func (conf *Configuration) renderLogin(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFS(templates, templateDir+r.URL.Path)
	if err != nil {
		log.Errorf("error parsing template %s: %v", r.URL.Path, err)
		conf.renderServerError(w, r)
		return
	}
	err = t.Execute(w, conf)
	if err != nil {
		log.Errorf("error executing template /login.html: %v", err)
		conf.renderServerError(w, r)
	} else {
		conf.pushRequests(r, http.StatusOK)
	}
}

type AccountData struct {
	Page         string
	Imap         *ImapConfiguration
	MailboxNames []string
}

func (conf *Configuration) renderAccount(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFS(templates, templateDir+r.URL.Path, templateDir+"/navbar.html")
	if err != nil {
		log.Errorf("error parsing template %s: %v", r.URL.Path, err)
		conf.renderServerError(w, r)
		return
	}
	a := r.URL.Query().Get("a")
	for _, ia := range conf.ImapAccounts {
		if ia.Name == a {
			err := ia.connect()
			if err != nil {
				log.Fatalf("imap login to %s failed: %v", ia.Host, err)
			}
			err = ia.login(conf.key)
			if err != nil {
				conf.renderServerError(w, r)
				return
			}
			defer ia.logout()
			m, err := ia.mailboxes()
			if err != nil {
				log.Errorf("error getting mailbox list for %s: %v", ia.Name, err)
				conf.renderServerError(w, r)
				return
			} else if len(m) > 0 {
				mbs := make([]string, 0)
				for mb := range m {
					mbs = append(mbs, mb.Name)
				}
				ad := AccountData{
					Page:         "account",
					Imap:         ia,
					MailboxNames: mbs,
				}
				err = t.Execute(w, &ad)
				if err != nil {
					log.Errorf("error executing account template: %v", err)
				}
				return
			}
		}
	}
	lastMessageText = fmt.Sprintf("IMAP account '%s' not found", a)
	lastMessageType = "danger"
	http.Redirect(w, r, "index.html", http.StatusFound)
	conf.pushRequests(r, http.StatusFound)
	//renderNotFound(w, r)
}

type MailsData struct {
	Page     string
	Elements []*QueueElement
}

func (conf *Configuration) renderMails(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFS(templates, templateDir+r.URL.Path, templateDir+"/navbar.html")
	if err != nil {
		log.Errorf("error parsing template %s: %v", r.URL.Path, err)
		conf.renderServerError(w, r)
		return
	}
	err = t.Execute(w, MailsData{
		Page:     "mails",
		Elements: queue.asList(),
	})
	if err != nil {
		log.Errorf("error executing mails template: %v", err)
	}
}

func (conf *Configuration) checkLoggedIn(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(cookieLoggedIn)
	if err == nil && cookie.Value != "" {
		osp, err := decrypt(cookie.Value, conf.key)
		if err == nil && strings.Compare(osp, secretPhrase) == 0 {
			return true
		}
	}
	c := http.Cookie{
		Name:   cookieLoggedIn,
		Value:  "",
		MaxAge: 0,
	}
	http.SetCookie(w, &c)
	http.Redirect(w, r, "/login.html", http.StatusFound)
	conf.pushRequests(r, http.StatusNotFound)
	return false
}

func (conf *Configuration) renderNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, err := fmt.Fprintf(w, "Could not find the page you requested: %s.", r.RequestURI)
	if err != nil {
		accessLog(r, http.StatusNotFound, "write not found")
	}
	conf.pushRequests(r, http.StatusNotFound)
}

func (conf *Configuration) renderUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusUnauthorized)
	_, err := fmt.Fprintf(w, "Unauthorized")
	if err != nil {
		accessLog(r, http.StatusUnauthorized, "unauthorized")
	}
	conf.pushRequests(r, http.StatusUnauthorized)
}

func (conf *Configuration) renderBadRequest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, err := fmt.Fprintf(w, "Bad Request.")
	if err != nil {
		accessLog(r, http.StatusBadRequest, "write bad request")
	}
	conf.pushRequests(r, http.StatusBadRequest)
}

func (conf *Configuration) renderServerError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	_, err := fmt.Fprintf(w, "Internal Server Error: %s.", r.RequestURI)
	if err != nil {
		accessLog(r, http.StatusInternalServerError, "write internal server error")
	}
	conf.pushRequests(r, http.StatusInternalServerError)
}

func accessLog(r *http.Request, httpCode int, payload string) {
	switch httpCode {
	case http.StatusInternalServerError, http.StatusBadRequest:
		log.Errorf("%s %s, %d, %s", r.Method, r.RequestURI, httpCode, payload)
	case http.StatusNotFound, http.StatusUnauthorized:
		log.Warnf("%s %s, %d, %s", r.Method, r.RequestURI, httpCode, payload)
	default:
		log.Infof("%s %s, %d, %s", r.Method, r.RequestURI, httpCode, payload)
	}
}
