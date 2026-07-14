package worker

import (
	"context"
	"log"
	"time"

	"tickethub/app/migrate-service/internal/application"
)

type Runner struct {
	migrations   application.MigrationWorker
	pollInterval time.Duration
}

func NewRunner(migrations application.MigrationWorker) Runner {
	return Runner{migrations: migrations, pollInterval: time.Second}
}

func (r Runner) Start(ctx context.Context) {
	go r.run(ctx)
}

func (r Runner) run(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := r.migrations.Poll(ctx); err != nil && ctx.Err() == nil {
			log.Printf("migrate-service migration batch failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
