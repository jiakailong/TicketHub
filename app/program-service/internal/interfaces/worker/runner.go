package worker

import "context"

type Runner struct {
	index             IndexSyncRunner
	outbox            OutboxRunner
	events            ProgramEventRunner
	cacheInvalidation BackgroundRunner
}

type BackgroundRunner interface {
	Start(ctx context.Context)
}

func (r Runner) WithProgramEvents(events ProgramEventRunner) Runner {
	r.events = events
	return r
}

func NewRunner(index IndexSyncRunner) Runner {
	return Runner{index: index}
}

func (r Runner) WithOutbox(outbox OutboxRunner) Runner {
	r.outbox = outbox
	return r
}

func (r Runner) WithCacheInvalidation(cacheInvalidation BackgroundRunner) Runner {
	r.cacheInvalidation = cacheInvalidation
	return r
}

func (r Runner) Start(ctx context.Context) {
	r.index.Start(ctx)
	r.outbox.Start(ctx)
	r.events.Start(ctx)
	if r.cacheInvalidation != nil {
		r.cacheInvalidation.Start(ctx)
	}
}
