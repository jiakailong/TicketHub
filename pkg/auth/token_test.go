package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTokenManager(t *testing.T) {
	manager := NewTokenManager("secret")
	token, err := manager.Generate(Claims{UserID: 1, Role: "user", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(token, ".")) != 3 {
		t.Fatalf("expected compact JWT, got %s", token)
	}
	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 1 || claims.Role != "user" {
		t.Fatalf("claims = %+v", claims)
	}
}
