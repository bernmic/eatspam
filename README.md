# EatSpam

eatspam reads unread mails from configured IMAP servers and check them against Spamassassin and Rspamd. 
If the Score is higher than the configured threshold, an action like `move to spam`, `marks subject` or 
`add x-spam header` to the mail can be triggered.

```
# eatspam --help
Usage of eatspam:
  -configFile string
        location of configuration file, default 'config/eatspam.yaml' (default "config/eatspam.yaml")
  -daemon
        start in daemon mode, default false
  -encrypt string
        password to encrypt with the internal key
  -interval string
        interval for checking new mails, default 300s (default "300s")
  -keyFile string
        location of the key file for password en-/decryption, default 'config/eatspam.key' (default "config/eatspam.key")
  -port int
        Port for the WebUI, default 8080 (default 8080)
  -rspamdHost string
        rspamd host name, default localhost (default "127.0.0.1")
  -rspamdPort int
        Port of the rspamd server, default 11333 (default 11333)
  -rspamdUse
        use rspamd, default true (default true)
  -spamMark string
        subject prefix for spam mails, default '*** SPAM ***' (default "*** SPAM ***")
  -spamdHost string
        spamd host name, default localhost (default "127.0.0.1")
  -spamdPort int
        Port of the spamd server, default 783 (default 783)
  -spamdUse
        use spamd, default true (default true)
```

- `eatspam --daemon` gets all parameters from eatspam.yaml or uses default values
- `eatspam --encrypt <string>` encrypts the given string with the internal key
- `eatspam` without any parameters runs the spam check one time and terminates

eatspam.yaml.example show the structure of the configuration.

## Installation

Create a folder config beside the executable and put a valid eatspam.yaml in that folder. It is also possible to have a 
different location and filename by setting the `--configFile` cli parameter. 

eatspam uses encrypted passwords for the IMAP server. On the first start eatspam will generate `config/eatspam.key`, 
a key file for password encryption. Again, location and name can be changed with cli parameter `--keyFile`. Passwords 
can now be encrypted by calling eatspam with the cli parameter `--encrypt <password>`. The encrypted password will be 
printed out and can be used in the password field for an IMAP configuration.

## Configuration

Strategy can be one of the following:

### average (default)

Take the average of all configured backends and calculate the action with the configured thresholds. Default threshold are:

```
  4.0: add header
  6.0: reject
```

### lowest

The lowest score and action is used

### highest

The highest score and action is used

### spamd

Use always spamd (spamassassin) result

### rspamd

Use always rspamd result
