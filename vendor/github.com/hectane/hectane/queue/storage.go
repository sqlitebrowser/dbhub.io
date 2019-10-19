package queue

import (
	"github.com/pborman/uuid"

	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
)

const (
	bodyFilename     = "body"
	messageExtension = ".message"
)

// Message metadata.
type Message struct {
	id   string
	body string
	Host string
	From string
	To   []string
}

// Manager for message metadata and body on disk. All methods are safe to call
// from multiple goroutines.
type Storage struct {
	m         sync.Mutex
	directory string
}

// Determine the path to the directory containing the specified body.
func (s *Storage) bodyDirectory(body string) string {
	return path.Join(s.directory, body)
}

// Determine the filename of the specified body.
func (s *Storage) bodyFilename(body string) string {
	return path.Join(s.bodyDirectory(body), bodyFilename)
}

// Determine the filename of the specified message.
func (s *Storage) messageFilename(m *Message) string {
	return path.Join(s.directory, m.body, m.id) + messageExtension
}

// Load all messages with the specified body.
func (s *Storage) loadMessages(body string) []*Message {
	messages := make([]*Message, 0, 1)
	if files, err := ioutil.ReadDir(s.bodyDirectory(body)); err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), messageExtension) {
				m := &Message{
					id:   strings.TrimSuffix(f.Name(), messageExtension),
					body: body,
				}
				if r, err := os.Open(s.messageFilename(m)); err == nil {
					if err := json.NewDecoder(r).Decode(m); err == nil {
						messages = append(messages, m)
					}
					r.Close()
				}
			}
		}
	}
	return messages
}

// Create a Storage instance for the specified directory.
func NewStorage(directory string) *Storage {
	return &Storage{
		directory: directory,
	}
}

// Create a new message body. The writer must be closed after writing the
// message body.
func (s *Storage) NewBody() (io.WriteCloser, string, error) {
	body := uuid.New()
	if err := os.MkdirAll(s.bodyDirectory(body), 0700); err != nil {
		return nil, "", err
	}
	w, err := os.OpenFile(s.bodyFilename(body), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, "", err
	}
	return w, body, nil
}

// Load messages from the storage directory. Any messages that could not be
// loaded are ignored.
func (s *Storage) LoadMessages() ([]*Message, error) {
	directories, err := ioutil.ReadDir(s.directory)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return []*Message{}, nil
	}
	var messages []*Message
	for _, d := range directories {
		if d.IsDir() {
			if _, err := os.Stat(s.bodyFilename(d.Name())); err == nil {
				messages = append(messages, s.loadMessages(d.Name())...)
			}
		}
	}
	return messages, nil
}

// Save the specified message to disk.
func (s *Storage) SaveMessage(m *Message, body string) error {
	s.m.Lock()
	defer s.m.Unlock()
	m.id = uuid.New()
	m.body = body
	w, err := os.OpenFile(s.messageFilename(m), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return err
	}
	return nil
}

// Retreive a reader for the message body.
func (s *Storage) GetMessageBody(m *Message) (io.ReadCloser, error) {
	s.m.Lock()
	defer s.m.Unlock()
	return os.Open(s.bodyFilename(m.body))
}

// Delete the specified message. The message body is also deleted if no more
// messages exist.
func (s *Storage) DeleteMessage(m *Message) error {
	s.m.Lock()
	defer s.m.Unlock()
	if err := os.Remove(s.messageFilename(m)); err != nil {
		return err
	}
	d, err := os.Open(s.bodyDirectory(m.body))
	if err != nil {
		return err
	}
	defer d.Close()
	e, err := d.Readdir(2)
	if err != nil {
		return err
	}
	if len(e) == 1 {
		return os.RemoveAll(s.bodyDirectory(m.body))
	}
	return nil
}
