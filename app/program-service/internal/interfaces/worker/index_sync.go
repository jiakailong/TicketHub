package worker

import (
	"context"
	"log"
	"time"

	"tickethub/app/program-service/internal/application"
)

type IndexSyncRunner struct {
	syncer   application.ProgramIndexSyncService
	interval time.Duration
	enabled  bool
}

func NewIndexSyncRunner(syncer application.ProgramIndexSyncService, interval time.Duration) IndexSyncRunner {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return IndexSyncRunner{syncer: syncer, interval: interval, enabled: true}
}

func (r IndexSyncRunner) Start(ctx context.Context) {
	if !r.enabled {
		return
	}
	go func() {
		r.sync(ctx)
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.sync(ctx)
			}
		}
	}()
}

func (r IndexSyncRunner) sync(ctx context.Context) {
	result, err := r.syncer.Sync(ctx)
	if err != nil {
		log.Printf("program search index sync failed: %v", err)
		return
	}
	log.Printf("program search index sync completed: indexed=%d batches=%d", result.Indexed, result.Batches)
}
