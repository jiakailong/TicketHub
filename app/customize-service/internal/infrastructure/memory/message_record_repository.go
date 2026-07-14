package memory

import (
	"context"
	"sync"

	"tickethub/app/customize-service/internal/domain/customize"
)

type MessageRecordRepository struct {
	mu      sync.RWMutex
	records map[string]customize.MessageRecord
}

func NewMessageRecordRepository() *MessageRecordRepository {
	return &MessageRecordRepository{records: make(map[string]customize.MessageRecord)}
}

func (r *MessageRecordRepository) Save(ctx context.Context, record customize.MessageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[record.MessageID+"@"+record.Topic] = record
	return nil
}

func (r *MessageRecordRepository) MarkFailed(ctx context.Context, messageID string, topic string, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := messageID + "@" + topic
	record := r.records[key]
	record.MessageID = messageID
	record.Topic = topic
	record.Status = customize.MessageFailed
	record.Reason = reason
	r.records[key] = record
	return nil
}
