package email

import (
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
)

// Email attachment. The content of the attachment is provided either as a
// UTF-8 string or as a Base64-encoded string ("encoded" set to "true").
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
	Encoded     bool   `json:"encoded"`
}

// Write the attachment to the specified multipart writer.
func (a Attachment) Write(w *multipart.Writer) error {
	headers := make(textproto.MIMEHeader)
	if len(a.Filename) != 0 {
		headers.Add("Content-Type", fmt.Sprintf("%s; name=%s", a.ContentType, a.Filename))
	} else {
		headers.Add("Content-Type", a.ContentType)
	}
	if a.Encoded {
		headers.Add("Content-Transfer-Encoding", "base64")
	} else {
		headers.Add("Content-Transfer-Encoding", "quoted-printable")
	}
	p, err := w.CreatePart(headers)
	if err != nil {
		return err
	}
	if a.Encoded {
		if _, err := p.Write([]byte(a.Content)); err != nil {
			return err
		}
	} else {
		q := quotedprintable.NewWriter(p)
		if _, err := q.Write([]byte(a.Content)); err != nil {
			return err
		}
		return q.Close()
	}
	return nil
}
