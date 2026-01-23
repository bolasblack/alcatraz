// Package runtime provides container runtime abstraction for Alcatraz.
// It supports multiple container runtimes including Docker, Podman, and Apple Container.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/bolasblack/alcatraz/internal/config"
)

// progress writes a progress message to the given writer.
// If writer is nil, the message is silently discarded.
func progress(w io.Writer, format string, args ...any) {
	if w != nil {
		fmt.Fprintf(w, format, args...)
	}
}

// Common errors returned by runtime implementations.
var (
	ErrNotAvailable    = errors.New("runtime not available")
	ErrContainerExists = errors.New("container already exists")
	ErrNotRunning      = errors.New("container is not running")
)

// ContainerState represents the state of a container.
type ContainerState string

const (
	StateUnknown ContainerState = "unknown"
	StateRunning ContainerState = "running"
	StateStopped ContainerState = "stopped"
	StateNotFound ContainerState = "not_found"
)

// ContainerStatus contains status information about a container.
type ContainerStatus struct {
	State     ContainerState
	ID        string
	Name      string
	Image     string
	StartedAt string
}

// Runtime defines the interface for container runtime operations.
type Runtime interface {
	// Name returns the human-readable name of this runtime (e.g., "Docker", "Podman").
	Name() string

	// Available checks if this runtime is installed and accessible.
	Available() bool

	// Up creates and starts a container based on the given configuration.
	// The projectDir is used to generate a unique container name.
	// The progressOut writer receives progress messages; may be nil to suppress output.
	Up(ctx context.Context, cfg *config.Config, projectDir string, progressOut io.Writer) error

	// Down stops and removes the container for the given project directory.
	Down(ctx context.Context, projectDir string) error

	// Exec runs a command inside the container for the given project directory.
	Exec(ctx context.Context, projectDir string, command []string) error

	// Status returns the current status of the container for the given project directory.
	Status(ctx context.Context, projectDir string) (ContainerStatus, error)

	// Reload re-applies mounts without rebuilding the container.
	// This is an experimental feature - see AGD-009 for design rationale.
	Reload(ctx context.Context, cfg *config.Config, projectDir string) error
}
