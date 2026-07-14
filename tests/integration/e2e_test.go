//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

type loginReply struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID int64JSON `json:"id"`
	} `json:"user"`
}

type registerReply struct {
	ID int64JSON `json:"id"`
}

type ticketUserReply struct {
	ID int64JSON `json:"id"`
}

type createOrderReply struct {
	OrderNumber int64JSON `json:"order_number"`
}

type orderReply struct {
	OrderNumber int64JSON `json:"order_number"`
	Status      string    `json:"status"`
	AmountCent  int64     `json:"amount_cent"`
	PaidAt      string    `json:"paid_at"`
}

type orderListReply struct {
	Orders     []orderReply `json:"orders"`
	NextCursor string       `json:"next_cursor"`
}

type migrationTaskReply struct {
	TaskID int64  `json:"task_id"`
	Status string `json:"status"`
}

type shardMappingsReply struct {
	Mappings []shardMapping `json:"shard_mappings"`
}

type shardMapping struct {
	VirtualShard  int32  `json:"virtual_shard"`
	PhysicalDB    string `json:"physical_db"`
	PhysicalTable string `json:"physical_table"`
	ShadowDB      string `json:"shadow_db"`
	ShadowTable   string `json:"shadow_table"`
	WriteMode     string `json:"write_mode"`
	Version       int64  `json:"version"`
}

type reconciliationReply struct {
	InventoryMismatchCount int64                 `json:"inventory_mismatch_count"`
	RepairedInventoryCount int64                 `json:"repaired_inventory_count"`
	InventoryDifferences   []inventoryDifference `json:"inventory_differences"`
}

type inventoryDifference struct {
	TicketCategoryID int64 `json:"ticket_category_id"`
	ExpectedRemain   int64 `json:"expected_remain"`
	MySQLRemain      int64 `json:"mysql_remain"`
	RedisRemain      int64 `json:"redis_remain"`
	Repaired         bool  `json:"repaired"`
}

