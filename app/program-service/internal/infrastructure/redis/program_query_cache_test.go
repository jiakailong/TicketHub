package redis

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
	"tickethub/pkg/lock"
)

func TestProgramQueryCacheUsesLocalRedisAndSourceLevels(t *testing.T) {
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	defer client.Close()
	source := &countingProgramSource{
		program:    program.Program{ID: 101, Title: "Database Program", City: "Shanghai"},
		categories: []program.TicketCategory{{ID: 201, ProgramID: 101, Name: "VIP", PriceCent: 88000}},
	}
	queryCache, local := newTestProgramQueryCache(t, source, client, "test:levels")
	ctx := context.Background()

	item, err := queryCache.FindProgram(ctx, 101)
	if err != nil || item.Title != "Database Program" || source.programCalls.Load() != 1 {
		t.Fatalf("database load: item=%+v calls=%d err=%v", item, source.programCalls.Load(), err)
	}
	local.Wait()
	if err := client.Del(ctx, "test:levels:cache:program:101").Err(); err != nil {
		t.Fatal(err)
	}
	item, err = queryCache.FindProgram(ctx, 101)
	if err != nil || item.Title != "Database Program" || source.programCalls.Load() != 1 {
		t.Fatalf("local load: item=%+v calls=%d err=%v", item, source.programCalls.Load(), err)
	}

	queryCache.InvalidateLocal(101)
	local.Wait()
	redisItem := program.Program{ID: 101, Title: "Redis Program", City: "Shanghai"}
	payload, err := json.Marshal(redisItem)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cacheEnvelope{Generation: "0", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, "test:levels:cache:program:101", data, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	item, err = queryCache.FindProgram(ctx, 101)
	if err != nil || item.Title != "Redis Program" || source.programCalls.Load() != 1 {
		t.Fatalf("redis load: item=%+v calls=%d err=%v", item, source.programCalls.Load(), err)
	}

	categories, err := queryCache.ListTicketCategories(ctx, 101)
	if err != nil || len(categories) != 1 || categories[0].Name != "VIP" || source.categoryCalls.Load() != 1 {
		t.Fatalf("category load: items=%+v calls=%d err=%v", categories, source.categoryCalls.Load(), err)
	}
	local.Wait()
	if err := client.Del(ctx, "test:levels:cache:program-categories:101").Err(); err != nil {
		t.Fatal(err)
	}
	categories, err = queryCache.ListTicketCategories(ctx, 101)
	if err != nil || len(categories) != 1 || source.categoryCalls.Load() != 1 {
		t.Fatalf("local category load: items=%+v calls=%d err=%v", categories, source.categoryCalls.Load(), err)
	}
}

func TestProgramQueryCacheSinglePodPreventsBreakdown(t *testing.T) {
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	defer client.Close()
	source := &countingProgramSource{
		program: program.Program{ID: 102, Title: "Hot Program"},
		delay:   50 * time.Millisecond,
	}
	queryCache, _ := newTestProgramQueryCache(t, source, client, "test:single-pod")

	runConcurrentProgramQueries(t, 100, func(int) error {
		_, err := queryCache.FindProgram(context.Background(), 102)
		return err
	})
	if calls := source.programCalls.Load(); calls != 1 {
		t.Fatalf("source calls = %d, want 1", calls)
	}
}

func TestProgramQueryCacheDistributedLockPreventsCrossPodBreakdown(t *testing.T) {
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	defer client.Close()
	source := &countingProgramSource{
		program: program.Program{ID: 103, Title: "Cross Pod Program"},
		delay:   80 * time.Millisecond,
	}
	first, _ := newTestProgramQueryCache(t, source, client, "test:cross-pod")
	second, _ := newTestProgramQueryCache(t, source, client, "test:cross-pod")

	runConcurrentProgramQueries(t, 100, func(index int) error {
		queryCache := first
		if index%2 == 1 {
			queryCache = second
		}
		_, err := queryCache.FindProgram(context.Background(), 103)
		return err
	})
	if calls := source.programCalls.Load(); calls != 1 {
		t.Fatalf("source calls = %d, want 1", calls)
	}
}

func TestProgramQueryCacheBroadcastInvalidatesOtherPodLocalCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	defer client.Close()
	source := &countingProgramSource{program: program.Program{ID: 104, Title: "Before Change"}}
	first, _ := newTestProgramQueryCache(t, source, client, "test:invalidation")
	second, secondLocal := newTestProgramQueryCache(t, source, client, "test:invalidation")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	NewCacheInvalidationSubscriber(client, second).Start(ctx)
	waitForSubscriber(t, client, second.invalidationChannel())

	if _, err := second.FindProgram(ctx, 104); err != nil {
		t.Fatal(err)
	}
	secondLocal.Wait()
	source.program = program.Program{ID: 104, Title: "After Change"}
	if err := first.Invalidate(ctx, 104); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool {
		item, err := second.FindProgram(ctx, 104)
		return err == nil && item.Title == "After Change"
	})
}

