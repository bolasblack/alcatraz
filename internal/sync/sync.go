package sync

import (
	"time"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// SyncSessionClient provides sync session operations.
// Implemented by the runtime layer; sync module depends only on this interface.
type SyncSessionClient interface {
	ListSessionJSON(sessionName string) ([]byte, error)
	ListSyncSessions(namePrefix string) ([]string, error)
	FlushSyncSession(name string) error
}

// SyncEnv holds dependencies for the sync module (AGD-029 pattern).
type SyncEnv struct {
	Fs       afero.Fs
	Cmd      util.CommandRunner
	Sessions SyncSessionClient
}

// NewSyncEnv creates a new SyncEnv from externally-created dependencies (AGD-029).
func NewSyncEnv(fs afero.Fs, cmd util.CommandRunner, sessions SyncSessionClient) *SyncEnv {
	return &SyncEnv{
		Fs:       fs,
		Cmd:      cmd,
		Sessions: sessions,
	}
}

// ConflictInfo represents a single sync conflict.
type ConflictInfo struct {
	Path           string    `json:"path"`           // Relative file path from project root
	LocalState     string    `json:"localState"`     // "modified", "created", "deleted", "directory"
	ContainerState string    `json:"containerState"` // "modified", "created", "deleted", "directory"
	DetectedAt     time.Time `json:"detectedAt"`     // When first detected
}
