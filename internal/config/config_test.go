package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_Model(t *testing.T) {
	cfg := Default()
	if cfg.Model != "glm4.6" {
		t.Fatalf("Default().Model = %q, want %q", cfg.Model, "glm4.6")
	}
}

func TestLoad_MissingFile_UsesDefaultModel(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Source != path {
		t.Fatalf("cfg.Source = %q, want %q", cfg.Source, path)
	}
	if cfg.Model != "glm4.6" {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, "glm4.6")
	}
}

func TestLoad_ModelFromTOML(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
url = "https://example.test"
token = "test-token"
model = "glm4.6.custom"
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "glm4.6.custom" {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, "glm4.6.custom")
	}
}

func TestApplyKVOverrides_Model(t *testing.T) {
	cfg := Default()
	got := ApplyKVOverrides(cfg, []string{"model=override-model"})
	if got.Model != "override-model" {
		t.Fatalf("ApplyKVOverrides(...).Model = %q, want %q", got.Model, "override-model")
	}
}
