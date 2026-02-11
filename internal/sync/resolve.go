package sync

import (
	"fmt"

	"github.com/spf13/afero"
)

// ContainerExecutor runs commands inside a container.
// The CLI layer provides the implementation (e.g. wrapping docker exec).
type ContainerExecutor interface {
	ExecInContainer(containerID string, cmd []string) error
}

// ResolveLocal resolves a conflict by keeping the local version.
// Deletes the file on the container side via the provided executor.
func ResolveLocal(executor ContainerExecutor, containerID string, containerPath string) error {
	if err := executor.ExecInContainer(containerID, []string{"rm", containerPath}); err != nil {
		return fmt.Errorf("failed to delete container file: %w", err)
	}
	return nil
}

// ResolveContainer resolves a conflict by keeping the container version.
// Deletes the file on the local side so mutagen syncs the container version over.
func ResolveContainer(fs afero.Fs, localPath string) error {
	if err := fs.Remove(localPath); err != nil {
		return fmt.Errorf("failed to delete local file: %w", err)
	}
	return nil
}
