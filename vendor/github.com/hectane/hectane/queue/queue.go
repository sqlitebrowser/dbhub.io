package queue

import (
	"github.com/Sirupsen/logrus"

	"time"
)

// Queue status information.
type QueueStatus struct {
	Uptime int                    `json:"uptime"`
	Hosts  map[string]*HostStatus `json:"hosts"`
}

// Mail queue managing the sending of messages to hosts.
type Queue struct {
	config     *Config
	Storage    *Storage
	log        *logrus.Entry
	hosts      map[string]*Host
	newMessage chan *Message
	getStats   chan chan *QueueStatus
	stop       chan bool
}

// Deliver the specified message to the appropriate host queue.
func (q *Queue) deliverMessage(m *Message) {
	if _, ok := q.hosts[m.Host]; !ok {
		q.hosts[m.Host] = NewHost(m.Host, q.Storage, q.config)
	}
	q.hosts[m.Host].Deliver(m)
}

// Generate stats for the queue. This is done by obtaining the information
// asynchronously and delivering it on the supplied channel when available.
func (q *Queue) stats(c chan *QueueStatus, startTime time.Time) {
	go func() {
		s := &QueueStatus{
			Uptime: int(time.Now().Sub(startTime) / time.Second),
			Hosts:  map[string]*HostStatus{},
		}
		for n, h := range q.hosts {
			s.Hosts[n] = h.Status()
		}
		c <- s
		close(c)
	}()
}

// Check for inactive host queues and shut them down.
func (q *Queue) checkForInactiveQueues() {
	for n, h := range q.hosts {
		if h.Idle() > time.Minute {
			h.Stop()
			delete(q.hosts, n)
		}
	}
}

// Receive new messages and deliver them to the specified host queue. Check for
// idle queues every so often and shut them down if they haven't been used.
func (q *Queue) run() {
	defer close(q.stop)
	startTime := time.Now()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
loop:
	for {
		select {
		case m := <-q.newMessage:
			q.deliverMessage(m)
		case c := <-q.getStats:
			q.stats(c, startTime)
		case <-ticker.C:
			q.checkForInactiveQueues()
		case <-q.stop:
			break loop
		}
	}
	q.log.Info("stopping host queues")
	for h := range q.hosts {
		q.hosts[h].Stop()
	}
	q.log.Info("shutting down")
}

// Create a new message queue. Any undelivered messages on disk will be added
// to the appropriate queue.
func NewQueue(c *Config) (*Queue, error) {
	q := &Queue{
		config:     c,
		Storage:    NewStorage(c.Directory),
		log:        logrus.WithField("context", "Queue"),
		hosts:      make(map[string]*Host),
		newMessage: make(chan *Message),
		getStats:   make(chan chan *QueueStatus),
		stop:       make(chan bool),
	}
	messages, err := q.Storage.LoadMessages()
	if err != nil {
		return nil, err
	}
	q.log.Infof("loaded %d message(s) from %s", len(messages), c.Directory)
	for _, m := range messages {
		q.deliverMessage(m)
	}
	go q.run()
	return q, nil
}

// Provide the status of each host queue.
func (q *Queue) Status() *QueueStatus {
	c := make(chan *QueueStatus)
	q.getStats <- c
	return <-c
}

// Deliver the specified message to the appropriate host queue.
func (q *Queue) Deliver(m *Message) {
	q.newMessage <- m
}

// Stop all active host queues.
func (q *Queue) Stop() {
	q.stop <- true
	<-q.stop
}