func TestProgramQueryCacheRejectsStaleValueAfterConcurrentInvalidation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	defer client.Close()
	source := newBlockingProgramSource(program.Program{ID: 106, Title: "Old Value"})
	loader, _ := newTestProgramQueryCache(t, source, client, "test:generation")
	invalidator, _ := newTestProgramQueryCache(t, source, client, "test:generation")

	loaded := make(chan program.Program, 1)
	errors := make(chan error, 1)
	go func() {
		item, err := loader.FindProgram(context.Background(), 106)
		loaded <- item
		errors <- err
	}()
	<-source.started
	source.SetProgram(program.Program{ID: 106, Title: "New Value"})
	if err := invalidator.Invalidate(context.Background(), 106); err != nil {
		t.Fatal(err)
	}
	close(source.release)
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	if item := <-loaded; item.Title != "Old Value" {
		t.Fatalf("in-flight query = %+v", item)
	}

	item, err := loader.FindProgram(context.Background(), 106)
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "New Value" {
		t.Fatalf("cached stale item after invalidation: %+v", item)
	}
}

func newTestProgramQueryCache(t *testing.T, source programQuerySource, client *redislib.Client, prefix string) (*ProgramQueryCache, *cache.RistrettoLocal) {
	t.Helper()
	local, err := cache.NewRistrettoLocal(cache.RistrettoConfig{NumCounters: 1_000, MaxCost: 1 << 20, BufferItems: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Close)
	queryCache := NewProgramQueryCache(
		source,
		client,
		cache.NewKeyBuilder(prefix),
		local,
		cache.NewStripedRWMutex(64),
		lock.NewRedisLocker(client),
		QueryCacheOptions{
			LocalTTL:       time.Minute,
			RedisTTL:       time.Minute,
			RebuildLockTTL: 2 * time.Second,
			RebuildWait:    time.Second,
			RebuildPoll:    5 * time.Millisecond,
		},
	)
	return queryCache, local
}

func runConcurrentProgramQueries(t *testing.T, count int, query func(index int) error) {
	t.Helper()
	start := make(chan struct{})
	errors := make(chan error, count)
	var wait sync.WaitGroup
	wait.Add(count)
	for index := 0; index < count; index++ {
		go func(index int) {
			defer wait.Done()
			<-start
			errors <- query(index)
		}(index)
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func waitForSubscriber(t *testing.T, client *redislib.Client, channel string) {
	t.Helper()
	eventually(t, time.Second, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), channel).Result()
		return err == nil && counts[channel] > 0
	})
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

type countingProgramSource struct {
	program       program.Program
	categories    []program.TicketCategory
	delay         time.Duration
	programCalls  atomic.Int64
	categoryCalls atomic.Int64
}

type blockingProgramSource struct {
	mu      sync.RWMutex
	program program.Program
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingProgramSource(item program.Program) *blockingProgramSource {
	return &blockingProgramSource{program: item, started: make(chan struct{}), release: make(chan struct{})}
}

func (s *blockingProgramSource) SetProgram(item program.Program) {
	s.mu.Lock()
	s.program = item
	s.mu.Unlock()
}

func (s *blockingProgramSource) SearchPrograms(context.Context, string, string, int, int) ([]program.Program, error) {
	return nil, nil
}

func (s *blockingProgramSource) FindProgram(context.Context, int64) (program.Program, error) {
	s.mu.RLock()
	item := s.program
	s.mu.RUnlock()
	s.once.Do(func() { close(s.started) })
	<-s.release
	return item, nil
}

func (s *blockingProgramSource) ListTicketCategories(context.Context, int64) ([]program.TicketCategory, error) {
	return nil, nil
}

func (s *blockingProgramSource) ListSeats(context.Context, int64, int64) ([]program.Seat, error) {
	return nil, nil
}

func (s *countingProgramSource) SearchPrograms(context.Context, string, string, int, int) ([]program.Program, error) {
	return []program.Program{s.program}, nil
}

func (s *countingProgramSource) FindProgram(context.Context, int64) (program.Program, error) {
	s.programCalls.Add(1)
	time.Sleep(s.delay)
	return s.program, nil
}

func (s *countingProgramSource) ListTicketCategories(context.Context, int64) ([]program.TicketCategory, error) {
	s.categoryCalls.Add(1)
	time.Sleep(s.delay)
	return append([]program.TicketCategory(nil), s.categories...), nil
}

func (s *countingProgramSource) ListSeats(context.Context, int64, int64) ([]program.Seat, error) {
	return nil, nil
}
