package domain

import "time"

type Message struct {
	RunID        string
	ReceiverType string
	ReceivedAt   time.Time
	Headers      map[string]string
	Fields       map[string]string
	Raw          []byte
}
