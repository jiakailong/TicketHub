package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIPResolver accepts forwarding headers only from configured reverse proxies.
type ClientIPResolver struct {
	trusted []netip.Prefix
}

func NewClientIPResolver(rawCIDRs []string) (ClientIPResolver, error) {
	trusted := make([]netip.Prefix, 0, len(rawCIDRs))
	for _, raw := range rawCIDRs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return ClientIPResolver{}, err
		}
		trusted = append(trusted, prefix)
	}
	return ClientIPResolver{trusted: trusted}, nil
}

func (r ClientIPResolver) Resolve(request *http.Request) string {
	if request == nil {
		return ""
	}
	peer, ok := parseAddress(request.RemoteAddr)
	if !ok {
		return ""
	}
	if !r.isTrusted(peer) {
		return peer.String()
	}
	// Proxies append the immediate source to X-Forwarded-For. Walking from the
	// right prevents a client-supplied leftmost value from changing its identity.
	for parts := strings.Split(request.Header.Get("X-Forwarded-For"), ","); len(parts) > 0; parts = parts[:len(parts)-1] {
		candidate, valid := parseAddress(strings.TrimSpace(parts[len(parts)-1]))
		if valid && !r.isTrusted(candidate) {
			return candidate.String()
		}
	}
	if value, valid := parseAddress(request.Header.Get("X-Real-IP")); valid && !r.isTrusted(value) {
		return value.String()
	}
	return peer.String()
}

func (r ClientIPResolver) isTrusted(address netip.Addr) bool {
	for _, prefix := range r.trusted {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseAddress(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}
