package compiler

import (
	"regexp"
	"strings"
)

var apisixEnvPattern = regexp.MustCompile(`\$\{\{\s*([A-Za-z_][A-Za-z0-9_]*)?\s*(?::=\s*([^}]*))?\s*\}\}`)

func SubstituteAPISIX(input string, envVars map[string]string) string {
	return apisixEnvPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := apisixEnvPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		name := strings.TrimSpace(parts[1])
		defaultValue := ""
		if len(parts) > 2 {
			defaultValue = strings.TrimSpace(parts[2])
		}
		if name == "" {
			return defaultValue
		}
		value, ok := envVars[name]
		if ok {
			return value
		}
		return defaultValue
	})
}
