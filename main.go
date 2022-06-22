package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/go-co-op/gocron"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	conf, err := New()
	if err != nil {
		log.Fatal(err)
	}
	b, err := os.ReadFile(conf.KeyFile)
	if err != nil {
		s := generateKey()
		b = []byte(s)
		err = os.WriteFile(conf.KeyFile, b, 0600)
		if err != nil {
			log.Fatalf("error writing key file: %v", err)
		}
		log.Println("New key was created. To encrypt password use `eatspam --encrypt <password>`")
	}
	conf.key = string(b)
	if conf.encrypt != "" {
		s, err := encrypt(conf.encrypt, conf.key)
		if err != nil {
			log.Fatalf("error encrypting string: %v\n", err)
		}
		fmt.Println(s)
		os.Exit(0)
	}
	d, _ := time.ParseDuration(conf.Interval)
	if conf.Daemon {
		log.Println("Start eatspam in daemon mode")
		log.Printf("Interval is %0.2f seconds\n", d.Seconds())
	} else {
		log.Println("Start eatspam in one time mode")
	}
	log.Printf("using strategy %s with thresholds %v\n", conf.Strategy, conf.Actions)
	if conf.Spamd.Use {
		log.Printf("use spamd at '%s' with port %d", conf.Spamd.Host, conf.Spamd.Port)
	}
	if conf.Rspamd.Use {
		log.Printf("use rspamd at '%s' with port %d", conf.Rspamd.Host, conf.Rspamd.Port)
	}
	if conf.Daemon {
		conf.startCron()
		conf.startHttpListener()
	} else {
		err := conf.spamChecker()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (conf *Configuration) startCron() {
	s := gocron.NewScheduler(time.UTC)
	frequency := conf.Interval
	value, unit, err := parseFrequency(frequency)
	if err != nil {
		log.Printf("Error starting sync cron job: %v", err)
		log.Printf("Start sync job every %d %s", 1, "days")
		_, err = s.Every(1).Days().Do(conf.cron)
	} else {
		switch unit {
		case "s":
			log.Printf("Start sync job every %d %s", value, "seconds")
			_, err = s.Every(value).Seconds().Do(conf.cron)
		case "m":
			log.Printf("Start sync job every %d %s", value, "minutes")
			_, err = s.Every(value).Minutes().Do(conf.cron)
		case "h":
			log.Printf("Start sync job every %d %s", value, "hours")
			_, err = s.Every(value).Hours().Do(conf.cron)
		case "d":
			log.Printf("Start sync job every %d %s", value, "days")
			_, err = s.Every(value).Days().Do(conf.cron)
		}
	}
	if err != nil {
		log.Fatalf("error creating cronjob: %v", err)
	}
	s.StartAsync()
}

func (conf *Configuration) cron() {
	err := conf.spamChecker()
	if err != nil {
		log.Printf("error checking spam: %v", err)
	}
}

func parseFrequency(f string) (int, string, error) {
	if len(f) < 2 {
		return 0, "", fmt.Errorf("illegal format")
	}
	valPart := f[:len(f)-1]
	unitPart := f[len(f)-1:]
	if len(valPart) == 0 || len(unitPart) == 0 || !strings.Contains("smhd", unitPart) {
		return 0, "", fmt.Errorf("illegal format")
	}
	value, err := strconv.Atoi(valPart)
	return value, unitPart, err
}

func generateKey() string {
	bytes := make([]byte, 32) //generate a random 32 byte key for AES-256
	if _, err := rand.Read(bytes); err != nil {
		panic(err.Error())
	}

	return hex.EncodeToString(bytes) //encode key in bytes to string for savin
}

func encrypt(stringToEncrypt string, keyString string) (string, error) {

	//Since the key is in string, we need to convert decode it to bytes
	key, _ := hex.DecodeString(keyString)
	plaintext := []byte(stringToEncrypt)

	//Create a new Cipher Block from the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("error creating aes cipher: %v", err)
	}

	//Create a new GCM - https://en.wikipedia.org/wiki/Galois/Counter_Mode
	//https://golang.org/pkg/crypto/cipher/#NewGCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating GCM: %v", err)
	}

	//Create a nonce. Nonce should be from GCM
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("error creating nonce: %v", err)
	}

	//Encrypt the data using aesGCM.Seal
	//Since we don't want to save the nonce somewhere else in this case, we add it as a prefix to the encrypted data. The first nonce argument in Seal is the prefix.
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return fmt.Sprintf("%x", ciphertext), nil
}

func decrypt(encryptedString string, keyString string) (string, error) {

	key, _ := hex.DecodeString(keyString)
	enc, _ := hex.DecodeString(encryptedString)

	//Create a new Cipher Block from the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("error creating aes cipher: %v", err)
	}

	//Create a new GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating GCM: %v", err)
	}

	//Get the nonce size
	nonceSize := aesGCM.NonceSize()

	//Extract the nonce from the encrypted data
	nonce, ciphertext := enc[:nonceSize], enc[nonceSize:]

	//Decrypt the data
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("error decrypting data: %v", err)
	}

	return fmt.Sprintf("%s", plaintext), nil
}
