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
	defaultConfigFile     = "config/eatspam.yaml"
	defaultKeyFile        = "config/eatspam.key"
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
	defaultStrategy       = "average"
)

const (
	strategyAverage = "average"
	strategyLowest  = "lowest"
	strategyHighest = "highest"
	strategySpamd   = "spamd"
	strategyRspamd  = "rspamd"
)

type Configuration struct {
	ImapAccounts  []*ImapConfiguration `yaml:"imapAccounts,omitempty"`
	Spamd         SpamdConfiguration   `yaml:"spamd,omitempty"`
	Rspamd        RspamdConfiguration  `yaml:"rspamd,omitempty"`
	Http          HttpConfiguration    `yaml:"http,omitempty"`
	SpamThreshold float64              `yaml:"spamThreshold,omitempty"`
	Daemon        bool                 `yaml:"daemon,omitempty"`
	Interval      string               `yaml:"interval,omitempty"`
	SpamPrefix    string               `yaml:"spamMark,omitempty"`
	ConfigFile    string               `yaml:"-"`
	KeyFile       string               `yaml:"keyFile,omitempty"`
	Actions       map[float64]string   `yaml:"actions,omitempty"`
	Strategy      string               `yaml:"strategy,omitempty"`
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

type HttpConfiguration struct {
	Port     int    `yaml:"port,omitempty"`
	Password string `yaml:"password,omitempty"`
}

func New() (*Configuration, error) {
	// peek cli params and environment for the configFile parameter
	cl := configLocation()
	c := Configuration{}
	// load and parse config file
	configdata, err := ioutil.ReadFile(cl)
	if err == nil {
		err = yaml.Unmarshal([]byte(configdata), &c)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling config file: %v\n", err)
		}
	} else {
		log.Printf("Config file %s not found. Use default parameters.\n", cl)
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
	if c.Actions == nil || len(c.Actions) == 0 {
		c.Actions = map[float64]string{
			4.0: spamActionAddHeader,
			6.0: spamActionRewriteSubject,
			8.0: spamActionReject,
		}
	}
	// parse all given cli parameters and environment variables
	c.parseArguments()
	return &c, nil
}

func (c *Configuration) parseArguments() {
	cp := Configuration{}
	flag.BoolVar(&cp.Spamd.Use, "spamdUse", defaultSpamdUse, "use spamd, default true")
	flag.StringVar(&cp.Spamd.Host, "spamdHost", defaultSpamdHost, "spamd host name, default localhost")
	flag.IntVar(&cp.Spamd.Port, "spamdPort", defaultSpamdPort, "Port of the spamd server, default 783")
	flag.BoolVar(&cp.Rspamd.Use, "rspamdUse", defaultRspamdUse, "use rspamd, default true")
	flag.StringVar(&cp.Rspamd.Host, "rspamdHost", defaultRspamdHost, "rspamd host name, default localhost")
	flag.IntVar(&cp.Rspamd.Port, "rspamdPort", defaultRspamdPort, "Port of the rspamd server, default 11333")
	flag.Float64Var(&cp.SpamThreshold, "spamThreshold", defaultSpamMoveScore, "score to move to spam folder, default 5.0")
	flag.StringVar(&cp.Interval, "interval", defaultInterval, "interval for checking new mails, default 300s")
	flag.BoolVar(&cp.Daemon, "daemon", defaultDaemon, "start in daemon mode, default false")
	flag.IntVar(&cp.Http.Port, "httpPort", defaultHttpPort, "Port for the WebUI, default 8080")
	flag.StringVar(&cp.encrypt, "encrypt", "", "password to encrypt with the internal key")
	flag.StringVar(&cp.SpamPrefix, "spamMark", defaultSpamMark, "subject prefix for spam mails, default '*** SPAM ***'")
	flag.StringVar(&cp.ConfigFile, "configFile", defaultConfigFile, "location of configuration file, default 'config/eatspam.yaml'")
	flag.StringVar(&cp.KeyFile, "keyFile", defaultKeyFile, "location of the key file for password en-/decryption, default 'config/eatspam.key'")
	flag.StringVar(&cp.Strategy, "strategy", defaultStrategy, "strategy for spam handling (average, lowest, highest, default 'average'")

	flag.Parse()

	c.encrypt = cp.encrypt
	c.Spamd.Use = boolConfig("spamdUse", cp.Spamd.Use, "SPAMD_USE", c.Spamd.Use)
	c.Spamd.Host = stringConfig("spamdHold", cp.Spamd.Host, "SPAMD_HOST", c.Spamd.Host)
	c.Spamd.Port = intConfig("spamdPort", cp.Spamd.Port, "SPAMD_PORT", c.Spamd.Port)

	c.Rspamd.Use = boolConfig("rspamdUse", cp.Rspamd.Use, "RSPAMD_USE", c.Rspamd.Use)
	c.Rspamd.Host = stringConfig("rspamdHold", cp.Rspamd.Host, "RSPAMD_HOST", c.Rspamd.Host)
	c.Rspamd.Port = intConfig("rspamdPort", cp.Rspamd.Port, "RSPAMD_PORT", c.Rspamd.Port)

	c.SpamThreshold = floatConfig("spamThreshold", cp.SpamThreshold, "SPAM_THRESHOLD", c.SpamThreshold)

	c.Interval = stringConfig("interval", cp.Interval, "INTERVAL", c.Interval)

	c.Daemon = boolConfig("daemon", cp.Daemon, "DAEMON", c.Daemon)

	c.Http.Port = intConfig("httpPort", cp.Http.Port, "HTTP_PORT", c.Http.Port)

	c.ConfigFile = stringConfig("configFile", cp.ConfigFile, "CONFIG_FILE", c.ConfigFile)
	c.KeyFile = stringConfig("keyFile", cp.KeyFile, "KEY_FILE", c.KeyFile)

	c.Strategy = stringConfig("strategy", cp.Strategy, "STRATEGY", c.Strategy)
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

func configLocation() string {
	args := os.Args
	for i, a := range args {
		if a == "-configFile" || a == "--configFile" {
			if i >= len(args)-1 {
				log.Fatalf("configFile parameter given without a value")
			}
			return args[i+1]
		}
	}
	if a, ok := os.LookupEnv("CONFIG_FILE"); ok {
		return a
	}
	return defaultConfigFile
}

func stringConfig(parmName string, parmValue string, envName string, fileValue string) string {
	if isFlagPassed(parmName) {
		return parmValue
	} else if val, ok := os.LookupEnv(envName); ok {
		return val
	}
	if fileValue == "" {
		return parmValue
	}
	return fileValue
}

func intConfig(parmName string, parmValue int, envName string, fileValue int) int {
	if isFlagPassed(parmName) {
		return parmValue
	} else if val, ok := os.LookupEnv(envName); ok {
		p, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("format for int is wrong: %s", val)
		}
		return p
	}
	if fileValue == 0 {
		return parmValue
	}
	return fileValue
}

func floatConfig(parmName string, parmValue float64, envName string, fileValue float64) float64 {
	if isFlagPassed(parmName) {
		return parmValue
	} else if val, ok := os.LookupEnv(envName); ok {
		t, err := strconv.ParseFloat(val, 64)
		if err != nil {
			log.Fatalf("format for float is wrong: %s", val)
		}
		return t
	}
	if fileValue == 0.0 {
		return parmValue
	}
	return fileValue
}

func boolConfig(parmName string, parmValue bool, envName string, fileValue bool) bool {
	if isFlagPassed(parmName) {
		return parmValue
	} else if val, ok := os.LookupEnv(envName); ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("format for bool is wrong: %s", val)
		}
		return b
	}
	return fileValue
}
