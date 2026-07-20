package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	redislib "github.com/redis/go-redis/v9"

	orderapp "tickethub/app/order-service/internal/application"
	orderdomain "tickethub/app/order-service/internal/domain/order"
	ordermysql "tickethub/app/order-service/internal/infrastructure/mysql"
	"tickethub/app/order-service/internal/infrastructure/rpc"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/delayqueue"
	"tickethub/pkg/lock"
)

type result struct {
	Scenario             string  `json:"scenario"`
	RunID                string  `json:"run_id"`
	Orders               int     `json:"orders"`
	Workers              int     `json:"workers"`
	DurationSec          float64 `json:"duration_seconds"`
	ThroughputOPS        float64 `json:"throughput_orders_per_second"`
	Canceled             int     `json:"canceled"`
	Paid                 int     `json:"paid"`
	NoPay                int     `json:"no_pay"`
	Errors               int64   `json:"errors"`
	WorkerRetries        int64   `json:"worker_retries"`
	PaymentConflicts     int64   `json:"payment_conflicts"`
	InventoryExpected    int64   `json:"inventory_expected"`
	InventoryActual      int64   `json:"inventory_actual"`
	RedeliveryRecovered  bool    `json:"redelivery_recovered"`
	DuplicateCloseStable bool    `json:"duplicate_close_stable"`
	OrderMin             int64   `json:"order_min"`
	OrderMax             int64   `json:"order_max"`
}

