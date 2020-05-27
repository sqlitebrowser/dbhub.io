package dkim

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

const (
	CRLF        = "\r\n"
	DOUBLE_CRLF = "\r\n\r\n"
	Empty       = ""
	WSP         = " "
)

var (
	CRLF_AS_BYTE  = []byte{'\r', '\n'}
	WSP_AS_BYTE   = []byte{' '}
)

var (
	headerRelaxRx = regexp.MustCompile(`\s+`)
	singleWspRx   = regexp.MustCompile(`[ \t]+`)
)

const (
	SignatureHeaderKey = "DKIM-Signature"
)

var StdSignableHeaders = []string{
	"Cc",
	"Content-Type",
	"Date",
	"From",
	"Reply-To",
	"Subject",
	"To",
	SignatureHeaderKey,
}

type DKIM struct {
	signableHeaders []string
	conf            Conf
	privateKey      *rsa.PrivateKey
	msgBody []byte
}

func New(conf Conf, keyPEM []byte) (d *DKIM, err error) {
	err = conf.Validate()
	if err != nil {
		return
	}
	if len(keyPEM) == 0 {
		return nil, errors.New("invalid key PEM data")
	}
	der, _ := pem.Decode(keyPEM)
	key, err := x509.ParsePKCS1PrivateKey(der.Bytes)
	if err != nil {
		return nil, err
	}
	return NewByKey(conf, key), nil
}

func NewByKey(conf Conf, key *rsa.PrivateKey) *DKIM {
	dkim := &DKIM{
		signableHeaders: StdSignableHeaders,
		conf: conf,
	}
	dkim.privateKey = key
	return dkim
}

var (
	rxWsCompress = regexp.MustCompile(`[ \t]+`)
	rxWsCRLF     = regexp.MustCompile(` \r\n`)
)

func (d *DKIM) canonicalBody(msg *mail.Message) []byte {
	body := d.msgBody
	if d.conf.RelaxedBody() {
		if len(d.msgBody) == 0 {
			return nil
		}
		// Reduce WSP sequences to single WSP
		body = singleWspRx.ReplaceAll(body, WSP_AS_BYTE)
		body = rxWsCompress.ReplaceAll(body, []byte(" "))

		// Ignore all whitespace at end of lines.
		// Implementations MUST NOT remove the CRLF
		// at the end of the line
		body = rxWsCRLF.ReplaceAll(body, []byte("\r\n"))
	} else {
		if len(body) == 0 {
			return CRLF_AS_BYTE
		}
	}

	// Ignore all empty lines at the end of the message body
	for i := len(body) - 1; i >= 0; i-- {
		if body[i] != '\r' && body[i] != '\n' && body[i] != ' ' {
			body = body[:i+1]
			break
		}
	}

	return append(body, CRLF_AS_BYTE...)
}

func (d *DKIM) canonicalBodyHash(msg *mail.Message) []byte {
	b := d.canonicalBody(msg)
	digest := d.conf.Hash().New()
	digest.Write([]byte(b))

	return digest.Sum(nil)
}

func (d *DKIM) signableHeaderBlock(msg *mail.Message) string {
	signableHeaderList := make(mail.Header)
	signableHeaderKeys := make([]string, 0)

	for _, k := range d.signableHeaders {
		if v := msg.Header[k]; len(v) != 0 {
			signableHeaderList[k] = v
			signableHeaderKeys = append(signableHeaderKeys, k)
		}
	}

	d.conf[BodyHashKey] = base64.StdEncoding.EncodeToString(d.canonicalBodyHash(msg))
	d.conf[FieldsKey] = strings.Join(signableHeaderKeys, ":")

	signableHeaderList[SignatureHeaderKey] = []string{d.conf.String()}
	signableHeaderKeys = append(signableHeaderKeys, SignatureHeaderKey)

	relax := d.conf.RelaxedHeader()
	canonical := make([]string, 0, len(signableHeaderKeys))
	for _, key := range signableHeaderKeys {
		value := signableHeaderList[key][0]
		if relax {
			value = headerRelaxRx.ReplaceAllString(value, WSP)
			key = strings.ToLower(key)
		}
		canonical = append(canonical, fmt.Sprintf("%s:%s", key, strings.TrimSpace(value)))
	}
	// According to RFC6376 http://tools.ietf.org/html/rfc6376#section-3.7
	// the DKIM header must be inserted without a trailing <CRLF>.
	// That's why we have to trim the space from the canonical header.
	return strings.TrimSpace(strings.Join(canonical, CRLF) + CRLF)
}

func (d *DKIM) signature(msg *mail.Message) (string, error) {
	block := d.signableHeaderBlock(msg)
	hash := d.conf.Hash()
	digest := hash.New()
	digest.Write([]byte(block))

	sig, err := rsa.SignPKCS1v15(rand.Reader, d.privateKey, hash, digest.Sum(nil))
	if err != nil {
		return Empty, err
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

func (d *DKIM) Sign(eml []byte) ([]byte, error) {
	msg, err := readEML(eml)

	if err != nil {
		return eml, err
	}

	d.readBody(msg)
	sig, err := d.signature(msg)
	if err != nil {
		return eml, err
	}
	d.conf[SignatureDataKey] = sig

	// Append the signature header. Keep in mind these are raw values,
	// so we add a <SP> character before the key-value list
	/* msg.Header[SignatureHeaderKey] = []string{d.conf.String()} */

	buf := new(bytes.Buffer)
	for key, _ := range msg.Header {
		buf.WriteString(key)
		buf.WriteString(": ")
		buf.WriteString(msg.Header.Get(key))
		buf.WriteString(CRLF)
	}

	buf.WriteString(SignatureHeaderKey)
	buf.WriteString(":")
	buf.WriteString(d.conf.String())
	buf.WriteString(DOUBLE_CRLF)
	buf.Write(d.msgBody)

	return buf.Bytes(), nil
}

func (d *DKIM) readBody(msg *mail.Message) {
	body := new(bytes.Buffer)
	body.ReadFrom(msg.Body)
	d.msgBody = body.Bytes()
}
