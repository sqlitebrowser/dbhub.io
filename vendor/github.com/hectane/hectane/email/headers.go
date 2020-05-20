package email

import (
	"fmt"
	"io"
	"mime"
)

// Map of email headers.
type Headers map[string]string

// Write the headers to the specified io.Writer. RFC 2047 provides details on
// how non-ASCII characters should be encoded.
func (e Headers) Write(w io.Writer) error {
	for k, v := range e {
		h := fmt.Sprintf("%s: %s\r\n", k, mime.QEncoding.Encode("utf-8", v))
		if _, err := w.Write([]byte(h)); err != nil {
			return err
		}
	}
	_, err := w.Write([]byte("\r\n"))
	return err
}
