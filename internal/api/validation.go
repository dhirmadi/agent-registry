package api

import (
	"net"
	"net/url"
	"regexp"
	"strings"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
)

// canonicalIPRegex matches a standard dotted-decimal IPv4 address (no leading zeros, no non-digit chars).
var canonicalIPRegex = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

// validateEndpointURL checks that an endpoint is a valid HTTP(S) URL pointing to a public host.
func validateEndpointURL(endpoint string) error {
	if len(endpoint) > 2000 {
		return apierrors.Validation("endpoint must be at most 2000 characters")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return apierrors.Validation("endpoint is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return apierrors.Validation("endpoint must use http or https scheme")
	}
	host := u.Hostname()
	if host == "" {
		return apierrors.Validation("endpoint must have a valid host")
	}
	if isPrivateHost(host) {
		return apierrors.Validation("endpoint must not point to a private or internal address")
	}
	return nil
}

// isPrivateHost returns true if the host is a private/internal/loopback address.
// It also rejects non-canonical IP representations (decimal, octal, hex, zero-padded)
// that could bypass IP-based SSRF checks.
func isPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}

	// Reject bare decimal IPs (e.g. "2130706433" = 127.0.0.1)
	// and any IP-like string with leading zeros (e.g. "127.0.0.01", "0177.0.0.1")
	// or hex notation (e.g. "0x7f.0.0.1").
	// Go's net.ParseIP is strict and returns nil for these, but HTTP clients resolve them.
	// If it looks like a non-standard IP notation, reject it as potentially private.
	if looksLikeNonCanonicalIP(host) {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	// Block 0.0.0.0 explicitly (binds to all interfaces)
	if ip.Equal(net.IPv4zero) {
		return true
	}

	privateRanges := []struct {
		network string
	}{
		{"127.0.0.0/8"},
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"169.254.0.0/16"},
		{"::1/128"},
		{"fc00::/7"},
		{"fe80::/10"},
	}
	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r.network)
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// looksLikeNonCanonicalIP detects IP representations that Go's net.ParseIP won't parse
// but that HTTP clients (curl, browsers) will resolve: decimal IPs ("2130706433"),
// octal ("0177.0.0.1"), hex ("0x7f.0.0.1"), and zero-padded ("127.0.0.01").
func looksLikeNonCanonicalIP(host string) bool {
	// Strip brackets for IPv6
	stripped := strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")

	// Pure numeric string (decimal IP like 2130706433)
	if len(stripped) > 0 && isAllDigits(stripped) {
		return true
	}

	// Check for hex prefix in any octet (0x7f.0.0.1)
	if strings.Contains(stripped, "0x") || strings.Contains(stripped, "0X") {
		return true
	}

	// Check if it looks like a dotted IP but with leading zeros in any octet
	if canonicalIPRegex.MatchString(stripped) {
		// Already matches standard format, check for leading zeros
		octets := strings.Split(stripped, ".")
		for _, o := range octets {
			if len(o) > 1 && o[0] == '0' {
				return true // Leading zero in octet (octal or zero-padded)
			}
		}
	}

	// Check for dotted notation with non-standard octets (e.g. "0177.0.0.1")
	parts := strings.Split(stripped, ".")
	if len(parts) == 4 {
		allNumeric := true
		for _, p := range parts {
			if len(p) == 0 || !isAllDigits(p) {
				allNumeric = false
				break
			}
		}
		if allNumeric {
			for _, p := range parts {
				if len(p) > 1 && p[0] == '0' {
					return true
				}
			}
		}
	}

	return false
}

// isAllDigits returns true if s is non-empty and contains only ASCII digits.
func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
