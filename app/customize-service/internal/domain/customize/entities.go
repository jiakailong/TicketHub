package customize

import "time"

type ApiData struct {
	ID        int64
	Path      string
	Method    string
	UserID    int64
	CreatedAt time.Time
}

type MessageStatus string

const (
	MessageProduced MessageStatus = "PRODUCED"
	MessageConsumed MessageStatus = "CONSUMED"
	MessageFailed   MessageStatus = "FAILED"
)

type MessageRecord struct {
	ID        int64
	MessageID string
	Topic     string
	Status    MessageStatus
	Reason    string
}

type Rule struct {
	ID      int64
	Name    string
	Enabled bool
	Payload string
}