func TestTicketHubInfrastructureFlow(t *testing.T) {
	if os.Getenv("TICKETHUB_INTEGRATION") != "1" {
		t.Skip("set TICKETHUB_INTEGRATION=1 to run infrastructure integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	h := newHarness(t)
	if err := h.checkGateway(ctx); err != nil {
		t.Fatalf("check APISIX gateway: %v", err)
	}

	var (
		ticketUserID int64
		taskID       int64
		virtualShard int32
		source       shardLocation
	)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if source.Database != "" {
			if err := h.restoreMapping(cleanupCtx, virtualShard, source, taskID); err != nil {
				t.Errorf("restore shard mapping: %v", err)
			}
		}
		if ticketUserID > 0 {
			if _, err := h.db.ExecContext(cleanupCtx, "DELETE FROM tickethub_user.ticket_users WHERE id = ?", ticketUserID); err != nil {
				t.Errorf("delete integration ticket user: %v", err)
			}
		}
		if err := h.cleanupFixture(cleanupCtx); err != nil {
			t.Errorf("cleanup integration fixture: %v", err)
		}
	})
	if err := h.prepareFixture(ctx); err != nil {
		t.Fatalf("prepare fixture: %v", err)
	}

	adminMobile := envRequired(t, "TICKETHUB_TEST_ADMIN_MOBILE")
	adminPassword := envRequired(t, "TICKETHUB_TEST_ADMIN_PASSWORD")
	login := loginAdmin(ctx, t, h, adminMobile, adminPassword)
	if login.AccessToken == "" || login.User.ID <= 0 {
		t.Fatalf("invalid login response: %+v", login)
	}
	if _, err := callAPI[map[string]any](ctx, h, http.MethodGet, "/api/admin/dashboard", login.AccessToken, nil); err != nil {
		t.Fatalf("verify admin role: %v", err)
	}

	ticketUser, err := callAPI[ticketUserReply](ctx, h, http.MethodPost, "/api/users/ticket-users", login.AccessToken, map[string]any{
		"name":           "Integration User",
		"certificate_no": fmt.Sprintf("IT-%d", time.Now().UnixNano()),
		"mobile":         adminMobile,
	})
	if err != nil {
		t.Fatalf("create ticket user: %v", err)
	}
	ticketUserID = int64(ticketUser.ID)

	mappings := loadMappings(ctx, t, h, login.AccessToken)
	virtualShard = int32(login.User.ID & 3)
	current, ok := findMapping(mappings, virtualShard)
	if !ok {
		t.Fatalf("virtual shard %d is not configured", virtualShard)
	}
	source = shardLocation{Database: current.PhysicalDB, Table: current.PhysicalTable}
	target, err := h.chooseMigrationTarget(mappings, source)
	if err != nil {
		t.Fatal(err)
	}

	task, err := callAPI[migrationTaskReply](ctx, h, http.MethodPost, "/api/migrate/tasks", login.AccessToken, map[string]any{
		"virtual_shard": virtualShard,
		"source_shard":  source.String(),
		"target_shard":  target.String(),
		"batch_size":    500,
	})
	if err != nil {
		t.Fatalf("create migration task: %v", err)
	}
	taskID = task.TaskID
	if task.Status != "PENDING" {
		t.Fatalf("migration status = %s, want PENDING", task.Status)
	}

	if err := waitFor(ctx, 250*time.Millisecond, func() (bool, error) {
		mapping, found := findMapping(loadMappings(ctx, t, h, login.AccessToken), virtualShard)
		return found && mapping.WriteMode == "DUAL_WRITE" && mapping.ShadowDB == target.Database && mapping.ShadowTable == target.Table, nil
	}); err != nil {
		t.Fatalf("wait for dual-write mapping: %v", err)
	}
	waitWithContext(ctx, t, 6*time.Second)

	created, err := callAPI[createOrderReply](ctx, h, http.MethodPost, "/api/orders", login.AccessToken, map[string]any{
		"program_id":         fixtureProgramID,
		"ticket_category_id": fixtureCategoryID,
		"seat_ids":           []int64{fixtureSeatID},
		"ticket_user_ids":    []int64{ticketUserID},
	})
	if err != nil {
		t.Fatalf("create asynchronous order: %v", err)
	}
	if created.OrderNumber <= 0 {
		t.Fatalf("order number must be positive, got %d", created.OrderNumber)
	}

	dualWriteCtx, dualWriteCancel := context.WithTimeout(ctx, 8*time.Second)
	defer dualWriteCancel()
	if err := waitFor(dualWriteCtx, 200*time.Millisecond, func() (bool, error) {
		sourceExists, err := h.orderExists(dualWriteCtx, source, int64(created.OrderNumber))
		if err != nil {
			return false, err
		}
		targetExists, err := h.orderExists(dualWriteCtx, target, int64(created.OrderNumber))
		return sourceExists && targetExists, err
	}); err != nil {
		t.Fatalf("order was not dual-written before migration copy delay: %v", err)
	}

	cutoverCtx, cutoverCancel := context.WithTimeout(ctx, 40*time.Second)
	defer cutoverCancel()
	if err := waitFor(cutoverCtx, 500*time.Millisecond, func() (bool, error) {
		mapping, found := findMapping(loadMappings(cutoverCtx, t, h, login.AccessToken), virtualShard)
		return found && mapping.WriteMode == "PRIMARY_ONLY" && mapping.PhysicalDB == target.Database && mapping.PhysicalTable == target.Table, nil
	}); err != nil {
		t.Fatalf("wait for migration cutover: %v", err)
	}

	if _, err := callAPI[map[string]any](ctx, h, http.MethodPost, "/api/payments", login.AccessToken, map[string]any{
		"order_number": created.OrderNumber,
		"amount_cent":  fixturePriceCent,
		"channel":      "mock",
	}); err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if _, err := callAPI[map[string]any](ctx, h, http.MethodPost, "/api/payments/callback", "", map[string]any{
		"order_number": created.OrderNumber,
		"amount_cent":  fixturePriceCent + 1,
		"channel":      "mock",
		"paid":         true,
	}); err == nil {
		t.Fatal("mismatched payment callback was accepted")
	}
	var paymentStatus string
	if err := h.db.QueryRowContext(ctx, "SELECT status FROM tickethub_pay.payments WHERE order_number = ?", created.OrderNumber).Scan(&paymentStatus); err != nil {
		t.Fatalf("read payment after rejected callback: %v", err)
	}
	if paymentStatus != "CREATED" {
		t.Fatalf("rejected callback changed payment status to %s", paymentStatus)
	}
	if _, err := callAPI[map[string]any](ctx, h, http.MethodPost, "/api/payments/callback", "", map[string]any{
		"order_number": created.OrderNumber,
		"amount_cent":  fixturePriceCent,
		"channel":      "mock",
		"paid":         true,
	}); err != nil {
		t.Fatalf("apply payment callback: %v", err)
	}

	paidCtx, paidCancel := context.WithTimeout(ctx, 10*time.Second)
	defer paidCancel()
	if err := waitFor(paidCtx, 250*time.Millisecond, func() (bool, error) {
		order, err := callAPI[orderReply](paidCtx, h, http.MethodGet, fmt.Sprintf("/api/orders?order_number=%d", created.OrderNumber), login.AccessToken, nil)
		return err == nil && order.Status == "PAY" && order.AmountCent == fixturePriceCent && order.PaidAt != "", err
	}); err != nil {
		t.Fatalf("wait for paid order: %v", err)
	}
	paidOrders, err := callAPI[orderListReply](ctx, h, http.MethodGet, "/api/orders?status=PAY&limit=20", login.AccessToken, nil)
	if err != nil {
		t.Fatalf("list paid orders: %v", err)
	}
	paidFound := false
	for _, item := range paidOrders.Orders {
		if item.Status != "PAY" {
			t.Fatalf("status filter returned %+v", item)
		}
		if item.OrderNumber == created.OrderNumber {
			paidFound = true
		}
	}
	if !paidFound {
		t.Fatalf("paid order missing from filtered list: %+v", paidOrders)
	}

	if err := h.redis.Set(ctx, inventoryKey(), fixtureTotal-3, 0).Err(); err != nil {
		t.Fatalf("inject Redis inventory mismatch: %v", err)
	}
	readOnly := reconcile(ctx, t, h, login.AccessToken, false)
	if readOnly.InventoryMismatchCount != 1 || len(readOnly.InventoryDifferences) != 1 {
		t.Fatalf("unexpected read-only reconciliation: %+v", readOnly)
	}
	difference := readOnly.InventoryDifferences[0]
	if difference.ExpectedRemain != fixtureTotal-1 || difference.MySQLRemain != fixtureTotal || difference.RedisRemain != fixtureTotal-3 {
		t.Fatalf("unexpected inventory difference: %+v", difference)
	}

	repaired := reconcile(ctx, t, h, login.AccessToken, true)
	if repaired.RepairedInventoryCount != 1 || len(repaired.InventoryDifferences) != 1 || !repaired.InventoryDifferences[0].Repaired {
		t.Fatalf("inventory repair was not reported: %+v", repaired)
	}
	verified := reconcile(ctx, t, h, login.AccessToken, false)
	if verified.InventoryMismatchCount != 0 {
		t.Fatalf("inventory mismatch remains after repair: %+v", verified)
	}

	var mysqlRemain int64
	if err := h.db.QueryRowContext(ctx, "SELECT remain FROM tickethub_program.ticket_categories WHERE id = ?", fixtureCategoryID).Scan(&mysqlRemain); err != nil {
		t.Fatalf("read repaired MySQL inventory: %v", err)
	}
	redisRemain, err := h.redis.Get(ctx, inventoryKey()).Int64()
	if err != nil {
		t.Fatalf("read repaired Redis inventory: %v", err)
	}
	if mysqlRemain != fixtureTotal-1 || redisRemain != fixtureTotal-1 {
		t.Fatalf("repaired remains: mysql=%d redis=%d want=%d", mysqlRemain, redisRemain, fixtureTotal-1)
	}
}

