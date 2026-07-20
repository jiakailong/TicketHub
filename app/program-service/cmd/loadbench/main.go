package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	redislib "github.com/redis/go-redis/v9"

	programapp "tickethub/app/program-service/internal/application"
	"tickethub/app/program-service/internal/domain/program"
	programmysql "tickethub/app/program-service/internal/infrastructure/mysql"
	programredis "tickethub/app/program-service/internal/infrastructure/redis"
	"tickethub/pkg/cache"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/idgen"
	"tickethub/pkg/lock"
	"tickethub/pkg/mq"
)

type result struct {
	Scenario        string  `json:"scenario"`
	Mode            string  `json:"mode,omitempty"`
	Requests        int     `json:"requests"`
	Concurrency     int     `json:"concurrency"`
	Successes       int64   `json:"successes"`
	Errors          int64   `json:"errors"`
	DurationSec     float64 `json:"duration_seconds"`
	ThroughputRPS   float64 `json:"throughput_rps"`
	P50MS           float64 `json:"p50_ms"`
	P95MS           float64 `json:"p95_ms"`
	P99MS           float64 `json:"p99_ms"`
	SourceQueries   int64   `json:"mysql_source_queries,omitempty"`
	RedisRemaining  int64   `json:"redis_remaining"`
	OrdersPersisted int64   `json:"orders_persisted,omitempty"`
	OrdersDiscarded int64   `json:"orders_discarded,omitempty"`
	AsyncDrainSec   float64 `json:"async_drain_seconds,omitempty"`
	BaseUserID      int64   `json:"base_user_id,omitempty"`
	RunID           string  `json:"run_id"`
}

type countingRepository struct {
	programmysql.ProgramRepository
	queries atomic.Int64
}

func (r *countingRepository) FindProgram(ctx context.Context, id int64) (program.Program, error) {
	r.queries.Add(1)
	return r.ProgramRepository.FindProgram(ctx, id)
}

func (r *countingRepository) ListTicketCategories(ctx context.Context, id int64) ([]program.TicketCategory, error) {
	r.queries.Add(1)
	return r.ProgramRepository.ListTicketCategories(ctx, id)
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: loadbench <cache|order> [flags]")
	}
	switch os.Args[1] {
	case "cache":
		runCache(os.Args[2:])
	case "order":
		runOrder(os.Args[2:])
	default:
		fatalf("unknown scenario %q", os.Args[1])
	}
}

func runCache(args []string) {
	fs := flag.NewFlagSet("cache", flag.ExitOnError)
	mode := fs.String("mode", "multilevel", "mysql, redis, or multilevel")
	requests := fs.Int("requests", 50000, "request count")
	concurrency := fs.Int("concurrency", 200, "worker count")
	programID := fs.Int64("program", 10001, "program id")
	cold := fs.Bool("cold", false, "start with an empty cache and include source rebuilds")
	runID := fs.String("run-id", time.Now().Format("20060102-150405"), "run identifier")
	dsn := fs.String("dsn", defaultDSN("tickethub_program"), "mysql dsn")
	redisAddr := fs.String("redis", "127.0.0.1:6379", "redis address")
	_ = fs.Parse(args)
	if *requests <= 0 || *concurrency <= 0 {
		fatalf("requests and concurrency must be positive")
	}

	ctx := context.Background()
	database := openDB(ctx, *dsn)
	defer database.Close()
	client := redislib.NewClient(&redislib.Options{Addr: *redisAddr})
	defer client.Close()
	prefix := "tickethub:load:cache:" + *runID
	deletePrefix(ctx, client, prefix)

	source := &countingRepository{ProgramRepository: programmysql.NewProgramRepository(database)}
	var repository programapp.ProgramSearchRepository = source
	var local cache.Local
	if *mode == "redis" || *mode == "multilevel" {
		if *mode == "multilevel" {
			var err error
			local, err = cache.NewRistrettoLocal(cache.RistrettoConfig{NumCounters: 100000, MaxCost: 64 << 20, BufferItems: 64})
			if err != nil {
				fatalf("create ristretto: %v", err)
			}
			defer local.Close()
		}
		repository = programredis.NewProgramQueryCache(
			source, client, cache.NewKeyBuilder(prefix), local, cache.NewStripedRWMutex(256),
			lock.NewRedisLocker(client), programredis.QueryCacheOptions{
				LocalTTL: 45 * time.Second, RedisTTL: 5 * time.Minute,
				RebuildLockTTL: 5 * time.Second, RebuildWait: 2 * time.Second, RebuildPoll: 5 * time.Millisecond,
			},
		)
	} else if *mode != "mysql" {
		fatalf("unsupported cache mode %q", *mode)
	}
	queries := programapp.NewProgramQueryService(repository)
	if !*cold {
		if _, err := queries.Detail(ctx, *programID, 0); err != nil {
			fatalf("warm detail: %v", err)
		}
		if localCache, ok := local.(*cache.RistrettoLocal); ok {
			localCache.Wait()
		}
		source.queries.Store(0)
	}
	latencies, successes, failures, elapsed := execute(*requests, *concurrency, func(index int) error {
		_, err := queries.Detail(ctx, *programID, 0)
		return err
	})
	emit(resultFrom("cache_detail", *mode, *runID, *requests, *concurrency, successes, failures, elapsed, latencies, source.queries.Load(), 0))
}

