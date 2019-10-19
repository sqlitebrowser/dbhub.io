package email

import (
	"github.com/hectane/hectane/queue"
	"github.com/kennygrant/sanitize"

	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// Abstract representation of an email.
type Email struct {
	From        string       `json:"from"`
	To          []string     `json:"to"`
	Cc          []string     `json:"cc"`
	Bcc         []string     `json:"bcc"`
	Subject     string       `json:"subject"`
	Headers     Headers      `json:"headers"`
	Text        string       `json:"text"`
	Html        string       `json:"html"`
	Attachments []Attachment `json:"attachments"`
}

// Write the headers for the email to the specified writer.
func (e *Email) writeHeaders(w io.Writer, id, boundary string) error {
	headers := Headers{
		"Message-Id":   fmt.Sprintf("<%s@hectane>", id),
		"From":         e.From,
		"To":           strings.Join(e.To, ", "),
		"Subject":      e.Subject,
		"Date":         time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700"),
		"MIME-Version": "1.0",
		"Content-Type": fmt.Sprintf("multipart/mixed; boundary=%s", boundary),
	}
	for k, v := range e.Headers {
		headers[k] = v
	}
	if len(e.Cc) > 0 {
		headers["Cc"] = strings.Join(e.Cc, ", ")
	}
	return headers.Write(w)
}

// Write the body of the email to the specified writer.
func (e *Email) writeBody(w *multipart.Writer) error {
	var (
		buff      = &bytes.Buffer{}
		altWriter = multipart.NewWriter(buff)
	)
	p, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{
			fmt.Sprintf("multipart/alternative; boundary=%s", altWriter.Boundary()),
		},
	})
	if err != nil {
		return err
	}
	if e.Text == "" {
		e.Text = sanitize.HTML(e.Html)
	}
	if e.Html == "" {
		e.Html = toHTML(e.Text)
	}
	if err := (Attachment{
		ContentType: "text/plain; charset=utf-8",
		Content:     e.Text,
	}.Write(altWriter)); err != nil {
		return err
	}
	if err := (Attachment{
		ContentType: "text/html; charset=utf-8",
		Content:     e.Html,
	}.Write(altWriter)); err != nil {
		return err
	}
	if err := altWriter.Close(); err != nil {
		return err
	}
	if _, err := io.Copy(p, buff); err != nil {
		return err
	}
	return nil
}

// Create an array of messages with the specified body.
func (e *Email) newMessages(s *queue.Storage, from, body string) ([]*queue.Message, error) {
	addresses := append(append(e.To, e.Cc...), e.Bcc...)
	m, err := GroupAddressesByHost(addresses)
	if err != nil {
		return nil, err
	}
	messages := make([]*queue.Message, 0, 1)
	for h, to := range m {
		msg := &queue.Message{
			Host: h,
			From: from,
			To:   to,
		}
		if err := s.SaveMessage(msg, body); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// Convert the email into an array of messages grouped by host suitable for
// delivery to the mail queue.
func (e *Email) Messages(s *queue.Storage) ([]*queue.Message, error) {
	from, err := mail.ParseAddress(e.From)
	if err != nil {
		return nil, err
	}
	w, body, err := s.NewBody()
	if err != nil {
		return nil, err
	}
	mpWriter := multipart.NewWriter(w)
	if err := e.writeHeaders(w, body, mpWriter.Boundary()); err != nil {
		return nil, err
	}
	if err := e.writeBody(mpWriter); err != nil {
		return nil, err
	}
	for _, a := range e.Attachments {
		if err := a.Write(mpWriter); err != nil {
			return nil, err
		}
	}
	if err := mpWriter.Close(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return e.newMessages(s, from.Address, body)
}
