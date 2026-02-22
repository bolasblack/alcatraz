package config

import "path/filepath"

// FileRef handles env expansion + relative path resolution for a single path.
// No glob expansion. Returns a single resolved path.
type FileRef struct {
	FromConfigFilePath string // full path of the declaring config file
	path               string // raw path (unexported)
}

// NewFileRef creates a FileRef.
func NewFileRef(fromConfigFilePath, path string) FileRef {
	return FileRef{FromConfigFilePath: fromConfigFilePath, path: path}
}

// Expand resolves env vars and relative paths. Returns a single absolute path.
func (r FileRef) Expand(expandEnv func(string) (string, error)) (string, error) {
	// 1. Expand env vars
	pattern, err := expandEnv(r.path)
	if err != nil {
		return "", err
	}

	// 2. Resolve relative to parent directory of the declaring config file
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(filepath.Dir(r.FromConfigFilePath), pattern)
	}

	return pattern, nil
}
