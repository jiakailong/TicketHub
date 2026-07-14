package redis

import (
	"github.com/redis/go-redis/v9"

	"tickethub/pkg/config"
)

func NewClient(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
