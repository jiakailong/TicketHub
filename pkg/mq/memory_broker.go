package mq

import (
	"context"
	"sync"
)

type MemoryBroker struct {
	mu     sync.Mutex
	events map[string][]Event
}

func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{events: make(map[string][]Event)}
}

func (b *MemoryBroker) Publish(ctx context.Context, event Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events[event.Topic] = append(b.events[event.Topic], event)
	return nil
}

func (b *MemoryBroker) Consume(ctx context.Context, topic string, limit int) ([]Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 || len(b.events[topic]) == 0 {
		return nil, nil
	}
	if limit > len(b.events[topic]) {
		limit = len(b.events[topic])
	}
	out := append([]Event(nil), b.events[topic][:limit]...)
	b.events[topic] = b.events[topic][limit:]
	return out, nil
}
