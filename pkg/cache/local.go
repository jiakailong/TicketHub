package cache

import (
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

type Local interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration) bool
	Delete(key string)
	Close()
}

type RistrettoConfig struct {
	NumCounters int64
	MaxCost     int64
	BufferItems int64
}

type RistrettoLocal struct {
	cache *ristretto.Cache[string, []byte]
}

func NewRistrettoLocal(cfg RistrettoConfig) (*RistrettoLocal, error) {
	client, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: cfg.BufferItems,
	})
	if err != nil {
		return nil, err
	}
	return &RistrettoLocal{cache: client}, nil
}

func (c *RistrettoLocal) Get(key string) ([]byte, bool) {
	value, ok := c.cache.Get(key)
	return value, ok
}

func (c *RistrettoLocal) Set(key string, value []byte, ttl time.Duration) bool {
	return c.cache.SetWithTTL(key, value, int64(len(value)), ttl)
}

func (c *RistrettoLocal) Delete(key string) {
	c.cache.Del(key)
}

func (c *RistrettoLocal) Close() {
	c.cache.Close()
}

func (c *RistrettoLocal) Wait() {
	c.cache.Wait()
}
