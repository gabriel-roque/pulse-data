package webhook

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

var ErrBlockedURL = errors.New("webhook endpoint blocked by SSRF policy")

func ValidateEndpoint(ctx context.Context, raw string, resolver func(context.Context, string) ([]net.IP, error)) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ErrBlockedURL
	}
	if resolver == nil {
		return ErrBlockedURL
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "metadata.google.internal" {
		return ErrBlockedURL
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ips, err := resolver(ctx, host)
	if err != nil || len(ips) == 0 {
		return ErrBlockedURL
	}
	for _, ip := range ips {
		if isPrivateIP(ip) && !allowPrivateEndpoints() {
			return ErrBlockedURL
		}
	}
	return nil
}

// Private endpoints are only useful for the isolated local E2E receiver. The
// opt-in is never enabled by the production defaults.
func allowPrivateEndpoints() bool { return os.Getenv("PULSE_ALLOW_PRIVATE_WEBHOOKS") == "true" }

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// IPv4-mapped IPv6 and the cloud metadata range are explicitly denied.
	v4 := ip.To4()
	if v4 != nil && ((v4[0] == 169 && v4[1] == 254) || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127)) {
		return true
	}
	return false
}
