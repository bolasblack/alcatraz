// Package runtime provides container runtime abstraction for Alcatraz.
// It supports multiple container runtimes including Docker and Podman.
package runtime

import (
	"context"
	"errors"
	"io"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

// RuntimeEnv contains runtime environment dependencies.
// Used for dependency injection to enable testing.
type RuntimeEnv struct {
	Cmd util.CommandRunner
}

// NewRuntimeEnv creates a new RuntimeEnv with the given CommandRunner.
func NewRuntimeEnv(cmd util.CommandRunner) *RuntimeEnv {
	return &RuntimeEnv{Cmd: cmd}
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
	StateUnknown  ContainerState = "unknown"
	StateRunning  ContainerState = "running"
	StateStopped  ContainerState = "stopped"
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

// ContainerInfo contains detailed information about a container for listing.
type ContainerInfo struct {
	Name        string
	State       ContainerState
	ProjectID   string
	ProjectPath string
	CreatedAt   string
	Image       string
}

// Runtime defines the interface for container runtime operations.
type Runtime interface {
	// Name returns the human-readable name of this runtime (e.g., "Docker", "Podman").
	Name() string

	// Available checks if this runtime is installed and accessible.
	Available(ctx context.Context, env *RuntimeEnv) bool

	// Up creates and starts a container based on the given configuration.
	// The projectDir is used to generate a unique container name.
	// The progressOut writer receives progress messages; may be nil to suppress output.
	// The state provides container identity (name, labels) that survives directory moves.
	Up(ctx context.Context, env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State, progressOut io.Writer) error

	// Down stops and removes the container for the given project directory.
	// The state provides container identity for lookup.
	Down(ctx context.Context, env *RuntimeEnv, projectDir string, st *state.State) error

	// Exec runs a command inside the container for the given project directory.
	// The state provides container identity for lookup.
	// The config provides environment variables with override_on_enter support.
	Exec(ctx context.Context, env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State, command []string) error

	// Status returns the current status of the container for the given project directory.
	// The state provides container identity for lookup. If state is nil, uses legacy name lookup.
	Status(ctx context.Context, env *RuntimeEnv, projectDir string, st *state.State) (ContainerStatus, error)

	// Reload re-applies mounts without rebuilding the container.
	// This is an experimental feature - see AGD-009 for design rationale.
	Reload(ctx context.Context, env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State) error

	// ListContainers returns all containers managed by alca (those with alca.project.id label).
	ListContainers(ctx context.Context, env *RuntimeEnv) ([]ContainerInfo, error)

	// RemoveContainer removes a container by name.
	RemoveContainer(ctx context.Context, env *RuntimeEnv, name string) error

	// GetContainerIP returns the IP address of a running container.
	// Used by firewall rules to restrict container network access.
	GetContainerIP(ctx context.Context, env *RuntimeEnv, containerName string) (string, error)
}
