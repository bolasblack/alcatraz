package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// CacheData represents the cached sync conflict state.
type CacheData struct {
	UpdatedAt time.Time      `json:"updatedAt"`
	Conflicts []ConflictInfo `json:"conflicts"`
}

// ReadCache reads and parses the cache file.
// Returns nil, nil if the cache file does not exist.
func ReadCache(fs afero.Fs, projectRoot string) (*CacheData, error) {
	data, err := afero.ReadFile(fs, cacheFilePath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cache CacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}
	return &cache, nil
}

// WriteCache writes cache data atomically, creating .alca/ dir if needed.
func WriteCache(fs afero.Fs, projectRoot string, data *CacheData) error {
	dir := filepath.Join(projectRoot, ".alca")
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := afero.WriteFile(fs, cacheFilePath(projectRoot), buf, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}
	return nil
}

func cacheFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".alca", "sync-conflicts-cache.json")
}

// detectAndUpdateCache detects conflicts and updates the cache.
// projectID is used to derive the session name prefix.
func detectAndUpdateCache(env *SyncEnv, projectID string, projectRoot string) (*CacheData, error) {
	prefix := fmt.Sprintf("alca-%s-", projectID)
	sessions, err := env.Sessions.ListSyncSessions(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list sync sessions: %w", err)
	}

	var allConflicts []ConflictInfo
	for _, sessionName := range sessions {
		conflicts, err := env.DetectConflicts(sessionName)
		if err != nil {
			return nil, fmt.Errorf("failed to detect conflicts for session %s: %w", sessionName, err)
		}
		allConflicts = append(allConflicts, conflicts...)
	}

	cacheData := &CacheData{
		UpdatedAt: time.Now(),
		Conflicts: allConflicts,
	}

	if err := WriteCache(env.Fs, projectRoot, cacheData); err != nil {
		return nil, err
	}
	return cacheData, nil
}

// SyncUpdateCache updates the cache synchronously and returns fresh data.
func SyncUpdateCache(env *SyncEnv, projectID string, projectRoot string) (*CacheData, error) {
	return detectAndUpdateCache(env, projectID, projectRoot)
}
