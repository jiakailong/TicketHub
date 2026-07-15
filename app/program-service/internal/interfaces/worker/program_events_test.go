package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/mq"
)

func TestProgramEventInvalidatesCacheBeforeIndexUpdate(t *testing.T) {
	payload, err := json.Marshal(mq.ProgramChangedEvent{ProgramID: 1001, Operation: "UPSERT"})
	if err != nil {
		t.Fatal(err)
	}
	cache := &recordingProgramCacheInvalidator{}
	runner := NewProgramEventRunner(nil, "program.changed", failingProgramIndexer{}, cache)
	err = runner.handle(context.Background(), mq.Event{Payload: payload})
	if err == nil {
		t.Fatal("expected index failure")
	}
	if cache.programID != 1001 {
		t.Fatalf("invalidated program = %d, want 1001", cache.programID)
	}
}

type recordingProgramCacheInvalidator struct {
	programID int64
}

func (c *recordingProgramCacheInvalidator) Invalidate(_ context.Context, programID int64) error {
	c.programID = programID
	return nil
}

type failingProgramIndexer struct{}

func (failingProgramIndexer) EnsureIndex(context.Context) error {
	return errors.New("elasticsearch unavailable")
}

func (failingProgramIndexer) UpsertPrograms(context.Context, []program.Program) error {
	return nil
}

func (failingProgramIndexer) DeletePrograms(context.Context, []int64) error {
	return nil
}
