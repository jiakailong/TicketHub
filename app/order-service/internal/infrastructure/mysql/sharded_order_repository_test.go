package mysql

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	orderapp "tickethub/app/order-service/internal/application"
	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/sharding"
)

func TestShardedOrderRepositoryRoutesSaveAndFind(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	location := sharding.Location{Database: "tickethub_order_1", Table: "orders_1"}
	router := fakeOrderShardRouter{primary: location, writes: []sharding.Location{location}, locations: []sharding.Location{location}}
	repository := NewShardedOrderRepository(sharding.NewDBPool(map[string]*sql.DB{"tickethub_order_1": database}), router)
	item := order.Order{
		OrderNumber: 11, ProgramID: 10001, UserID: 22, TicketCategoryID: 33,
		SeatIDs: []int64{44, 45}, AmountCent: 256000, Status: order.StatusNoPay, CreatedAt: time.Unix(1, 0),
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `orders_1`")).
		WithArgs(item.OrderNumber, item.ProgramID, item.UserID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), item.AmountCent, string(item.Status), item.CreatedAt, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repository.Save(context.Background(), item); err != nil {
		t.Fatal(err)
	}

	columns := []string{"order_number", "program_id", "user_id", "ticket_category_id", "seat_ids", "ticket_user_ids", "amount_cent", "status", "created_at", "paid_at", "canceled_at", "refunded_at"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM `orders_1` WHERE order_number = ? AND user_id = ?")).
		WithArgs(item.OrderNumber, item.UserID).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(item.OrderNumber, item.ProgramID, item.UserID, item.TicketCategoryID, "[44,45]", nil, item.AmountCent, string(item.Status), item.CreatedAt, nil, nil, nil))
	found, err := repository.FindByOrderNumber(context.Background(), item.OrderNumber, item.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if found.OrderNumber != item.OrderNumber || len(found.SeatIDs) != 2 {
		t.Fatalf("found = %+v", found)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShardedOrderRepositoryFailsClosedOnStaleRuntimeMapping(t *testing.T) {
	router := sharding.NewMappingOrderRouter(sharding.NewGeneOrderRouter("tickethub_order", "orders", 2, 2))
	repository := NewShardedOrderRepository(sharding.NewDBPool(nil), router)

	err := repository.Save(context.Background(), order.Order{OrderNumber: 1, UserID: 1})
	if !therrors.IsCode(err, therrors.CodeInfrastructure) {
		t.Fatalf("expected stale mapping infrastructure error, got %v", err)
	}
}

func TestShardedOrderRepositoryWritesEveryMigrationLocation(t *testing.T) {
	database0, mock0, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database0.Close()
	database1, mock1, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database1.Close()
	primary := sharding.Location{Database: "tickethub_order_0", Table: "orders_0"}
	shadow := sharding.Location{Database: "tickethub_order_1", Table: "orders_1"}
	router := fakeOrderShardRouter{primary: primary, writes: []sharding.Location{primary, shadow}, locations: []sharding.Location{primary}}
	repository := NewShardedOrderRepository(sharding.NewDBPool(map[string]*sql.DB{
		"tickethub_order_0": database0,
		"tickethub_order_1": database1,
	}), router)
	item := order.Order{OrderNumber: 1, ProgramID: 2, UserID: 3, AmountCent: 4, Status: order.StatusPaid, CreatedAt: time.Unix(1, 0)}

	mock0.ExpectExec(regexp.QuoteMeta("INSERT IGNORE INTO `orders_0`")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock0.ExpectExec(regexp.QuoteMeta("UPDATE `orders_0`")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock1.ExpectExec(regexp.QuoteMeta("INSERT IGNORE INTO `orders_1`")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock1.ExpectExec(regexp.QuoteMeta("UPDATE `orders_1`")).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.Update(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if err := mock0.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := mock1.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShardedOrderRepositoryAggregatesInventoryAcrossPrimaryShards(t *testing.T) {
	database0, mock0, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database0.Close()
	database1, mock1, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database1.Close()
	location0 := sharding.Location{Database: "tickethub_order_0", Table: "orders_0"}
	location1 := sharding.Location{Database: "tickethub_order_1", Table: "orders_1"}
	router := fakeOrderShardRouter{primary: location0, locations: []sharding.Location{location0, location1}}
	repository := NewShardedOrderRepository(sharding.NewDBPool(map[string]*sql.DB{
		"tickethub_order_0": database0,
		"tickethub_order_1": database1,
	}), router)

	mock0.ExpectQuery("SELECT ticket_category_id,").WithArgs(int64(10001)).WillReturnRows(
		sqlmock.NewRows([]string{"ticket_category_id", "occupied_count"}).AddRow(10, 2).AddRow(20, 1),
	)
	mock1.ExpectQuery("SELECT ticket_category_id,").WithArgs(int64(10001)).WillReturnRows(
		sqlmock.NewRows([]string{"ticket_category_id", "occupied_count"}).AddRow(10, 3),
	)
	usages, err := repository.ListInventoryUsage(context.Background(), 10001)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 2 || usages[0].TicketCategoryID != 10 || usages[0].OccupiedCount != 5 || usages[1].OccupiedCount != 1 {
		t.Fatalf("usages = %+v", usages)
	}
}

func TestShardedOrderRepositoryRoutesUserListToOnePrimaryShard(t *testing.T) {
	database0, mock0, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database0.Close()
	database1, mock1, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database1.Close()
	primary := sharding.Location{Database: "tickethub_order_0", Table: "orders_0"}
	other := sharding.Location{Database: "tickethub_order_1", Table: "orders_1"}
	router := fakeOrderShardRouter{primary: primary, locations: []sharding.Location{primary, other}}
	repository := NewShardedOrderRepository(sharding.NewDBPool(map[string]*sql.DB{
		"tickethub_order_0": database0,
		"tickethub_order_1": database1,
	}), router)
	columns := []string{"order_number", "program_id", "user_id", "ticket_category_id", "seat_ids", "ticket_user_ids", "amount_cent", "status", "created_at", "paid_at", "canceled_at", "refunded_at"}
	mock0.ExpectQuery("FROM `orders_0`").WithArgs(int64(22), string(order.StatusPaid), 3).WillReturnRows(
		sqlmock.NewRows(columns).AddRow(11, 10001, 22, 33, nil, nil, 9900, string(order.StatusPaid), time.Now(), time.Now(), nil, nil),
	)
	items, err := repository.ListByUserIDPage(context.Background(), 22, order.StatusPaid, orderapp.OrderListCursor{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OrderNumber != 11 {
		t.Fatalf("items = %+v", items)
	}
	if err := mock0.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := mock1.ExpectationsWereMet(); err != nil {
		t.Fatalf("unrelated shard was queried: %v", err)
	}
}

type fakeOrderShardRouter struct {
	primary   sharding.Location
	writes    []sharding.Location
	locations []sharding.Location
}

func (r fakeOrderShardRouter) RouteOrder(orderNumber int64, userID int64) sharding.Location {
	return r.primary
}

func (r fakeOrderShardRouter) RouteOrderWrites(orderNumber int64, userID int64) []sharding.Location {
	return append([]sharding.Location(nil), r.writes...)
}

func (r fakeOrderShardRouter) PrimaryLocations() []sharding.Location {
	return append([]sharding.Location(nil), r.locations...)
}
