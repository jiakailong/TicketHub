package worker

import "context"

type Runner struct {
	index  IndexSyncRunner
	outbox OutboxRunner
	events ProgramEventRunner
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

func (r Runner) Start(ctx context.Context) {
	r.index.Start(ctx)
	r.outbox.Start(ctx)
	r.events.Start(ctx)
}
