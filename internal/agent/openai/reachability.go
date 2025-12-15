package openai

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func CheckBaseURLReachable(ctx context.Context, baseURL string) error {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return nil
	}

	normalized := normalizeBaseURL(raw)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil {
		return fmt.Errorf("invalid base_url %q: %w", baseURL, err)
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.TrimSpace(parsed.Hostname())
	if scheme == "" || host == "" {
		return fmt.Errorf("invalid base_url %q: scheme=%q host=%q", baseURL, parsed.Scheme, parsed.Host)
	}

	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return fmt.Errorf("unsupported base_url scheme %q (base_url=%q)", parsed.Scheme, baseURL)
		}
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("invalid base_url port %q (base_url=%q): %w", port, baseURL, err)
	}

	addr := net.JoinHostPort(host, port)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot connect to %s (base_url=%q): %w", addr, baseURL, err)
	}
	_ = conn.Close()
	return nil
}
