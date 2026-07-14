//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	redisclient "github.com/redis/go-redis/v9"
)

const (
	fixtureProgramID  int64 = 990001
	fixtureCategoryID int64 = 990001
	fixtureSeatID     int64 = 990001
	fixturePriceCent  int64 = 12800
	fixtureTotal      int64 = 10
)

var physicalOrderLocations = []shardLocation{
	{Database: "tickethub_order_0", Table: "orders_0"},
	{Database: "tickethub_order_0", Table: "orders_1"},
	{Database: "tickethub_order_1", Table: "orders_0"},
	{Database: "tickethub_order_1", Table: "orders_1"},
	{Database: "tickethub_order_2", Table: "orders_0"},
	{Database: "tickethub_order_2", Table: "orders_1"},
}

type harness struct {
	t       *testing.T
	baseURL string
	http    *http.Client
	db      *sql.DB
	redis   *redisclient.Client
}

type apiEnvelope[T any] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type int64JSON int64

func (v *int64JSON) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), "\"")
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	*v = int64JSON(parsed)
	return nil
}

type apiFailure struct {
	Status  int
	Code    string
	Message string
}

func (e apiFailure) Error() string {
	return fmt.Sprintf("api request failed: status=%d code=%s message=%s", e.Status, e.Code, e.Message)
}

type shardLocation struct {
	Database string
	Table    string
}

func (l shardLocation) String() string {
	return l.Database + "." + l.Table
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	baseURL := strings.TrimRight(envOrDefault("TICKETHUB_TEST_BASE_URL", "http://127.0.0.1:9080"), "/")
	dsn := envRequired(t, "TICKETHUB_TEST_MYSQL_DSN")
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		t.Fatalf("ping MySQL: %v", err)
	}

	redisAddress := envOrDefault("TICKETHUB_TEST_REDIS_ADDR", "127.0.0.1:6379")
	redis := redisclient.NewClient(&redisclient.Options{Addr: redisAddress})
	if err := redis.Ping(ctx).Err(); err != nil {
		database.Close()
		redis.Close()
		t.Fatalf("ping Redis: %v", err)
	}

	h := &harness{
		t:       t,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		db:      database,
		redis:   redis,
	}
	t.Cleanup(func() {
		_ = redis.Close()
		_ = database.Close()
	})
	return h
}

func (h *harness) checkGateway(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway health returned %s", resp.Status)
	}
	return nil
}

