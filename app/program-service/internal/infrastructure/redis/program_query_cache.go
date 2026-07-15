package redis

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/lock"
	"tickethub/pkg/observability"
)

type programQuerySource interface {
	SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error)
	FindProgram(ctx context.Context, programID int64) (program.Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error)
}

type QueryCacheOptions struct {
	LocalTTL       time.Duration
	RedisTTL       time.Duration
	RebuildLockTTL time.Duration
	RebuildWait    time.Duration
	RebuildPoll    time.Duration
}

type cacheEnvelope struct {
	Generation string          `json:"generation"`
	Payload    json.RawMessage `json:"payload"`
}

var invalidateProgramCacheScript = redislib.NewScript(`
redis.call("INCR", KEYS[1])
redis.call("PEXPIRE", KEYS[1], ARGV[1])
for index = 2, #KEYS do
  redis.call("DEL", KEYS[index])
end
return 1
`)

var storeProgramCacheScript = redislib.NewScript(`
local generation = redis.call("GET", KEYS[1]) or "0"
if generation ~= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[2], ARGV[2], "PX", ARGV[3])
return 1
`)

type ProgramQueryCache struct {
	source       programQuerySource
	client       redislib.Cmdable
	keys         cache.KeyBuilder
	local        cache.Local
	localLocks   *cache.StripedRWMutex
	rebuildLocks lock.Locker
	options      QueryCacheOptions
}

func NewProgramQueryCache(
	source programQuerySource,
	client redislib.Cmdable,
	keys cache.KeyBuilder,
	local cache.Local,
	localLocks *cache.StripedRWMutex,
	rebuildLocks lock.Locker,
	options QueryCacheOptions,
) *ProgramQueryCache {
	if options.LocalTTL <= 0 {
		options.LocalTTL = 45 * time.Second
	}
	if options.RedisTTL <= 0 {
		options.RedisTTL = 5 * time.Minute
	}
	if options.RebuildLockTTL <= 0 {
		options.RebuildLockTTL = 5 * time.Second
	}
	if options.RebuildWait <= 0 {
		options.RebuildWait = 750 * time.Millisecond
	}
	if options.RebuildPoll <= 0 {
		options.RebuildPoll = 25 * time.Millisecond
	}
	if localLocks == nil {
		localLocks = cache.NewStripedRWMutex(256)
	}
	return &ProgramQueryCache{
		source: source, client: client, keys: keys, local: local,
		localLocks: localLocks, rebuildLocks: rebuildLocks, options: options,
	}
}

func (c *ProgramQueryCache) SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	return c.source.SearchPrograms(ctx, keyword, city, page, pageSize)
}

func (c *ProgramQueryCache) SearchProgramsAfter(ctx context.Context, keyword string, city string, cursor string, pageSize int) ([]program.Program, string, error) {
	if source, ok := c.source.(interface {
		SearchProgramsAfter(context.Context, string, string, string, int) ([]program.Program, string, error)
	}); ok {
		return source.SearchProgramsAfter(ctx, keyword, city, cursor, pageSize)
	}
	items, err := c.source.SearchPrograms(ctx, keyword, city, 1, pageSize)
	return items, "", err
}

func (c *ProgramQueryCache) SuggestPrograms(ctx context.Context, prefix string, limit int) ([]string, error) {
	if source, ok := c.source.(interface {
		SuggestPrograms(context.Context, string, int) ([]string, error)
	}); ok {
		return source.SuggestPrograms(ctx, prefix, limit)
	}
	items, err := c.source.SearchPrograms(ctx, prefix, "", 1, limit)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Title)
	}
	return result, nil
}

func (c *ProgramQueryCache) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	key := c.keys.Build("cache", "program", programID)
	lockKey := c.keys.Build("lock", "cache-rebuild", "program", programID)
	versionKey := c.keys.Build("cache", "version", programID)
	var item program.Program
	err := c.loadThrough(ctx, key, lockKey, versionKey, "program", &item, func() (any, error) {
		return c.source.FindProgram(ctx, programID)
	})
	return item, err
}

func (c *ProgramQueryCache) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	key := c.keys.Build("cache", "program-categories", programID)
	lockKey := c.keys.Build("lock", "cache-rebuild", "program-categories", programID)
	versionKey := c.keys.Build("cache", "version", programID)
	var items []program.TicketCategory
	err := c.loadThrough(ctx, key, lockKey, versionKey, "program_categories", &items, func() (any, error) {
		return c.source.ListTicketCategories(ctx, programID)
	})
	return items, err
}

func (c *ProgramQueryCache) ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error) {
	items, err := c.source.ListSeats(ctx, programID, ticketCategoryID)
	if err != nil || len(items) == 0 {
		return items, err
	}
	keys := make([]string, len(items))
	for index, item := range items {
		keys[index] = c.keys.Build("seat", programID, item.ID)
	}
	states, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		// Redis Lua remains the authoritative state check during order creation.
		return items, nil
	}
	for index, raw := range states {
		value, _ := raw.(string)
		switch {
		case strings.HasPrefix(value, "locked:"):
			items[index].Status = program.SeatLocked
		case strings.HasPrefix(value, "sold:"):
			items[index].Status = program.SeatSold
		}
	}
	return items, nil
}

