// ssrf.go -- SSRF protection helpers.
package api

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// privateRanges lists IP ranges that should not be reachable via server-side
// HTTP requests initiated by user input or AI tool calls.
var privateRanges = []net.IPNet{
	cidr("127.0.0.0/8"),    // loopback
	cidr("10.0.0.0/8"),     // RFC1918
	cidr("172.16.0.0/12"),  // RFC1918
	cidr("192.168.0.0/16"), // RFC1918
	cidr("169.254.0.0/16"), // link-local / cloud metadata
	cidr("100.64.0.0/10"),  // CGNAT
	cidr("::1/128"),        // IPv6 loopback
	cidr("fc00::/7"),       // IPv6 ULA
	cidr("fe80::/10"),      // IPv6 link-local
}

func cidr(s string) net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("bad cidr: " + s)
	}
	return *n
}

// isPrivateIP reports whether ip falls in a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// checkSSRF resolves rawURL and returns an error if any resolved IP is private
// or loopback. This prevents server-side request forgery.
func checkSSRF(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http/https
	scheme := u.Scheme
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL scheme %q is not allowed", scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL has no host")
	}

	// If the host is already an IP, check it directly.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("requests to private/loopback addresses are not allowed")
		}
		return nil
	}

	// Otherwise resolve and check each returned address.
	var resolver net.Resolver
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed: %w", err)
	}
	for _, addr := range addrs {
		if isPrivateIP(addr.IP) {
			return fmt.Errorf("requests to private/loopback addresses are not allowed")
		}
	}
	return nil
}