func callAPI[T any](ctx context.Context, h *harness, method string, path string, token string, payload any) (T, error) {
	h.t.Helper()
	var zero T
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return zero, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, body)
	if err != nil {
		return zero, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if method == http.MethodPost && path == "/api/orders" {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("integration-%d", time.Now().UnixNano()))
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return zero, err
	}
	var envelope apiEnvelope[T]
	if err := json.Unmarshal(data, &envelope); err != nil {
		return zero, fmt.Errorf("decode %s %s response: %w; body=%s", method, path, err, data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != "OK" {
		return zero, apiFailure{Status: resp.StatusCode, Code: envelope.Code, Message: envelope.Message}
	}
	return envelope.Data, nil
}

func (h *harness) prepareFixture(ctx context.Context) error {
	if err := h.cleanupFixture(ctx); err != nil {
		return err
	}
	statements := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO tickethub_program.programs (id, title, city, place, show_time, status, created_at)
VALUES (?, 'TicketHub Integration Event', 'Shanghai', 'Integration Venue', DATE_ADD(CURRENT_TIMESTAMP(3), INTERVAL 30 DAY), 'ON_SALE', CURRENT_TIMESTAMP(3))`,
			args: []any{fixtureProgramID},
		},
		{
			query: `INSERT INTO tickethub_program.ticket_categories (id, program_id, name, price_cent, total, remain, sell_started)
VALUES (?, ?, 'Integration Zone', ?, ?, ?, 1)`,
			args: []any{fixtureCategoryID, fixtureProgramID, fixturePriceCent, fixtureTotal, fixtureTotal},
		},
		{
			query: `INSERT INTO tickethub_program.seats (id, program_id, ticket_category_id, row_code, col_code, price_cent, status)
VALUES (?, ?, ?, 'IT', '01', ?, 'no_sold')`,
			args: []any{fixtureSeatID, fixtureProgramID, fixtureCategoryID, fixturePriceCent},
		},
	}
	for _, statement := range statements {
		if _, err := h.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("prepare integration fixture: %w", err)
		}
	}
	return h.redis.Set(ctx, inventoryKey(), fixtureTotal, 0).Err()
}

func (h *harness) cleanupFixture(ctx context.Context) error {
	orderNumbers := make(map[int64]struct{})
	for _, location := range physicalOrderLocations {
		query := fmt.Sprintf("SELECT order_number FROM `%s`.`%s` WHERE program_id = ?", location.Database, location.Table)
		rows, err := h.db.QueryContext(ctx, query, fixtureProgramID)
		if err != nil {
			return fmt.Errorf("query fixture orders from %s: %w", location, err)
		}
		for rows.Next() {
			var orderNumber int64
			if err := rows.Scan(&orderNumber); err != nil {
				rows.Close()
				return err
			}
			orderNumbers[orderNumber] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		deleteQuery := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE program_id = ?", location.Database, location.Table)
		if _, err := h.db.ExecContext(ctx, deleteQuery, fixtureProgramID); err != nil {
			return fmt.Errorf("delete fixture orders from %s: %w", location, err)
		}
	}

	for orderNumber := range orderNumbers {
		if _, err := h.db.ExecContext(ctx, "DELETE FROM tickethub_pay.payments WHERE order_number = ?", orderNumber); err != nil {
			return err
		}
		if _, err := h.db.ExecContext(ctx, "DELETE FROM tickethub_pay.refunds WHERE order_number = ?", orderNumber); err != nil {
			return err
		}
		if err := h.redis.Del(ctx,
			fmt.Sprintf("tickethub:program:order-lock:%d", orderNumber),
			fmt.Sprintf("tickethub:program:order-rollback:%d", orderNumber),
			fmt.Sprintf("tickethub:program:order-release:%d", orderNumber),
		).Err(); err != nil {
			return err
		}
	}

	statements := []string{
		"DELETE FROM tickethub_order.discard_orders WHERE program_id = ?",
		"DELETE FROM tickethub_program.program_records WHERE program_id = ?",
		"DELETE FROM tickethub_program.seats WHERE program_id = ?",
		"DELETE FROM tickethub_program.ticket_categories WHERE program_id = ?",
		"DELETE FROM tickethub_program.programs WHERE id = ?",
	}
	for _, statement := range statements {
		if _, err := h.db.ExecContext(ctx, statement, fixtureProgramID); err != nil {
			return err
		}
	}
	if err := h.redis.Del(ctx, inventoryKey(), seatKey()).Err(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, envOrDefault("TICKETHUB_TEST_ES_URL", "http://127.0.0.1:9200")+"/tickethub_programs/_doc/990001", nil)
	if err == nil {
		if response, requestErr := h.http.Do(request); requestErr == nil {
			response.Body.Close()
		}
	}
	return nil
}

func (h *harness) orderExists(ctx context.Context, location shardLocation, orderNumber int64) (bool, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s` WHERE order_number = ?", location.Database, location.Table)
	var count int
	if err := h.db.QueryRowContext(ctx, query, orderNumber).Scan(&count); err != nil {
		return false, err
	}
	return count == 1, nil
}

func (h *harness) chooseMigrationTarget(mappings []shardMapping, current shardLocation) (shardLocation, error) {
	assigned := make(map[string]struct{})
	for _, mapping := range mappings {
		assigned[shardLocation{Database: mapping.PhysicalDB, Table: mapping.PhysicalTable}.String()] = struct{}{}
		if mapping.ShadowDB != "" && mapping.ShadowTable != "" {
			assigned[shardLocation{Database: mapping.ShadowDB, Table: mapping.ShadowTable}.String()] = struct{}{}
		}
	}
	candidates := append([]shardLocation(nil), physicalOrderLocations...)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Database == candidates[j].Database {
			return candidates[i].Table < candidates[j].Table
		}
		return candidates[i].Database > candidates[j].Database
	})
	for _, candidate := range candidates {
		if candidate != current {
			if _, exists := assigned[candidate.String()]; !exists {
				return candidate, nil
			}
		}
	}
	return shardLocation{}, fmt.Errorf("no free physical shard is available")
}

func (h *harness) restoreMapping(ctx context.Context, virtualShard int32, source shardLocation, taskID int64) error {
	if taskID > 0 {
		_, _ = h.db.ExecContext(ctx, "UPDATE tickethub_migrate.migration_tasks SET status = 'PAUSED' WHERE id = ? AND status IN ('PENDING', 'RUNNING')", taskID)
	}
	_, err := h.db.ExecContext(ctx, `UPDATE tickethub_migrate.shard_mappings
SET physical_db = ?, physical_table = ?, shadow_db = NULL, shadow_table = NULL,
    write_mode = 'PRIMARY_ONLY', version = version + 1, updated_at = CURRENT_TIMESTAMP(3)
WHERE virtual_shard = ?`, source.Database, source.Table, virtualShard)
	if err != nil {
		return err
	}
	if taskID > 0 {
		_, err = h.db.ExecContext(ctx, "DELETE FROM tickethub_migrate.migration_tasks WHERE id = ?", taskID)
	}
	return err
}

func waitFor(ctx context.Context, interval time.Duration, check func() (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func inventoryKey() string {
	return fmt.Sprintf("tickethub:program:inventory:%d:%d", fixtureProgramID, fixtureCategoryID)
}

func seatKey() string {
	return fmt.Sprintf("tickethub:program:seat:%d:%d", fixtureProgramID, fixtureSeatID)
}

func envOrDefault(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envRequired(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required for integration tests", name)
	}
	return value
}
