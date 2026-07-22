package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMobileLookupScannerReadsBatchesByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	query := "SELECT id, mobile_lookup"
	mock.ExpectQuery(query).WithArgs(int64(0), 2).WillReturnRows(
		sqlmock.NewRows([]string{"id", "mobile_lookup"}).AddRow(1, []byte("first")).AddRow(2, []byte("second")),
	)
	mock.ExpectQuery(query).WithArgs(int64(2), 2).WillReturnRows(sqlmock.NewRows([]string{"id", "mobile_lookup"}))
	var values [][]byte
	err = NewMobileLookupScanner(db).ScanMobileLookups(context.Background(), 2, func(_ context.Context, value []byte) error {
		values = append(values, append([]byte(nil), value...))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || string(values[0]) != "first" || string(values[1]) != "second" {
		t.Fatalf("values = %q", values)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
