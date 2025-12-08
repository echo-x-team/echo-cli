package main

import "strings"

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type csvSlice []string

func (s *csvSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *csvSlice) Set(v string) error {
	parts := strings.Split(v, ",")
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			*s = append(*s, trimmed)
		}
	}
	return nil
}
