# EatSpam

eatspam reads unread mails from configured IMAP servers and check them against Spamassassin and Rspamd. 
If the Score is higher than the configured threshold, an action like `move to spam`, `marks subject` or 
`add x-spam header` to the mail.

```
# eatspam --help
Usage of eatspam:
  -daemon
        start in daemon mode
  -encrypt string
        password to encrypt with the internal key
  -interval string
        interval for checking new mails (default "300s")
  -port int
        Port for the WebUI (default 8080)
  -rspamdHost string
        rspamd host name (default "127.0.0.1")
  -rspamdPort int
        Port of the rspamd server (default 11333)
  -rspamdUse
        use rspamd (default true)
  -spamMark string
        subject prefix for spam mails (default "*** SPAM ***")
  -spamThreshold float
        score to move to spam folder (default 5)
  -spamdHost string
        spamd host name (default "127.0.0.1")
  -spamdPort int
        Port of the spamd server (default 783)
  -spamdUse
        use spamd (default true)
```

- `eatspam --daemon` gets all parameters from eatspam.yaml or uses default values
- `eatspam --enccrypt <string>` encrypts the given string with the internal key
- `eatspam` without any parameters runs the spam check one time and terminates

eatspam.yaml.example show the structure of the configuration.
