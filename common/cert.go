package common

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"time"
)

func GenerateClientCert(userName string) (_ []byte, err error) {
	pageName := "Add user:generateClientCert()"

	// Use a template approach, similar to:
	//   https://github.com/driskell/log-courier/blob/master/lc-tlscert/lc-tlscert.go
	nowTime := time.Now()
	emailAddress := fmt.Sprintf("%s@%s", userName, DB4SServer())
	newCert := x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"DB Browser for SQLite"},
			CommonName:   emailAddress,
		},
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:                  false,
		NotAfter:              nowTime.AddDate(0, 0, CertDaysValid),
		NotBefore:             nowTime,
	}

	// TODO: Check this serial number isn't already in use, just to be safe
	serialNumLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	newCert.SerialNumber, err = rand.Int(rand.Reader, serialNumLimit)
	if err != nil {
		log.Printf("%s: Error when generating serial number: %v\n", pageName, err)
		return nil, err
	}

	// Load the certificate used for signing (the intermediate certificate)
	certFile, err := ioutil.ReadFile(SigningCert())
	if err != nil {
		log.Printf("%s: Error opening intermediate certificate file: %v\n", pageName, err)
		return
	}
	certPEM, _ := pem.Decode(certFile)
	if certPEM == nil {
		log.Printf("%s: Error when PEM decoding the intermediate certificate file\n", pageName)
		return
	}
	intCert, err := x509.ParseCertificate(certPEM.Bytes)
	if err != nil {
		log.Printf("%s: Error when parsing decoded intermediate certificate data: %v\n", pageName, err)
		return
	}

	// Load the private key for the intermediate certificate
	intKeyFile, err := ioutil.ReadFile(SigningCertKey())
	if err != nil {
		log.Printf("%s: Error opening intermediate certificate key: %v\n", pageName, err)
		return
	}
	intKeyPEM, _ := pem.Decode(intKeyFile)
	if certPEM == nil {
		log.Printf("%s: Error when PEM decoding the intermediate key file\n", pageName)
		return
	}
	intKey, err := x509.ParsePKCS1PrivateKey(intKeyPEM.Bytes)
	if err != nil {
		log.Printf("%s: Error when parsing intermediate certificate key: %v\n", pageName, err)
		return
	}

	// Generate a public key to sign the new certificate with
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Printf("%s: Failed to public key for signing: %v\n", pageName, err)
		return
	}

	// Generate the new certificate
	clientCert, err := x509.CreateCertificate(rand.Reader, &newCert, intCert, &clientKey.PublicKey, intKey)
	if err != nil {
		log.Printf("%s: Failed to create certificate: %v\n", pageName, err)
		return
	}

	// Convert the new certificate into PEM format
	buf := &bytes.Buffer{}
	err = pem.Encode(buf, &pem.Block{Type: "CERTIFICATE", Bytes: clientCert})
	if err != nil {
		log.Printf("%s: Failed to PEM encode certificate: %v\n", pageName, err)
		return
	}

	// Convert the private key for the certificate into PEM format
	buf2 := &bytes.Buffer{}
	err = pem.Encode(buf2, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})
	if err != nil {
		log.Printf("%s: Failed to PEM encode private key: %v\n", pageName, err)
		return
	}

	// Concatenate the newly generated certificate and its key
	_, err = buf.ReadFrom(buf2)
	if err != nil {
		log.Printf("%s: Failed to concatenate the PEM blocks: %v\n", pageName, err)
		return
	}

	log.Printf("New client cert generated for user '%s'", userName)

	return buf.Bytes(), nil
}
