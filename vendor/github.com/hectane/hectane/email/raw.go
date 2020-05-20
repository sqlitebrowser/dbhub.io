package email

import (
	"github.com/hectane/hectane/queue"
)

// Raw represents a raw email message ready for delivery.
type Raw struct {
	From string   `json:"from"`
	To   []string `json:"to"`
	Body string   `json:"body"`
}

// DeliverToQueue delivers raw messages to the queue.
func (r *Raw) DeliverToQueue(q *queue.Queue) error {
	w, body, err := q.Storage.NewBody()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(r.Body)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	hostMap, err := GroupAddressesByHost(r.To)
	if err != nil {
		return err
	}
	for h, to := range hostMap {
		m := &queue.Message{
			Host: h,
			From: r.From,
			To:   to,
		}
		if err := q.Storage.SaveMessage(m, body); err != nil {
			return err
		}
		q.Deliver(m)
	}
	return nil
}