func loginAdmin(ctx context.Context, t *testing.T, h *harness, mobile string, password string) loginReply {
	t.Helper()
	payload := map[string]string{"mobile": mobile, "password": password}
	login, err := callAPI[loginReply](ctx, h, http.MethodPost, "/api/users/login", "", payload)
	if err == nil {
		return login
	}
	var failure apiFailure
	if !errors.As(err, &failure) {
		t.Fatalf("login administrator: %v", err)
	}
	if _, registerErr := callAPI[registerReply](ctx, h, http.MethodPost, "/api/users/register", "", payload); registerErr != nil {
		t.Fatalf("register integration administrator after login failure (%v): %v", err, registerErr)
	}
	login, err = callAPI[loginReply](ctx, h, http.MethodPost, "/api/users/login", "", payload)
	if err != nil {
		t.Fatalf("login registered integration administrator: %v", err)
	}
	return login
}

func loadMappings(ctx context.Context, t *testing.T, h *harness, token string) []shardMapping {
	t.Helper()
	reply, err := callAPI[shardMappingsReply](ctx, h, http.MethodGet, "/api/migrate/shard-mappings", token, nil)
	if err != nil {
		t.Fatalf("load shard mappings: %v", err)
	}
	return reply.Mappings
}

func findMapping(mappings []shardMapping, virtualShard int32) (shardMapping, bool) {
	for _, mapping := range mappings {
		if mapping.VirtualShard == virtualShard {
			return mapping, true
		}
	}
	return shardMapping{}, false
}

func reconcile(ctx context.Context, t *testing.T, h *harness, token string, repair bool) reconciliationReply {
	t.Helper()
	reply, err := callAPI[reconciliationReply](ctx, h, http.MethodPost, "/api/admin/reconciliation/run", token, map[string]any{
		"program_id":       fixtureProgramID,
		"repair_inventory": repair,
	})
	if err != nil {
		t.Fatalf("run reconciliation repair=%t: %v", repair, err)
	}
	return reply
}

func waitWithContext(ctx context.Context, t *testing.T, duration time.Duration) {
	t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
}
