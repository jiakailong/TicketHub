package mysql

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

func TestShardedReconciliationRepositoryChecksOrdersThroughShardFinder(t *testing.T) {
	controlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer controlDB.Close()
	finder := fakeSystemOrderFinder{orders: map[int64]order.Order{1: {OrderNumber: 1}}}
	repository := NewShardedReconciliationRepository(controlDB, finder)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT order_number FROM order_records WHERE program_id = ?")).
		WithArgs(int64(10001)).
		WillReturnRows(sqlmock.NewRows([]string{"order_number"}).AddRow(1).AddRow(2))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE order_records SET reconciliation_status = ? WHERE order_number = ?")).
		WithArgs("MATCHED", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE order_records SET reconciliation_status = ? WHERE order_number = ?")).
		WithArgs("ORDER_MISSING", int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM order_records WHERE reconciliation_status = ? AND program_id = ?")).
		WithArgs("MATCHED", int64(10001)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM order_records WHERE reconciliation_status IS NOT NULL AND reconciliation_status <> 'MATCHED' AND program_id = ?")).
		WithArgs(int64(10001)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	result, err := repository.ReconcileProgram(context.Background(), 10001)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProcessedCount != 2 || result.MatchedCount != 1 || result.MismatchCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type fakeSystemOrderFinder struct {
	orders map[int64]order.Order
}

func (f fakeSystemOrderFinder) FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error) {
	item, ok := f.orders[orderNumber]
	if !ok {
		return order.Order{}, therrors.New(therrors.CodeNotFound, "order not found")
	}
	return item, nil
}
