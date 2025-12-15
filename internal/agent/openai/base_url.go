package openai

import (
	"net/url"
	"strings"
)

func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return raw
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		path = strings.TrimSuffix(path, "/chat/completions")
	case strings.HasSuffix(path, "/completions"):
		path = strings.TrimSuffix(path, "/completions")
	case strings.HasSuffix(path, "/responses"):
		path = strings.TrimSuffix(path, "/responses")
	}
	path = strings.TrimRight(path, "/")

	if !strings.HasSuffix(path, "/v1") {
		if path == "" {
			path = "/v1"
		} else {
			path = path + "/v1"
		}
	}
	for strings.Contains(path, "/v1/v1") {
		path = strings.ReplaceAll(path, "/v1/v1", "/v1")
	}

	parsed.Path = path
	return parsed.String()
}
