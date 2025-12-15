package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func CheckResponsesEndpoint(ctx context.Context, baseURL string, apiKey string, model string) (string, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return "", errors.New("missing OPENAI_API_KEY")
	}

	base := normalizeBaseURL(baseURL)
	if strings.TrimSpace(base) == "" {
		base = "https://api.openai.com/v1"
	}
	endpoint := strings.TrimRight(base, "/") + "/responses"

	reqBody := map[string]any{
		"model": strings.TrimSpace(model),
		"input": []map[string]any{
			{"role": "system", "content": "请严格只输出 ping（全小写），不要任何其他字符（不要标点、不要换行）。"},
			{"role": "user", "content": "ping"},
		},
	}
	if strings.TrimSpace(model) == "" {
		reqBody["model"] = "gpt-4o-mini"
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http_%d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	text := strings.TrimSpace(extractOutputText(body))
	if text == "" {
		return "", errors.New("responses api returned no text")
	}
	return text, nil
}

func extractOutputText(raw []byte) string {
	var decoded struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	if strings.TrimSpace(decoded.OutputText) != "" {
		return strings.TrimSpace(decoded.OutputText)
	}
	for _, item := range decoded.Output {
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}
		for _, c := range item.Content {
			if strings.TrimSpace(c.Text) != "" {
				return strings.TrimSpace(c.Text)
			}
		}
	}
	return ""
}