func runOrder(args []string) {
	fs := flag.NewFlagSet("order", flag.ExitOnError)
	requests := fs.Int("requests", 100000, "request count")
	concurrency := fs.Int("concurrency", 300, "worker count")
	programID := fs.Int64("program", 10001, "program id")
	categoryID := fs.Int64("category", 2, "ticket category id")
	runID := fs.String("run-id", time.Now().Format("20060102-150405"), "run identifier")
	dsn := fs.String("dsn", defaultDSN("tickethub_program"), "mysql dsn")
	orderDSN := fs.String("order-dsn", defaultDSN("tickethub_order"), "order mysql dsn")
	redisAddr := fs.String("redis", "127.0.0.1:6379", "redis address")
	broker := fs.String("broker", "127.0.0.1:9094", "kafka broker")
	drainTimeout := fs.Duration("drain-timeout", 10*time.Minute, "maximum wait for async order persistence")
	_ = fs.Parse(args)
	if *requests <= 0 || *concurrency <= 0 {
		fatalf("requests and concurrency must be positive")
	}

	ctx := context.Background()
	database := openDB(ctx, *dsn)
	defer database.Close()
	client := redislib.NewClient(&redislib.Options{Addr: *redisAddr, PoolSize: *concurrency + 20})
	defer client.Close()
	prefix := "tickethub:load:order:" + *runID
	deletePrefix(ctx, client, prefix)
	keys := cache.NewKeyBuilder(prefix)
	inventoryKey := keys.Build("inventory", *programID, *categoryID)
	if err := client.Set(ctx, inventoryKey, *requests, 0).Err(); err != nil {
		fatalf("seed inventory: %v", err)
	}

	ids, err := idgen.NewSnowflake(12)
	if err != nil {
		fatalf("create id generator: %v", err)
	}
	publisher := mq.NewKafkaProducer([]string{*broker}, "loadbench-"+*runID)
	defer publisher.Close()
	repository := programmysql.NewProgramRepository(database)
	usecase := programapp.NewCreateOrderUsecase(
		idgen.NewOrderNumberGenerator(ids), ids,
		programredis.NewInventoryLocker(keys, cache.NewRedisLuaExecutor(client), 3600),
		publisher, programapp.NewProgramPricingService(repository),
	).WithIdempotency(programredis.NewIdempotencyStore(client, keys))

	baseUser := time.Now().Unix()%1_000_000*1_000_000 + 20_000_000
	latencies, successes, failures, elapsed := execute(*requests, *concurrency, func(index int) error {
		userID := baseUser + int64(index)
		_, err := usecase.CreateAsync(ctx, program.CreateOrderCommand{
			RequestID: fmt.Sprintf("%s-%d", *runID, index), UserID: userID,
			ProgramID: *programID, TicketCategoryID: *categoryID, TicketUserIDs: []int64{userID},
		})
		return err
	})
	remaining, err := client.Get(ctx, inventoryKey).Int64()
	if err != nil {
		fatalf("read remaining inventory: %v", err)
	}
	orderDB := openDB(ctx, *orderDSN)
	defer orderDB.Close()
	drainStarted := time.Now()
	var persisted int64
	var discarded int64
	deadline := time.Now().Add(*drainTimeout)
	for time.Now().Before(deadline) {
		if err := orderDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM orders WHERE user_id BETWEEN ? AND ?", baseUser, baseUser+int64(*requests)-1).Scan(&persisted); err != nil {
			fatalf("count persisted orders: %v", err)
		}
		if persisted >= successes {
			break
		}
		if err := orderDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM discard_orders WHERE user_id BETWEEN ? AND ?", baseUser, baseUser+int64(*requests)-1).Scan(&discarded); err != nil {
			fatalf("count discarded orders: %v", err)
		}
		if persisted+discarded >= successes {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	value := resultFrom("async_create_order", "", *runID, *requests, *concurrency, successes, failures, elapsed, latencies, 0, remaining)
	value.OrdersPersisted = persisted
	value.OrdersDiscarded = discarded
	value.AsyncDrainSec = time.Since(drainStarted).Seconds()
	value.BaseUserID = baseUser
	emit(value)
}

