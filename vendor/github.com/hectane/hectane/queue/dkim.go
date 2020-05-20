package queue

import (
	"bytes"
	"io"
	"net/mail"
	"strings"

	"fmt"

	"io/ioutil"

	"github.com/Freeaqingme/dkim"
)

var dkimInstances = make(map[string]*dkim.DKIM)

func dkimFor(from string, config *Config) (*dkim.DKIM, error) {
	emailAddress, err := mail.ParseAddress(from)
	if err != nil {
		return nil, err
	}
	domain := strings.Split(emailAddress.Address, "@")[1]
	dkimInstance, found := dkimInstances[domain]
	if found {
		return dkimInstance, nil
	}
	if config.DKIMConfigs == nil {
		dkimInstances[domain] = nil
		return nil, nil
	}
	dkimConfig, found := config.DKIMConfigs[domain]
	if !found {
		dkimInstances[domain] = nil
		return nil, nil
	}
	conf, err := dkim.NewConf(domain, dkimConfig.Selector)
	if err != nil {
		dkimInstances[domain] = nil
		return nil, err
	}
	if dkimConfig.Canonicalization != "" {
		conf[dkim.CanonicalizationKey] = dkimConfig.Canonicalization
	}
	// dkim.StdSignableHeaders = []string{"From", "To", "Subject", "From"}
	dkimInstance, err = dkim.New(conf, []byte(dkimConfig.PrivateKey))

	dkimInstances[domain] = dkimInstance
	return dkimInstance, nil
}

func dkimSigned(from string, input io.ReadCloser, config *Config) (io.ReadCloser, error) {
	dkim, err := dkimFor(from, config)
	if err != nil {
		return nil, fmt.Errorf("error while getting dkimInstances for %q: %s", from, err)
	}
	if dkim == nil {
		return input, nil
	}
	// TODO: Do not load the content
	defer input.Close()
	email, err := ioutil.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("error while ReadAll: %s", err)
	}
	signedEmail, err := dkim.Sign(email)
	if err != nil {
		return nil, fmt.Errorf("error while signing the email: %s", err)
	}
	return ioutil.NopCloser(bytes.NewReader(signedEmail)), nil
}
