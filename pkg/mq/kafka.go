package mq

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaProducer struct {
	writer *kafka.Writer
}

func NewKafkaProducer(brokers []string, clientID string) *KafkaProducer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		BatchTimeout: 10 * time.Millisecond,
	}
	return &KafkaProducer{writer: writer}
}

func (p *KafkaProducer) Publish(ctx context.Context, event Event) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: event.Topic,
		Key:   []byte(event.Key),
		Value: event.Payload,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(event.Header.EventID)},
			{Key: "trace_id", Value: []byte(event.Header.TraceID)},
			{Key: "schema_version", Value: []byte(event.Header.SchemaVersion)},
			{Key: "occurred_at_unix_ms", Value: []byte(strconv.FormatInt(event.Header.OccurredAt.UnixMilli(), 10))},
		},
		Time: event.Header.OccurredAt,
	})
}

func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}

type KafkaConsumer struct {
	brokers []string
	groupID string
	mu      sync.Mutex
	readers map[string]*kafka.Reader
}

func NewKafkaConsumer(brokers []string, groupID string) *KafkaConsumer {
	return &KafkaConsumer{brokers: brokers, groupID: groupID, readers: make(map[string]*kafka.Reader)}
}

func (c *KafkaConsumer) Consume(ctx context.Context, topic string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 1
	}
	reader := c.reader(topic)

	events := make([]Event, 0, limit)
	for len(events) < limit {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return events, ctx.Err()
			}
			return events, err
		}
		events = append(events, eventFromKafkaMessage(reader, message))
	}
	return events, nil
}

func (c *KafkaConsumer) reader(topic string) *kafka.Reader {
	c.mu.Lock()
	defer c.mu.Unlock()
	if reader := c.readers[topic]; reader != nil {
		return reader
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  c.brokers,
		GroupID:  c.groupID,
		Topic:    topic,
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  500 * time.Millisecond,
	})
	c.readers[topic] = reader
	return reader
}

func (c *KafkaConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var firstErr error
	for topic, reader := range c.readers {
		if err := reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(c.readers, topic)
	}
	return firstErr
}

func eventFromKafkaMessage(reader *kafka.Reader, message kafka.Message) Event {
	header := Header{}
	for _, item := range message.Headers {
		switch item.Key {
		case "event_id":
			header.EventID = string(item.Value)
		case "trace_id":
			header.TraceID = string(item.Value)
		case "schema_version":
			header.SchemaVersion = string(item.Value)
		}
	}
	header.OccurredAt = message.Time
	return Event{
		Topic:   message.Topic,
		Key:     string(message.Key),
		Header:  header,
		Payload: message.Value,
		ack: func(ctx context.Context) error {
			return reader.CommitMessages(ctx, message)
		},
	}
}
