package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPResolverRejectsSpoofedForwardingHeaders(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/", nil)
	request.RemoteAddr = "198.51.100.20:3456"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := resolver.Resolve(request); got != "198.51.100.20" {
		t.Fatalf("client ip = %q", got)
	}
}

func TestClientIPResolverUsesRightmostUntrustedForwardedAddress(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/", nil)
	request.RemoteAddr = "10.0.0.5:8080"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.20, 10.0.0.4")
	if got := resolver.Resolve(request); got != "198.51.100.20" {
		t.Fatalf("client ip = %q", got)
	}
}
