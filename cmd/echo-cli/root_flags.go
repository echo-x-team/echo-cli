package main

import (
	"fmt"
	"strings"

	"echo-cli/internal/features"
)

type rootArgs struct {
	overrides []string
}

func parseRootArgs(args []string) (rootArgs, []string, error) {
	var overrides stringSlice
	var enable stringSlice
	var disable stringSlice
	rest := []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c":
			if i+1 >= len(args) {
				return rootArgs{}, nil, fmt.Errorf("flag needs an argument: -c")
			}
			overrides = append(overrides, args[i+1])
			i++
		case strings.HasPrefix(arg, "-c=") || strings.HasPrefix(arg, "--c="):
			overrides = append(overrides, trimFlagValue(arg, "-c=", "--c="))
		case arg == "-enable" || arg == "--enable":
			if i+1 >= len(args) {
				return rootArgs{}, nil, fmt.Errorf("flag needs an argument: %s", arg)
			}
			enable = append(enable, args[i+1])
			i++
		case strings.HasPrefix(arg, "-enable=") || strings.HasPrefix(arg, "--enable="):
			enable = append(enable, trimFlagValue(arg, "-enable=", "--enable="))
		case arg == "-disable" || arg == "--disable":
			if i+1 >= len(args) {
				return rootArgs{}, nil, fmt.Errorf("flag needs an argument: %s", arg)
			}
			disable = append(disable, args[i+1])
			i++
		case strings.HasPrefix(arg, "-disable=") || strings.HasPrefix(arg, "--disable="):
			disable = append(disable, trimFlagValue(arg, "-disable=", "--disable="))
		default:
			rest = append(rest, arg)
		}
	}

	featureOverrides, err := buildFeatureOverrides(enable, disable)
	if err != nil {
		return rootArgs{}, nil, err
	}
	all := append([]string{}, overrides...)
	all = append(all, featureOverrides...)
	return rootArgs{overrides: all}, rest, nil
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

func trimFlagValue(arg string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return arg
}
