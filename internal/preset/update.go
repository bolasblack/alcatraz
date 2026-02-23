package preset

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// managedFile pairs a local file path with its parsed source info.
type managedFile struct {
	localPath string
	info      *SourceInfo
}

// RunUpdateFlow scans scanDir for .alca.*.toml files with source comments,
// fetches latest versions from their source repos, and overwrites local files.
func RunUpdateFlow(ctx context.Context, env *PresetEnv, cacheDir string, scanDir string, w io.Writer) error {
	// 1. Scan for managed files.
	managed, err := scanManagedFiles(env.Fs, scanDir)
	if err != nil {
		return fmt.Errorf("scanning managed files: %w", err)
	}
	if len(managed) == 0 {
		return nil
	}

	// 2. Group by clone URL.
	groups := groupByCloneURL(managed)

	// 3. Process each repo group.
	cm := NewCacheManager(env, cacheDir)
	for cloneURL, files := range groups {
		if err := updateRepoGroup(ctx, cm, env, cloneURL, files, w); err != nil {
			return err
		}
	}

	return nil
}

// scanManagedFiles reads .alca.*.toml files in dir and returns those with source comments.
func scanManagedFiles(fs afero.Fs, dir string) ([]managedFile, error) {
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var managed []managedFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !matchPresetFile(entry.Name()) {
			continue
		}

		localPath := filepath.Join(dir, entry.Name())
		content, err := afero.ReadFile(fs, localPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", localPath, err)
		}

		info, err := ParseSourceComment(content)
		if err != nil {
			// Malformed source comment — skip with no error (not managed).
			continue
		}
		if info == nil {
			continue
		}

		managed = append(managed, managedFile{localPath: localPath, info: info})
	}

	return managed, nil
}

// groupByCloneURL groups managed files by their clone URL.
func groupByCloneURL(files []managedFile) map[string][]managedFile {
	groups := make(map[string][]managedFile)
	for _, f := range files {
		groups[f.info.CloneURL] = append(groups[f.info.CloneURL], f)
	}
	return groups
}

// updateRepoGroup fetches latest for a single repo and updates all its managed files.
func updateRepoGroup(ctx context.Context, cm *CacheManager, env *PresetEnv, cloneURL string, files []managedFile, w io.Writer) error {
	// Derive cache path from the first file's RawURL.
	// All files in this group share the same clone URL, so any file's RawURL works.
	cachePath, err := deriveCachePath(files[0].info.RawURL)
	if err != nil {
		_, _ = fmt.Fprintf(w, "Warning: cannot derive cache path for %s: %v\n", cloneURL, err)
		return nil
	}

	// Fetch latest (empty commit = HEAD).
	repoDir, resolvedCommit, err := cm.EnsureRepo(ctx, cloneURL, cachePath, "")
	if err != nil {
		// Try to fall back to stale cache.
		repoDir = cm.RepoDir(cachePath)
		exists, dirErr := afero.DirExists(env.Fs, repoDir)
		if dirErr == nil && exists {
			hash, revErr := cm.git.RevParse(ctx, repoDir, "HEAD")
			if revErr == nil {
				resolvedCommit = hash
				_, _ = fmt.Fprintf(w, "Warning: failed to fetch %s: %v. Using cached version.\n", cloneURL, err)
			} else {
				_, _ = fmt.Fprintf(w, "Warning: failed to fetch %s: %v. No cache available, skipping.\n", cloneURL, err)
				return nil
			}
		} else {
			_, _ = fmt.Fprintf(w, "Warning: failed to fetch %s: %v. No cache available, skipping.\n", cloneURL, err)
			return nil
		}
	}

	// Update each file in this group.
	for _, f := range files {
		content, err := cm.CheckoutFile(ctx, repoDir, resolvedCommit, f.info.FilePath)
		if err != nil {
			_, _ = fmt.Fprintf(w, "Warning: %s no longer exists in %s, leaving local file untouched\n", f.info.FilePath, cloneURL)
			continue
		}

		// Overwrite local file with new content + updated source comment.
		if err := WriteSourceComment(env.Fs, f.localPath, content, cloneURL, resolvedCommit, f.info.FilePath); err != nil {
			return fmt.Errorf("writing %s: %w", f.localPath, err)
		}
	}

	return nil
}

// deriveCachePath extracts the cache path from a source comment's RawURL.
// CachePath only needs Host, Protocol, Credentials, RepoPath — so we strip
// the fragment and reuse ParsePresetURL for the clone URL portion.
func deriveCachePath(rawURL string) (string, error) {
	hashIdx := strings.LastIndex(rawURL, "#")
	if hashIdx < 0 {
		return "", fmt.Errorf("no fragment in URL: %s", rawURL)
	}

	parsedURL, err := ParsePresetURL(rawURL[:hashIdx])
	if err != nil {
		return "", err
	}

	return parsedURL.CachePath(""), nil
}
