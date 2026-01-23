package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
)

// Podman implements the Runtime interface using the Podman CLI.
// See AGD-002 for Linux isolation solution rationale.
type Podman struct{}

// NewPodman creates a new Podman runtime instance.
func NewPodman() *Podman {
	return &Podman{}
}

// Name returns the runtime name.
func (p *Podman) Name() string {
	return "Podman"
}

// Available checks if Podman CLI is installed and accessible.
func (p *Podman) Available() bool {
	cmd := exec.Command("podman", "version", "--format", "{{.Version}}")
	return cmd.Run() == nil
}

// Up creates and starts a Podman container.
func (p *Podman) Up(ctx context.Context, cfg *config.Config, projectDir string, progressOut io.Writer) error {
	name := containerName(projectDir)

	// Check if container already exists
	status, err := p.Status(ctx, projectDir)
	if err == nil && status.State == StateRunning {
		progress(progressOut, "→ Container already running: %s\n", name)
		return nil // Already running
	}

	// Remove existing stopped container if any
	if status.State == StateStopped {
		progress(progressOut, "→ Removing stopped container: %s\n", name)
		if err := p.removeContainer(ctx, name); err != nil {
			return fmt.Errorf("failed to remove stopped container: %w", err)
		}
	}

	// Build podman run command
	progress(progressOut, "→ Pulling image: %s\n", cfg.Image)

	args := []string{
		"run", "-d",
		"--name", name,
		"-w", cfg.Workdir,
	}

	// Add mounts
	for _, mount := range cfg.Mounts {
		args = append(args, "-v", mount)
	}

	// Mount project directory
	args = append(args, "-v", fmt.Sprintf("%s:%s", projectDir, cfg.Workdir))

	// Add image
	args = append(args, cfg.Image)

	// Add init command to keep container running
	args = append(args, "sleep", "infinity")

	progress(progressOut, "→ Creating container: %s\n", name)
	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman run failed: %w: %s", err, string(output))
	}
	progress(progressOut, "→ Container started\n")

	// Run the up command if specified
	if cfg.Commands.Up != "" {
		progress(progressOut, "→ Running setup command...\n")
		execArgs := []string{"exec", name, "sh", "-c", cfg.Commands.Up}
		cmd := exec.CommandContext(ctx, "podman", execArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("up command failed: %w: %s", err, string(output))
		}
	}

	return nil
}

// Down stops and removes the Podman container.
func (p *Podman) Down(ctx context.Context, projectDir string) error {
	name := containerName(projectDir)

	// Stop the container
	stopCmd := exec.CommandContext(ctx, "podman", "stop", name)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		// Ignore error if container is not running
		if !strings.Contains(string(output), "no such container") &&
			!strings.Contains(string(output), "No such container") {
			return fmt.Errorf("podman stop failed: %w: %s", err, string(output))
		}
	}

	// Remove the container
	return p.removeContainer(ctx, name)
}

// Exec runs a command inside the Podman container.
func (p *Podman) Exec(ctx context.Context, projectDir string, command []string) error {
	name := containerName(projectDir)

	// For non-interactive exec, don't use -it
	args := []string{"exec", name}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status returns the current status of the Podman container.
func (p *Podman) Status(ctx context.Context, projectDir string) (ContainerStatus, error) {
	name := containerName(projectDir)

	cmd := exec.CommandContext(ctx, "podman", "inspect",
		"--format", "{{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}}",
		name)

	output, err := cmd.Output()
	if err != nil {
		// Container not found
		return ContainerStatus{State: StateNotFound}, nil
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 5 {
		return ContainerStatus{State: StateUnknown}, nil
	}

	state := StateUnknown
	switch parts[0] {
	case "running":
		state = StateRunning
	case "exited", "stopped":
		state = StateStopped
	}

	return ContainerStatus{
		State:     state,
		ID:        parts[1],
		Name:      parts[2],
		Image:     parts[3],
		StartedAt: parts[4],
	}, nil
}

// Reload re-applies configuration by recreating the container.
// This is an experimental feature that stops and restarts the container with updated mounts.
func (p *Podman) Reload(ctx context.Context, cfg *config.Config, projectDir string) error {
	status, err := p.Status(ctx, projectDir)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return ErrNotRunning
	}

	// Stop and remove existing container
	if err := p.Down(ctx, projectDir); err != nil {
		return fmt.Errorf("failed to stop container for reload: %w", err)
	}

	// Start new container with updated configuration (silent reload)
	if err := p.Up(ctx, cfg, projectDir, nil); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}

	return nil
}

// removeContainer removes a container by name.
func (p *Podman) removeContainer(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "podman", "rm", "-f", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no such container") ||
			strings.Contains(string(output), "No such container") {
			return nil
		}
		return fmt.Errorf("podman rm failed: %w: %s", err, string(output))
	}
	return nil
}
