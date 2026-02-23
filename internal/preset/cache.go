package preset

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// PresetEnv contains dependencies for preset operations.
type PresetEnv struct {
	Fs  afero.Fs
	Cmd util.CommandRunner
}

// NewPresetEnv creates a new PresetEnv with the given filesystem and command runner.
func NewPresetEnv(fs afero.Fs, cmd util.CommandRunner) *PresetEnv {
	return &PresetEnv{Fs: fs, Cmd: cmd}
}

// CacheManager manages git bare-repo caches for preset repositories.
type CacheManager struct {
	env      *PresetEnv
	git      *gitOps
	cacheDir string // base cache dir, e.g. ~/.alcatraz/cache-presets
}

// NewCacheManager creates a new CacheManager with the given environment and base cache directory.
func NewCacheManager(env *PresetEnv, cacheDir string) *CacheManager {
	return &CacheManager{env: env, git: &gitOps{cmd: env.Cmd}, cacheDir: cacheDir}
}

// EnsureRepo ensures a git bare repo cache exists and contains the requested commit.
// It returns the repo directory path and the resolved commit hash.
// Parameters are primitives so this module is independent of the URL parser.
func (cm *CacheManager) EnsureRepo(ctx context.Context, cloneURL string, cachePath string, commitHash string) (repoDir string, resolvedCommit string, err error) {
	repoDir = filepath.Join(cm.cacheDir, cachePath)

	exists, err := afero.DirExists(cm.env.Fs, repoDir)
	if err != nil {
		return "", "", fmt.Errorf("checking cache directory: %w", err)
	}

	if !exists {
		if err := cm.env.Fs.MkdirAll(repoDir, 0o755); err != nil {
			return "", "", fmt.Errorf("creating cache directory: %w", err)
		}
		if err := cm.git.InitBare(ctx, repoDir); err != nil {
			return "", "", fmt.Errorf("initializing bare repo: %w", err)
		}
	}

	// If a specific commit is requested and already present, skip fetch.
	if commitHash != "" && cm.commitExists(ctx, repoDir, commitHash) {
		return repoDir, commitHash, nil
	}

	// Fetch the requested commit (or HEAD).
	fetchTarget := "HEAD"
	if commitHash != "" {
		fetchTarget = commitHash
	}
	if err := cm.git.ShallowFetch(ctx, repoDir, cloneURL, fetchTarget); err != nil {
		return "", "", fmt.Errorf("fetching from %s: %w", cloneURL, err)
	}

	// Resolve the actual commit hash.
	if commitHash != "" {
		resolvedCommit = commitHash
	} else {
		hash, err := cm.git.RevParse(ctx, repoDir, "FETCH_HEAD")
		if err != nil {
			return "", "", fmt.Errorf("resolving FETCH_HEAD: %w", err)
		}
		resolvedCommit = hash
	}

	return repoDir, resolvedCommit, nil
}

// commitExists checks if a commit exists in the local bare repo cache.
func (cm *CacheManager) commitExists(ctx context.Context, repoDir string, hash string) bool {
	objType, err := cm.git.ObjectType(ctx, repoDir, hash)
	if err != nil {
		return false
	}
	return objType == "commit"
}

// RepoDir returns the cache directory path for a given cache path, without performing any git operations.
func (cm *CacheManager) RepoDir(cachePath string) string {
	return filepath.Join(cm.cacheDir, cachePath)
}

// CheckoutFile reads a single file from the cache at a specific commit.
func (cm *CacheManager) CheckoutFile(ctx context.Context, repoDir string, commitHash string, filePath string) ([]byte, error) {
	out, err := cm.git.ShowFile(ctx, repoDir, commitHash, filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s at %s: %w", filePath, commitHash, err)
	}
	return out, nil
}

// ListFiles lists preset config files in a directory at a specific commit.
// It filters for .alca.*.toml and .alca.*.toml.example patterns.
func (cm *CacheManager) ListFiles(ctx context.Context, repoDir string, commitHash string, dirPath string) ([]string, error) {
	ref := commitHash + ":" + dirPath
	out, err := cm.git.ListTree(ctx, repoDir, ref)
	if err != nil {
		return nil, fmt.Errorf("listing files at %s: %w", ref, err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	var filtered []string
	for _, name := range strings.Split(raw, "\n") {
		if matchPresetFile(name) {
			filtered = append(filtered, name)
		}
	}
	return filtered, nil
}

// matchPresetFile returns true if the filename matches .alca.*.toml or .alca.*.toml.example.
func matchPresetFile(name string) bool {
	if matched, _ := filepath.Match(".alca.*.toml", name); matched {
		return true
	}
	if matched, _ := filepath.Match(".alca.*.toml.example", name); matched {
		return true
	}
	return false
}
