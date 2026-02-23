package config

import (
	"fmt"
	"os"
	"strings"
)

// StrictExpandEnv expands environment variables in s, returning an error for undefined variables.
// Unlike os.ExpandEnv which silently replaces undefined vars with "", this returns a clear error.
func StrictExpandEnv(s string) (string, error) {
	var undefined []string
	expanded := os.Expand(s, func(key string) string {
		val, ok := os.LookupEnv(key)
		if !ok {
			undefined = append(undefined, "$"+key)
		}
		return val
	})
	if len(undefined) > 0 {
		return "", fmt.Errorf("undefined environment variable: %s: %w", strings.Join(undefined, ", "), ErrUndefinedEnvVar)
	}
	return expanded, nil
}