func execute(requests int, concurrency int, operation func(int) error) ([]time.Duration, int64, int64, time.Duration) {
	jobs := make(chan int, concurrency)
	latencies := make([]time.Duration, requests)
	var successes atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	started := time.Now()
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				requestStarted := time.Now()
				err := operation(index)
				latencies[index] = time.Since(requestStarted)
				if err != nil {
					failures.Add(1)
				} else {
					successes.Add(1)
				}
			}
		}()
	}
	for index := 0; index < requests; index++ {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return latencies, successes.Load(), failures.Load(), time.Since(started)
}

func resultFrom(scenario string, mode string, runID string, requests int, concurrency int, successes int64, failures int64, elapsed time.Duration, latencies []time.Duration, sourceQueries int64, remaining int64) result {
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(value float64) float64 {
		index := int(float64(len(latencies)-1) * value)
		return float64(latencies[index].Microseconds()) / 1000
	}
	return result{
		Scenario: scenario, Mode: mode, Requests: requests, Concurrency: concurrency,
		Successes: successes, Errors: failures, DurationSec: elapsed.Seconds(), ThroughputRPS: float64(requests) / elapsed.Seconds(),
		P50MS: percentile(.50), P95MS: percentile(.95), P99MS: percentile(.99), SourceQueries: sourceQueries,
		RedisRemaining: remaining, RunID: runID,
	}
}

func openDB(ctx context.Context, dsn string) *sql.DB {
	database, err := db.OpenMySQL(ctx, config.MySQLConfig{DSN: dsn, MaxOpenConns: 100, MaxIdleConns: 50})
	if err != nil {
		fatalf("open mysql: %v", err)
	}
	return database
}

func defaultDSN(database string) string {
	user := getenv("TICKETHUB_MYSQL_USER", "tickethub")
	password := getenv("TICKETHUB_MYSQL_PASSWORD", "tickethub")
	host := getenv("TICKETHUB_MYSQL_HOST", "127.0.0.1:3306")
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&loc=Local&charset=utf8mb4", user, password, host, database)
}

func getenv(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func deletePrefix(ctx context.Context, client *redislib.Client, prefix string) {
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, prefix+"*", 1000).Result()
		if err != nil {
			fatalf("scan redis prefix: %v", err)
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				fatalf("delete redis prefix: %v", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func emit(value result) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fatalf("encode result: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
