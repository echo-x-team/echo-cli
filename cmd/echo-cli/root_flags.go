package main

import (
	"flag"
	"fmt"

	"echo-cli/internal/features"
)

type rootArgs struct {
	overrides []string
}

func parseRootArgs(args []string) (rootArgs, []string, error) {
	fs := flag.NewFlagSet("echo-cli", flag.ContinueOnError)
	var overrides stringSlice
	var enable stringSlice
	var disable stringSlice
	fs.Var(&overrides, "c", "Override config value key=value (repeatable, applied before subcommand overrides)")
	fs.Var(&enable, "enable", "Enable a feature (repeatable). Equivalent to -c features.<name>=true")
	fs.Var(&disable, "disable", "Disable a feature (repeatable). Equivalent to -c features.<name>=false")
	if err := fs.Parse(args); err != nil {
		return rootArgs{}, nil, err
	}

	featureOverrides, err := buildFeatureOverrides(enable, disable)
	if err != nil {
		return rootArgs{}, nil, err
	}
	all := append([]string{}, overrides...)
	all = append(all, featureOverrides...)
	return rootArgs{overrides: all}, fs.Args(), nil
}

func prependOverrides(root []string, overrides []string) []string {
	merged := append([]string{}, root...)
	return append(merged, overrides...)
}

func buildFeatureOverrides(enable []string, disable []string) ([]string, error) {
	var overrides []string
	for _, key := range enable {
		if !features.IsKnown(key) {
			return nil, fmt.Errorf("unknown feature flag: %s", key)
		}
		overrides = append(overrides, fmt.Sprintf("features.%s=%t", key, true))
	}
	for _, key := range disable {
		if !features.IsKnown(key) {
			return nil, fmt.Errorf("unknown feature flag: %s", key)
		}
		overrides = append(overrides, fmt.Sprintf("features.%s=%t", key, false))
	}
	return overrides, nil
}
