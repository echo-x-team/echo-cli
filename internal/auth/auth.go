package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Credentials struct {
	APIKey       string    `json:"api_key"`
	LegacyAPIKey string    `json:"OPENAI_API_KEY"`
	Updated      time.Time `json:"updated"`
}

func authPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".echo", "auth.json"), nil
}

// SaveAPIKey persists an API key for later use by the CLI.
func SaveAPIKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("empty API key")
	}
	path, err := authPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(Credentials{APIKey: key, Updated: time.Now()}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadAPIKey loads the stored API key, returning an empty string when none is present.
func LoadAPIKey() (string, error) {
	path, err := authPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", err
	}
	key := strings.TrimSpace(creds.APIKey)
	if key == "" {
		key = strings.TrimSpace(creds.LegacyAPIKey)
	}
	return key, nil
}

// Clear removes any stored credentials.
func Clear() error {
	path, err := authPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
