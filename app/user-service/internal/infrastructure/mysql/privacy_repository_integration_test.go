//go:build integration

package mysql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"tickethub/app/user-service/internal/domain/user"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/privacy"
)

func TestEncryptedRepositoriesAgainstMySQL(t *testing.T) {
	if os.Getenv("TICKETHUB_INTEGRATION") != "1" {
		t.Skip("set TICKETHUB_INTEGRATION=1 to run infrastructure integration tests")
	}
	dsn := os.Getenv("TICKETHUB_TEST_USER_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("TICKETHUB_TEST_USER_MYSQL_DSN is required for integration tests")
	}
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	protector, err := privacy.NewProtector(
		"test-v1",
		map[string]string{"test-v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{41}, 32))},
		base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32)),
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	id := now.UnixNano() & 0x3fffffffffffffff
	ticketUserID := id + 1
	mobile := fmt.Sprintf("139%08d", id%100000000)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer func() {
		_, _ = database.Exec("DELETE FROM ticket_users WHERE id = ?", ticketUserID)
		_, _ = database.Exec("DELETE FROM users WHERE id IN (?, ?)", id, id+2)
	}()

	users := NewUserRepository(database, protector)
	created := user.NewUser(id, mobile, "bcrypt-hash", now)
	created.Email = "Private.User@example.com"
	if err := users.Save(ctx, created); err != nil {
		t.Fatal(err)
	}
	found, err := users.FindByMobile(ctx, mobile)
	if err != nil {
		t.Fatal(err)
	}
	if found.Mobile != mobile || found.Email != "private.user@example.com" {
		t.Fatalf("decrypted user = %+v", found)
	}
	duplicate := user.NewUser(id+2, mobile, "different-hash", now)
	if err := users.Save(ctx, duplicate); !therrors.IsCode(err, therrors.CodeConflict) {
		t.Fatalf("duplicate error = %v", err)
	}

	var mobileCiphertext []byte
	var mobileLookup []byte
	if err := database.QueryRowContext(ctx, "SELECT mobile_ciphertext, mobile_lookup FROM users WHERE id=?", id).Scan(&mobileCiphertext, &mobileLookup); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(mobileCiphertext, []byte(mobile)) || len(mobileLookup) != sha256Size {
		t.Fatalf("mobile was not safely persisted: ciphertext=%x lookup_length=%d", mobileCiphertext, len(mobileLookup))
	}

	ticketUsers := NewTicketUserRepository(database, protector)
	privateTicketUser := user.TicketUser{
		ID:            ticketUserID,
		UserID:        id,
		Name:          "张三",
		CertificateNo: "310101199001010011",
		Mobile:        mobile,
	}
	if err := ticketUsers.Save(ctx, privateTicketUser); err != nil {
		t.Fatal(err)
	}
	items, err := ticketUsers.ListByUserID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "张三" || items[0].CertificateNo != "310101199001010011" {
		t.Fatalf("decrypted ticket users = %+v", items)
	}
	var certificateCiphertext []byte
	if err := database.QueryRowContext(ctx, "SELECT certificate_ciphertext FROM ticket_users WHERE id=?", ticketUserID).Scan(&certificateCiphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(certificateCiphertext, []byte(privateTicketUser.CertificateNo)) {
		t.Fatal("certificate plaintext was found in ciphertext column")
	}

	for table, columns := range map[string][]string{
		"users":        {"mobile", "email"},
		"ticket_users": {"name", "certificate_no", "mobile"},
	} {
		for _, column := range columns {
			var count int
			if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("legacy plaintext column still exists: %s.%s", table, column)
			}
		}
	}
}

const sha256Size = 32
