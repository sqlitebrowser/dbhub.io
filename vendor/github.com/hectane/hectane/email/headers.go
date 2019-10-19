package email

import (
	"fmt"
	"io"
)

// Map of email headers.
type Headers map[string]string

// Write the headers to the specified io.Writer.
func (e Headers) Write(w io.Writer) error {
	for k, v := range e {
		if _, err := w.Write([]byte(fmt.Sprintf("%s: %s\r\n", k, v))); err != nil {
			return err
		}
	}
	_, err := w.Write([]byte("\r\n"))
	return err
}
