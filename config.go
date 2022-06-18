package main

import (
	"flag"
	"fmt"
	"github.com/emersion/go-imap/client"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"os"
	"strconv"
)

const (
	defaultConfigFile     = "eatspam.yaml"
	defaultSpamdPort      = 783
	defaultSpamdUse       = true
	defaultSpamdHost      = "127.0.0.1"
	defaultRspamdPort     = 11333
	defaultRspamdUse      = true
	defaultRspamdHost     = "127.0.0.1"
	defaultSpamMoveScore  = 5.0
	defaultInterval       = "300s"
	defaultDaemon         = false
	defaultHttpPort       = 8080
	defaultImapTls        = true
	defaultImapPort       = 993
	defaultImapInbox      = "INBOX"
	defaultImapSpamFolder = "Spam"
	defaultSpamMark       = "*** SPAM ***"
)

type Configuration struct {
	ImapAccounts  []*ImapConfiguration `yaml:"imapAccounts,omitempty"`
	Spamd         SpamdConfiguration   `yaml:"spamd,omitempty"`
	Rspamd        RspamdConfiguration  `yaml:"rspamd,omitempty"`
	SpamThreshold float64              `yaml:"spamThreshold,omitempty"`
	Daemon        bool                 `yaml:"daemon,omitempty"`
	Interval      string               `yaml:"interval,omitempty"`
	HttpPort      int                  `yaml:"httpPort,omitempty"`
	SpamPrefix    string               `yaml:"spamMark,omitempty"`
	encrypt       string
	key           string
}

type ImapConfiguration struct {
	Name        string         `yaml:"name,omitempty"`
	Username    string         `yaml:"username,omitempty"`
	Password    string         `yaml:"password,omitempty"`
	Host        string         `yaml:"host,omitempty"`
	Port        int            `yaml:"port,omitempty"`
	Tls         bool           `yaml:"tls,omitempty"`
	Inbox       string         `yaml:"inbox,omitempty"`
	SpamFolder  string         `yaml:"spamFolder,omitempty"`
	Ok          bool           `yaml:"-"`
	UnreadMails int            `yaml:"-"`
	client      *client.Client `yaml:"-"`
}

type SpamdConfiguration struct {
	Use  bool   `yaml:"use,omitempty"`
	Host string `yaml:"host,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

type RspamdConfiguration struct {
	Use  bool   `yaml:"use,omitempty"`
	Host string `yaml:"host,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

func New() (*Configuration, error) {
	c := Configuration{}
	configdata, err := ioutil.ReadFile(defaultConfigFile)
	if err == nil {
		err = yaml.Unmarshal([]byte(configdata), &c)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling config file: %v\n", err)
		}
	} else {
		log.Printf("Config file not found. Use default parameters.\n")
	}
	if len(c.ImapAccounts) == 0 {
		log.Fatalf("No imap accounts configured. Stopping here.")
	}
	for _, a := range c.ImapAccounts {
		if a.Username == "" || a.Password == "" || a.Host == "" {
			log.Fatalf("Missing arguments for imap account. username, password and host are needed.")
		}
		if !a.Tls {
			a.Tls = defaultImapTls
		}
		if a.Port == 0 {
			a.Port = defaultImapPort
		}
		if a.Inbox == "" {
			a.Inbox = defaultImapInbox
		}
		if a.SpamFolder == "" {
			a.SpamFolder = defaultImapSpamFolder
		}
	}
	c.parseArguments()
	return &c, nil
}

func (c *Configuration) parseArguments() {
	flag.BoolVar(&c.Spamd.Use, "spamdUse", defaultSpamdUse, "use spamd")
	flag.StringVar(&c.Spamd.Host, "spamdHost", defaultSpamdHost, "spamd host name")
	flag.IntVar(&c.Spamd.Port, "spamdPort", defaultSpamdPort, "Port of the spamd server")
	flag.BoolVar(&c.Rspamd.Use, "rspamdUse", defaultRspamdUse, "use rspamd")
	flag.StringVar(&c.Rspamd.Host, "rspamdHost", defaultRspamdHost, "rspamd host name")
	flag.IntVar(&c.Rspamd.Port, "rspamdPort", defaultRspamdPort, "Port of the rspamd server")
	flag.Float64Var(&c.SpamThreshold, "spamThreshold", defaultSpamMoveScore, "score to move to spam folder")
	flag.StringVar(&c.Interval, "interval", defaultInterval, "interval for checking new mails")
	flag.BoolVar(&c.Daemon, "daemon", defaultDaemon, "start in daemon mode")
	flag.IntVar(&c.HttpPort, "port", defaultHttpPort, "Port for the WebUI")
	flag.StringVar(&c.encrypt, "encrypt", "", "password to encrypt with the internal key")
	flag.StringVar(&c.SpamPrefix, "spamMark", defaultSpamMark, "subject prefix for spam mails")

	flag.Parse()

	val, ok := os.LookupEnv("SPAMD_USE")
	if !isFlagPassed("spamdUse") && ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("format for spamd use is wrong: %s", val)
		}
		c.Spamd.Use = b
	}
	val, ok = os.LookupEnv("SPAMD_HOST")
	if !isFlagPassed("spamdHost") && ok {
		c.Spamd.Host = val
	}
	val, ok = os.LookupEnv("SPAMD_PORT")
	if !isFlagPassed("spamdPort") && ok {
		//var err error = nil
		p, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("format for spamd port is wrong: %s", val)
		}
		c.Spamd.Port = p
	}
	val, ok = os.LookupEnv("RSPAMD_USE")
	if !isFlagPassed("rspamdUse") && ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("format for rspamd use is wrong: %s", val)
		}
		c.Rspamd.Use = b
	}
	val, ok = os.LookupEnv("RSPAMD_HOST")
	if !isFlagPassed("rspamdHost") && ok {
		c.Rspamd.Host = val
	}
	val, ok = os.LookupEnv("RSPAMD_PORT")
	if !isFlagPassed("rspamdPort") && ok {
		//var err error = nil
		p, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("format for rspamd port is wrong: %s", val)
		}
		c.Rspamd.Port = p
	}
	val, ok = os.LookupEnv("SPAM_THRESHOLD")
	if !isFlagPassed("spamThreshold") && ok {
		t, err := strconv.ParseFloat(val, 64)
		if err != nil {
			log.Fatalf("format for spam threshold is wrong: %s", val)
		}
		c.SpamThreshold = t
	}
	val, ok = os.LookupEnv("Interval")
	if !isFlagPassed("interval") && ok {
		c.Interval = val
	}
	val, ok = os.LookupEnv("DAEMON")
	if !isFlagPassed("daemon") && ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("format for daemon use is wrong: %s", val)
		}
		c.Daemon = b
	}
	val, ok = os.LookupEnv("PORT")
	if !isFlagPassed("port") && ok {
		//var err error = nil
		p, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("format for http port is wrong: %s", val)
		}
		c.HttpPort = p
	}
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
