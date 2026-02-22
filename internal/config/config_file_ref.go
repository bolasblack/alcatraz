package config

import (
	"sort"

	"github.com/spf13/afero"
)

// ConfigFileRef is a raw config file reference (extends/includes entry).
// It may contain env vars (${VAR}), relative paths, and glob patterns.
type ConfigFileRef struct {
	FromConfigFilePath string // full path of the declaring config file
	path               string // raw path (unexported)
}

// NewConfigFileRef creates a ConfigFileRef with the given config file path and raw path.
func NewConfigFileRef(fromConfigFilePath, path string) ConfigFileRef {
	return ConfigFileRef{FromConfigFilePath: fromConfigFilePath, path: path}
}

// Expand resolves env vars, relative paths, and globs. Returns resolved absolute paths.
func (r ConfigFileRef) Expand(expandEnv func(string) (string, error), fs afero.Fs) ([]string, error) {
	// Delegate env expansion + path resolution to FileRef
	ref := NewFileRef(r.FromConfigFilePath, r.path)
	resolved, err := ref.Expand(expandEnv)
	if err != nil {
		return nil, err
	}

	// Glob expansion
	if !isGlobPattern(resolved) {
		// Literal path - must exist
		if _, err := fs.Stat(resolved); err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	}

	// Glob pattern - empty result is OK
	matches, err := afero.Glob(fs, resolved)
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
