package delayqueue

import (
	"context"
	"sort"
	"sync"
	"time"
)

type memoryClaim struct {
	message   Message
	visibleAt time.Time
}

type MemoryQueue struct {
	mu                sync.Mutex
	messages          map[string]map[string]Message
	processing        map[string]map[string]memoryClaim
	visibilityTimeout time.Duration
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		messages:          make(map[string]map[string]Message),
		processing:        make(map[string]map[string]memoryClaim),
		visibilityTimeout: defaultVisibilityTimeout,
	}
}

func (q *MemoryQueue) WithVisibilityTimeout(timeout time.Duration) *MemoryQueue {
	if timeout > 0 {
		q.visibilityTimeout = timeout
	}
	return q
}

func (q *MemoryQueue) Enqueue(ctx context.Context, msg Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.messages[msg.Topic] == nil {
		q.messages[msg.Topic] = make(map[string]Message)
	}
	q.messages[msg.Topic][msg.ID] = msg
	if q.processing[msg.Topic] != nil {
		delete(q.processing[msg.Topic], msg.ID)
	}
	return nil
}

func (q *MemoryQueue) ClaimDue(ctx context.Context, topic string, now time.Time, limit int) ([]Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit <= 0 {
		return nil, nil
	}
	if q.messages[topic] == nil {
		q.messages[topic] = make(map[string]Message)
	}
	if q.processing[topic] == nil {
		q.processing[topic] = make(map[string]memoryClaim)
	}
	for id, claim := range q.processing[topic] {
		if !claim.visibleAt.After(now) {
			q.messages[topic][id] = claim.message
			delete(q.processing[topic], id)
		}
	}
	due := make([]Message, 0)
	for _, msg := range q.messages[topic] {
		if !msg.AvailableAt.After(now) {
			due = append(due, msg)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].AvailableAt.Equal(due[j].AvailableAt) {
			return due[i].ID < due[j].ID
		}
		return due[i].AvailableAt.Before(due[j].AvailableAt)
	})
	if len(due) > limit {
		due = due[:limit]
	}
	for index := range due {
		due[index].Attempts++
		delete(q.messages[topic], due[index].ID)
		q.processing[topic][due[index].ID] = memoryClaim{message: due[index], visibleAt: now.Add(q.visibilityTimeout)}
	}
	return due, nil
}

func (q *MemoryQueue) Ack(ctx context.Context, topic string, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.messages[topic], id)
	delete(q.processing[topic], id)
	return nil
}
