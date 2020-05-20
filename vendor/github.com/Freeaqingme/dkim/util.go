package dkim

import (
	"bytes"
	"net/mail"
)

func readEML(eml []byte) (m *mail.Message, err error) {
	r := bytes.NewReader(eml)
	return mail.ReadMessage(r)
}
