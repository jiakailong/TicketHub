package mq

import (
	"context"
	"testing"
)

func TestMemoryBroker(t *testing.T) {
	broker := NewMemoryBroker()
	if err := broker.Publish(context.Background(), Event{Topic: "ticket_hub.create_order", Key: "1"}); err != nil {
		t.Fatal(err)
	}
	events, err := broker.Consume(context.Background(), "ticket_hub.create_order", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Key != "1" {
		t.Fatalf("events = %+v", events)
	}
}