func main() {
	fs := flag.NewFlagSet("delay", flag.ExitOnError)
	orders := fs.Int("orders", 5000, "number of orders")
	workers := fs.Int("workers", 8, "parallel cancel workers")
	batch := fs.Int("batch", 128, "claim batch")
	visibility := fs.Duration("visibility", 2*time.Second, "visibility timeout")
	raceOrders := fs.Int("race-orders", 0, "orders that race payment against delayed close")
	runID := fs.String("run-id", time.Now().Format("20060102-150405"), "run identifier")
	dsn := fs.String("dsn", defaultDSN("tickethub_order"), "mysql dsn")
	redisAddr := fs.String("redis", "127.0.0.1:6379", "redis address")
	programGRPC := fs.String("program-grpc", "127.0.0.1:9002", "program service grpc address")
	_ = fs.Parse(os.Args[1:])
	if *orders <= 10 || *workers <= 0 || *batch <= 0 || *raceOrders < 0 || *raceOrders >= *orders {
		fatalf("orders must exceed 10; workers and batch must be positive")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	database := openDB(ctx, *dsn)
	defer database.Close()
	client := redislib.NewClient(&redislib.Options{Addr: *redisAddr, PoolSize: *workers*4 + 20})
	defer client.Close()
	programClient, err := rpc.NewProgramGRPCClient(*programGRPC)
	if err != nil {
		fatalf("connect program service: %v", err)
	}
	defer programClient.Close()

	base := (time.Now().UnixMilli()%1_000_000_000)*1_000_000 + 100_000
	minOrder := base
	maxOrder := base + int64(*orders) - 1
	if _, err := database.ExecContext(ctx, "DELETE FROM orders WHERE order_number BETWEEN ? AND ?", minOrder, maxOrder); err != nil {
		fatalf("cleanup order range: %v", err)
	}
	prefix := "tickethub:load:delay:" + *runID
	queue := delayqueue.NewRedisQueue(client, prefix).WithVisibilityTimeout(*visibility)
	repository := ordermysql.NewOrderRepository(database)
	service := orderapp.NewOrderCommandService(repository, programClient).WithLocker(lock.NewRedisLocker(client), 5*time.Second)
	worker := orderapp.NewCancelOrderWorker(queue, service)
	programID, categoryID := int64(10001), int64(2)
	programPrefix := "tickethub:program"
	inventoryKey := fmt.Sprintf("%s:inventory:%d:%d", programPrefix, programID, categoryID)
	if err := client.Set(ctx, inventoryKey, 0, 0).Err(); err != nil {
		fatalf("seed program inventory: %v", err)
	}

	seedStarted := time.Now()
	for offset := 0; offset < *orders; offset++ {
		orderNumber := minOrder + int64(offset)
		userID := 70_000_000 + int64(offset)
		ticketUserID := 80_000_000 + int64(offset)
		item := orderdomain.New(orderNumber, programID, userID, 88000, time.Now().Add(-time.Minute))
		item.TicketCategoryID = categoryID
		item.TicketUserIDs = []int64{ticketUserID}
		if err := repository.Save(ctx, item); err != nil {
			fatalf("seed order %d: %v", orderNumber, err)
		}
		pipe := client.Pipeline()
		pipe.HSet(ctx, fmt.Sprintf("%s:order-lock:%d", programPrefix, orderNumber), "order_number", orderNumber, "program_id", programID, "ticket_category_id", categoryID)
		pipe.Set(ctx, fmt.Sprintf("%s:ticket-user:%d:%d", programPrefix, programID, ticketUserID), fmt.Sprintf("locked:%d", orderNumber), time.Hour)
		if _, err := pipe.Exec(ctx); err != nil {
			fatalf("seed redis reservation %d: %v", orderNumber, err)
		}
		payload, _ := json.Marshal(map[string]any{
			"order_number": orderNumber, "user_id": userID, "program_id": programID,
			"ticket_category_id": categoryID, "ticket_user_ids": []int64{ticketUserID},
		})
		if err := queue.Enqueue(ctx, delayqueue.Message{
			ID: fmt.Sprint(orderNumber), Topic: orderapp.CancelOrderDelayTopic,
			Payload: payload, AvailableAt: time.Now(),
		}); err != nil {
			fatalf("enqueue order %d: %v", orderNumber, err)
		}
	}
	_ = seedStarted

	// Simulate a process crash: claim one message and deliberately do not ACK it.
	claimed, err := queue.ClaimDue(ctx, orderapp.CancelOrderDelayTopic, time.Now(), 1)
	if err != nil || len(claimed) != 1 {
		fatalf("prepare redelivery: claimed=%d err=%v", len(claimed), err)
	}
	time.Sleep(*visibility + 100*time.Millisecond)

	started := time.Now()
	var failures atomic.Int64
	var paymentConflicts atomic.Int64
	var wg sync.WaitGroup
	var paymentWG sync.WaitGroup
	for offset := 0; offset < *raceOrders; offset++ {
		paymentWG.Add(1)
		go func(offset int) {
			defer paymentWG.Done()
			time.Sleep(time.Duration((offset*37)%500) * time.Millisecond)
			if err := service.MarkPaid(ctx, minOrder+int64(offset), 70_000_000+int64(offset)); err != nil {
				paymentConflicts.Add(1)
			}
		}(offset)
	}
	for index := 0; index < *workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				canceled, paid, noPay := statusCounts(ctx, database, minOrder, maxOrder)
				if canceled+paid >= *orders && noPay == 0 {
					return
				}
				if err := worker.Poll(ctx, *batch); err != nil {
					failures.Add(1)
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
	paymentWG.Wait()
	elapsed := time.Since(started)

	// Repeat close on a sample. The order state and Redis inventory must remain unchanged.
	before, err := client.Get(ctx, inventoryKey).Int64()
	if err != nil {
		fatalf("read inventory before duplicate close: %v", err)
	}
	for repeat := 0; repeat < 5; repeat++ {
		for offset := 0; offset < 10; offset++ {
			if err := service.CloseExpired(ctx, minOrder+int64(offset), 70_000_000+int64(offset)); err != nil {
				failures.Add(1)
			}
		}
	}
	after, err := client.Get(ctx, inventoryKey).Int64()
	if err != nil {
		fatalf("read inventory after duplicate close: %v", err)
	}
	canceled, paid, noPay := statusCounts(ctx, database, minOrder, maxOrder)
	recovered, _ := repository.FindByOrderNumber(ctx, minOrder, 70_000_000)

	emit(result{
		Scenario: "delay_close_idempotency", RunID: *runID, Orders: *orders, Workers: *workers,
		DurationSec: elapsed.Seconds(), ThroughputOPS: float64(*orders) / elapsed.Seconds(),
		Canceled: canceled, Paid: paid, NoPay: noPay, Errors: failures.Load() + paymentConflicts.Load(),
		WorkerRetries: failures.Load(), PaymentConflicts: paymentConflicts.Load(),
		InventoryExpected: int64(canceled), InventoryActual: after,
		RedeliveryRecovered:  recovered.Status != orderdomain.StatusNoPay,
		DuplicateCloseStable: before == after,
		OrderMin:             minOrder, OrderMax: maxOrder,
	})
}

func statusCounts(ctx context.Context, database *sql.DB, minOrder int64, maxOrder int64) (canceled int, paid int, noPay int) {
	rows, err := database.QueryContext(ctx, `SELECT status, COUNT(*) FROM orders WHERE order_number BETWEEN ? AND ? GROUP BY status`, minOrder, maxOrder)
	if err != nil {
		return 0, 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if rows.Scan(&status, &count) != nil {
			continue
		}
		switch orderdomain.Status(status) {
		case orderdomain.StatusCancel:
			canceled = count
		case orderdomain.StatusPaid:
			paid = count
		case orderdomain.StatusNoPay:
			noPay = count
		}
	}
	return canceled, paid, noPay
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
