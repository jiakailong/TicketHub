package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"tickethub/pkg/mq"
)

func TestOutboxRunnerPublishesAndCompletesEvent(t *testing.T) {
	repository := &fakeOutboxRepository{events: []mq.Event{{Topic: "orders", Header: mq.Header{EventID: "1"}}}}
	publisher := &fakeOutboxProducer{}
	runner := NewOutboxRunner(repository, publisher)
	runner.flush(context.Background())
	if publisher.calls != 1 || repository.published != "1" || repository.retried != "" {
		t.Fatalf("publisher=%+v repository=%+v", publisher, repository)
	}
}

func TestOutboxRunnerReschedulesPublishFailure(t *testing.T) {
	repository := &fakeOutboxRepository{events: []mq.Event{{Topic: "orders", Header: mq.Header{EventID: "2"}}}}
	publisher := &fakeOutboxProducer{err: errors.New("kafka unavailable")}
	runner := NewOutboxRunner(repository, publisher)
	runner.flush(context.Background())
	if repository.retried != "2" || repository.published != "" {
		t.Fatalf("repository=%+v", repository)
	}
}

type fakeOutboxRepository struct {
	events    []mq.Event
	published string
	retried   string
}

func (r *fakeOutboxRepository) Save(ctx context.Context, event mq.Event) error { return nil }

func (r *fakeOutboxRepository) Claim(ctx context.Context, limit int, lease time.Duration) ([]mq.Event, error) {
	return append([]mq.Event(nil), r.events...), nil
}

func (r *fakeOutboxRepository) MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	r.published = eventID
	return nil
}

func (r *fakeOutboxRepository) MarkRetry(ctx context.Context, eventID string, availableAt time.Time, detail string) error {
	r.retried = eventID
	return nil
}

type fakeOutboxProducer struct {
	calls int
	err   error
}

func (p *fakeOutboxProducer) Publish(ctx context.Context, event mq.Event) error {
	p.calls++
	return p.err
}
