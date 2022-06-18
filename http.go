package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

const (
	templateDir = "templates"
	assetsDir   = "assets"
)

//go:embed templates
var templates embed.FS

//go:embed assets
var assets embed.FS

func (conf *Configuration) startHttpListener() {
	http.HandleFunc("/", conf.handlerIndex)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%04d", conf.HttpPort), nil))
}

func (conf *Configuration) handlerIndex(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "" || r.RequestURI == "/" {
		http.Redirect(w, r, "/index.html", http.StatusMovedPermanently)
	} else if f, err := templates.Open(templateDir + r.URL.Path); err == nil {
		f.Close()
		conf.handleTemplate(w, r)
	} else if f, err := assets.Open(assetsDir + r.URL.Path); err == nil {
		f.Close()
		conf.serveFile(w, r)
	} else {
		renderNotFound(w, r)
		accessLog(r, http.StatusNotFound, "")
	}
}

func (conf *Configuration) handleTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFS(templates, templateDir+r.URL.Path)
	if err != nil {
		log.Printf("Error parsing template %s: %v\n", r.URL.Path, err)
		renderServerError(w, r)
		return
	}
	switch r.URL.Path {
	case "/index.html":
		err = t.Execute(w, conf)
		if err != nil {
			log.Printf("Error executing template /index.html: %v\n", err)
		}
	}
	accessLog(r, http.StatusOK, r.RequestURI)
}

func (conf *Configuration) serveFile(w http.ResponseWriter, r *http.Request) {
	data, err := assets.ReadFile(assetsDir + r.RequestURI)
	if err != nil {
		accessLog(r, http.StatusInternalServerError, err.Error())
		renderServerError(w, r)
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
	}
}

func renderNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, err := fmt.Fprintf(w, "Could not find the page you requested: %s.", r.RequestURI)
	if err != nil {
		accessLog(r, http.StatusInternalServerError, "write not found")
	}
}

func renderBadRequest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, err := fmt.Fprintf(w, "Bad Request.")
	if err != nil {
		accessLog(r, http.StatusInternalServerError, "write bad request")
	}
}

func renderServerError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	_, err := fmt.Fprintf(w, "Internal Server Error: %s.", r.RequestURI)
	if err != nil {
		accessLog(r, http.StatusInternalServerError, "write internal server error")
	}
}

func accessLog(r *http.Request, httpCode int, payload string) {
	log.Printf("%s %s, %d, %s", r.Method, r.RequestURI, httpCode, payload)
}
