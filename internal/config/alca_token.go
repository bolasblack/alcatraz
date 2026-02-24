package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	// alcaTokenPattern matches ${alca:...} patterns, capturing the content after "alca:".
	alcaTokenPattern = regexp.MustCompile(`\$\{alca:([^}]*)\}`)

	// validTokenName checks that a token name is alphanumeric + underscore, non-empty.
	validTokenName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

	// knownAlcaTokens is the set of recognized alca token names.
	knownAlcaTokens = map[string]struct{}{
		"HOST_IP": {},
	}
)

// ValidateAlcaTokens scans s for ${alca:...} patterns and validates them.
// It checks token name syntax (alphanumeric + underscore) and rejects unknown token names.
// Patterns like ${VAR} (no alca: prefix) are ignored.
func ValidateAlcaTokens(s string) error {
	matches := alcaTokenPattern.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		name := match[1]
		if !validTokenName.MatchString(name) {
			return fmt.Errorf("invalid alca token syntax %q in %q: %w", match[0], s, ErrInvalidAlcaToken)
		}
		if _, ok := knownAlcaTokens[name]; !ok {
			return fmt.Errorf("unknown alca token %q in %q; valid tokens: %s: %w", name, s, knownAlcaTokenNames(), ErrUnknownAlcaToken)
		}
	}
	return nil
}

// ExpandAlcaTokens finds all ${alca:TOKEN_NAME} patterns in s and replaces them
// using the provided resolver function. Patterns like ${VAR} (no alca: prefix) are
// left untouched.
func ExpandAlcaTokens(s string, resolver func(string) (string, error)) (string, error) {
	if err := ValidateAlcaTokens(s); err != nil {
		return "", err
	}

	var expandErr error
	result := alcaTokenPattern.ReplaceAllStringFunc(s, func(match string) string {
		if expandErr != nil {
			return match
		}
		sub := alcaTokenPattern.FindStringSubmatch(match)
		name := sub[1]
		val, err := resolver(name)
		if err != nil {
			expandErr = fmt.Errorf("resolving alca token %q: %w", name, err)
			return match
		}
		return val
	})
	if expandErr != nil {
		return "", expandErr
	}
	return result, nil
}

// knownAlcaTokenNames returns a sorted, comma-separated list of valid token names.
func knownAlcaTokenNames() string {
	names := make([]string, 0, len(knownAlcaTokens))
	for name := range knownAlcaTokens {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ExpandAlcaTokensInStrings expands ${alca:...} tokens in a slice of strings.
// Strings without alca tokens are passed through unchanged.
// The resolver is called for each unique token name encountered.
func ExpandAlcaTokensInStrings(strs []string, resolver func(string) (string, error)) ([]string, error) {
	hasTokens := false
	for _, s := range strs {
		if ContainsAlcaTokens(s) {
			hasTokens = true
			break
		}
	}
	if !hasTokens {
		return strs, nil
	}

	expanded := make([]string, len(strs))
	for i, s := range strs {
		result, err := ExpandAlcaTokens(s, resolver)
		if err != nil {
			return nil, err
		}
		expanded[i] = result
	}
	return expanded, nil
}

// ContainsAlcaTokens reports whether s contains any ${alca:...} patterns.
func ContainsAlcaTokens(s string) bool {
	return strings.Contains(s, "${alca:")
}