func (c *ProgramQueryCache) MinPricesByProgramIDs(ctx context.Context, programIDs []int64) (map[int64]int64, error) {
	if source, ok := c.source.(interface {
		MinPricesByProgramIDs(context.Context, []int64) (map[int64]int64, error)
	}); ok {
		return source.MinPricesByProgramIDs(ctx, programIDs)
	}
	result := make(map[int64]int64, len(programIDs))
	for _, programID := range programIDs {
		categories, err := c.ListTicketCategories(ctx, programID)
		if err != nil {
			return nil, err
		}
		for _, category := range categories {
			if result[programID] == 0 || category.PriceCent < result[programID] {
				result[programID] = category.PriceCent
			}
		}
	}
	return result, nil
}

func (c *ProgramQueryCache) Invalidate(ctx context.Context, programID int64) error {
	keys := c.cacheKeys(programID)
	for _, key := range keys {
		c.invalidateLocalKey(key)
	}
	scriptKeys := append([]string{c.keys.Build("cache", "version", programID)}, keys...)
	if err := invalidateProgramCacheScript.Run(ctx, c.client, scriptKeys, (7 * 24 * time.Hour).Milliseconds()).Err(); err != nil {
		return err
	}
	return c.client.Publish(ctx, c.invalidationChannel(), programID).Err()
}

func (c *ProgramQueryCache) InvalidateLocal(programID int64) {
	for _, key := range c.cacheKeys(programID) {
		c.invalidateLocalKey(key)
	}
}

func (c *ProgramQueryCache) loadThrough(
	ctx context.Context,
	key string,
	lockKey string,
	versionKey string,
	kind string,
	target any,
	loadSource func() (any, error),
) error {
	if c.loadLocal(key, target) {
		c.recordCacheRequest(kind, "local", "hit")
		return nil
	}
	c.recordCacheRequest(kind, "local", "miss")

	if data, generation, ok := c.loadRedis(ctx, key, target); ok {
		c.recordCacheRequest(kind, "redis", "hit")
		c.storeLocalIfCurrent(ctx, key, data, generation)
		return nil
	}
	c.recordCacheRequest(kind, "redis", "miss")

	localLock := c.localLocks.For(key)
	localLock.Lock()
	defer localLock.Unlock()

	if c.loadLocalUnlocked(key, target) {
		c.recordCacheRequest(kind, "local_after_lock", "hit")
		return nil
	}
	if data, generation, ok := c.loadRedis(ctx, key, target); ok {
		c.recordCacheRequest(kind, "redis_after_lock", "hit")
		c.storeLocalIfCurrentUnlocked(ctx, key, data, generation)
		return nil
	}

	if c.rebuildLocks == nil {
		generation, err := c.cacheGeneration(ctx, versionKey)
		if err != nil {
			return err
		}
		return c.loadSourceAndStore(ctx, key, versionKey, generation, kind, target, loadSource)
	}
	guard, err := c.rebuildLocks.Acquire(ctx, lockKey, c.options.RebuildLockTTL)
	if errors.Is(err, lock.ErrNotAcquired) {
		observability.IncCounter("ticket_hub_cache_rebuild_lock_contention_total", map[string]string{"kind": kind})
		return c.waitForRedis(ctx, key, kind, target)
	}
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "acquire cache rebuild lock failed", err)
	}
	defer releaseCacheLock(guard)

	if data, generation, ok := c.loadRedis(ctx, key, target); ok {
		c.recordCacheRequest(kind, "redis_after_distributed_lock", "hit")
		c.storeLocalIfCurrentUnlocked(ctx, key, data, generation)
		return nil
	}
	generation, err := c.cacheGeneration(ctx, versionKey)
	if err != nil {
		return err
	}
	return c.loadSourceAndStore(ctx, key, versionKey, generation, kind, target, loadSource)
}

func (c *ProgramQueryCache) loadSourceAndStore(ctx context.Context, key string, versionKey string, generation string, kind string, target any, loadSource func() (any, error)) error {
	startedAt := time.Now()
	value, err := loadSource()
	observability.ObserveHistogram("ticket_hub_cache_rebuild_duration_seconds", map[string]string{"kind": kind}, time.Since(startedAt).Seconds())
	if err != nil {
		return err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encode cache value failed", err)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "decode cache value failed", err)
	}
	data, err := json.Marshal(cacheEnvelope{Generation: generation, Payload: payload})
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encode cache envelope failed", err)
	}
	c.recordCacheRequest(kind, "mysql", "hit")
	redisTTL := withJitter(c.options.RedisTTL)
	stored, err := storeProgramCacheScript.Run(ctx, c.client, []string{versionKey, key}, generation, data, redisTTL.Milliseconds()).Int64()
	if err != nil {
		observability.IncCounter("ticket_hub_cache_store_errors_total", map[string]string{"kind": kind, "level": "redis"})
		return nil
	}
	if stored == 0 {
		observability.IncCounter("ticket_hub_cache_store_rejected_total", map[string]string{"kind": kind, "reason": "version_changed"})
		return nil
	}
	c.storeLocalUnlocked(key, data)
	return nil
}

