package config

import (
	"path/filepath"
	"sort"

	"github.com/spf13/afero"
)

// ConfigFileRef is a raw config file reference (extends/includes entry).
// It may contain env vars (${VAR}), relative paths, and glob patterns.
type ConfigFileRef struct {
	ParentDir string // directory of the declaring config file
	path      string // raw path from TOML (unexported)
}

// NewConfigFileRef creates a ConfigFileRef with the given parent directory and raw path.
func NewConfigFileRef(parentDir, path string) ConfigFileRef {
	return ConfigFileRef{ParentDir: parentDir, path: path}
}

// Expand resolves env vars, relative paths, and globs. Returns resolved absolute paths.
func (r ConfigFileRef) Expand(getenv func(string) string, fs afero.Fs) ([]string, error) {
	// 1. Expand env vars
	pattern := getenv(r.path)

	// 2. Resolve relative paths
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(r.ParentDir, pattern)
	}

	// 3. Glob expansion
	if !isGlobPattern(pattern) {
		// Literal path - must exist
		if _, err := fs.Stat(pattern); err != nil {
			return nil, err
		}
		return []string{pattern}, nil
	}

	// Glob pattern - empty result is OK
	matches, err := afero.Glob(fs, pattern)
	if err != nil {
		return nil, err
	}
	// Sort for deterministic order
	sort.Strings(matches)
	return matches, nil
}

// isGlobPattern checks if the pattern contains glob special characters.
func isGlobPattern(pattern string) bool {
	for _, c := range pattern {
		switch c {
		case '*', '?', '[':
			return true
		}
	}
	return false
}
