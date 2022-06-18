# EatSpam

eatspam reads unread mails from configured IMAP servers and check them against Spamassassin and Rspamd. 
If the Score is higher than the configured threshold, an action like `move to spam`, `marks subject` or 
`add x-spam header` to the mail.
