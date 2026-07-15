package redis

import (
	"context"
	"log"
	"strconv"

	redislib "github.com/redis/go-redis/v9"
)

type CacheInvalidationSubscriber struct {
	client  *redislib.Client
	channel string
	cache   *ProgramQueryCache
}

func NewCacheInvalidationSubscriber(client *redislib.Client, cache *ProgramQueryCache) CacheInvalidationSubscriber {
	return CacheInvalidationSubscriber{client: client, channel: cache.invalidationChannel(), cache: cache}
}

func (s CacheInvalidationSubscriber) Start(ctx context.Context) {
	if s.client == nil || s.cache == nil {
		return
	}
	go s.run(ctx)
}

func (s CacheInvalidationSubscriber) run(ctx context.Context) {
	subscription := s.client.Subscribe(ctx, s.channel)
	defer subscription.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-subscription.Channel():
			if !ok {
				return
			}
			programID, err := strconv.ParseInt(message.Payload, 10, 64)
			if err != nil {
				log.Printf("ignore invalid program cache invalidation payload: %q", message.Payload)
				continue
			}
			s.cache.InvalidateLocal(programID)
		}
	}
}
