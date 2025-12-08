package main

import (
	"reflect"
	"testing"
)

func TestParseRootArgsAllowsUnknownFlags(t *testing.T) {
	orig := []string{"--prompt", "测试"}
	root, rest, err := parseRootArgs(orig)
	if err != nil {
		t.Fatalf("parseRootArgs returned error: %v", err)
	}
	if len(root.overrides) != 0 {
		t.Fatalf("expected no overrides, got %v", root.overrides)
	}
	if !reflect.DeepEqual(rest, orig) {
		t.Fatalf("expected rest to preserve args %v, got %v", orig, rest)
	}
}

func TestParseRootArgsExtractsOverrides(t *testing.T) {
	args := []string{
		"-c", "k=v",
		"--enable", "web_search_request",
		"-disable=skills",
		"--prompt", "hi",
	}
	root, rest, err := parseRootArgs(args)
	if err != nil {
		t.Fatalf("parseRootArgs returned error: %v", err)
	}
	expectedOverrides := []string{
		"k=v",
		"features.web_search_request=true",
		"features.skills=false",
	}
	if !reflect.DeepEqual(root.overrides, expectedOverrides) {
		t.Fatalf("unexpected overrides: got %v, want %v", root.overrides, expectedOverrides)
	}
	expectedRest := []string{"--prompt", "hi"}
	if !reflect.DeepEqual(rest, expectedRest) {
		t.Fatalf("unexpected rest args: got %v, want %v", rest, expectedRest)
	}
}