func (c *ProgramQueryCache) cacheGeneration(ctx context.Context, versionKey string) (string, error) {
	generation, err := c.client.Get(ctx, versionKey).Result()
	if errors.Is(err, redislib.Nil) {
		return "0", nil
	}
	if err != nil {
		return "", therrors.Wrap(therrors.CodeInfrastructure, "read cache generation failed", err)
	}
	return generation, nil
}

func (c *ProgramQueryCache) waitForRedis(ctx context.Context, key string, kind string, target any) error {
	timer := time.NewTimer(c.options.RebuildWait)
	defer timer.Stop()
	ticker := time.NewTicker(c.options.RebuildPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			observability.IncCounter("ticket_hub_cache_rebuild_wait_timeout_total", map[string]string{"kind": kind})
			return therrors.New(therrors.CodeInfrastructure, "cache rebuild wait timed out")
		case <-ticker.C:
			if data, generation, ok := c.loadRedis(ctx, key, target); ok {
				c.recordCacheRequest(kind, "redis_after_wait", "hit")
				c.storeLocalIfCurrentUnlocked(ctx, key, data, generation)
				return nil
			}
		}
	}
}

func (c *ProgramQueryCache) loadLocal(key string, target any) bool {
	localLock := c.localLocks.For(key)
	localLock.RLock()
	defer localLock.RUnlock()
	return c.loadLocalUnlocked(key, target)
}

func (c *ProgramQueryCache) loadLocalUnlocked(key string, target any) bool {
	if c.local == nil {
		return false
	}
	data, ok := c.local.Get(key)
	if !ok {
		return false
	}
	if _, ok := decodeCacheEnvelope(data, target); !ok {
		c.local.Delete(key)
		return false
	}
	return true
}

func (c *ProgramQueryCache) storeLocalIfCurrent(ctx context.Context, key string, data []byte, generation string) {
	localLock := c.localLocks.For(key)
	localLock.Lock()
	defer localLock.Unlock()
	c.storeLocalIfCurrentUnlocked(ctx, key, data, generation)
}

func (c *ProgramQueryCache) storeLocalIfCurrentUnlocked(ctx context.Context, key string, data []byte, generation string) {
	current, err := c.cacheGeneration(ctx, c.versionKeyFromCacheKey(key))
	if err != nil || current != generation {
		observability.IncCounter("ticket_hub_cache_store_rejected_total", map[string]string{"kind": cacheKind(key), "reason": "version_changed_before_local_store"})
		return
	}
	c.storeLocalUnlocked(key, data)
}

func (c *ProgramQueryCache) storeLocalUnlocked(key string, data []byte) {
	if c.local != nil && !c.local.Set(key, data, withJitter(c.options.LocalTTL)) {
		observability.IncCounter("ticket_hub_cache_store_errors_total", map[string]string{"level": "local", "kind": cacheKind(key)})
	}
}

func (c *ProgramQueryCache) loadRedis(ctx context.Context, key string, target any) ([]byte, string, bool) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, "", false
	}
	generation, ok := decodeCacheEnvelope(data, target)
	if !ok {
		_ = c.client.Del(ctx, key).Err()
		return nil, "", false
	}
	return data, generation, true
}

func (c *ProgramQueryCache) invalidateLocalKey(key string) {
	if c.local == nil {
		return
	}
	localLock := c.localLocks.For(key)
	localLock.Lock()
	c.local.Delete(key)
	localLock.Unlock()
}

func (c *ProgramQueryCache) cacheKeys(programID int64) []string {
	return []string{
		c.keys.Build("cache", "program", programID),
		c.keys.Build("cache", "program-categories", programID),
	}
}

func (c *ProgramQueryCache) invalidationChannel() string {
	return c.keys.Build("cache", "invalidate")
}

func (c *ProgramQueryCache) versionKeyFromCacheKey(key string) string {
	parts := strings.Split(key, ":")
	return c.keys.Build("cache", "version", parts[len(parts)-1])
}

func (c *ProgramQueryCache) recordCacheRequest(kind string, level string, result string) {
	observability.IncCounter("ticket_hub_cache_requests_total", map[string]string{"kind": kind, "level": level, "result": result})
}

func releaseCacheLock(guard lock.Lock) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = guard.Release(ctx)
}

func withJitter(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	return ttl + time.Duration(rand.Int63n(int64(ttl/5)+1))
}

func cacheKind(key string) string {
	if strings.Contains(key, "program-categories") {
		return "program_categories"
	}
	return "program"
}

func decodeCacheEnvelope(data []byte, target any) (string, bool) {
	var envelope cacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Generation == "" || len(envelope.Payload) == 0 {
		return "", false
	}
	if err := json.Unmarshal(envelope.Payload, target); err != nil {
		return "", false
	}
	return envelope.Generation, true
}
