package config

import "strings"

type stringSliceValue struct {
	target *[]string
}

func (s *stringSliceValue) String() string {
	if s == nil || s.target == nil {
		return ""
	}
	return strings.Join(*s.target, ",")
}

func (s *stringSliceValue) Set(value string) error {
	*s.target = append(*s.target, value)
	return nil
}
