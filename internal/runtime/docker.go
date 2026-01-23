package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
)

// Docker implements the Runtime interface using the Docker CLI.
type Docker struct{}

// NewDocker creates a new Docker runtime instance.
func NewDocker() *Docker {
	return &Docker{}
}

// Name returns the runtime name.
func (d *Docker) Name() string {
	return "Docker"
}

// Available checks if Docker CLI is installed and accessible.
func (d *Docker) Available() bool {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

// Up creates and starts a Docker container.
func (d *Docker) Up(ctx context.Context, cfg *config.Config, projectDir string, progressOut io.Writer) error {
	name := containerName(projectDir)

	// Check if container already exists
	status, err := d.Status(ctx, projectDir)
	if err == nil && status.State == StateRunning {
		progress(progressOut, "→ Container already running: %s\n", name)
		return nil // Already running
	}

	// Remove existing stopped container if any
	if status.State == StateStopped {
		progress(progressOut, "→ Removing stopped container: %s\n", name)
		if err := d.removeContainer(ctx, name); err != nil {
			return fmt.Errorf("failed to remove stopped container: %w", err)
		}
	}

	// Build docker run command
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
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %w: %s", err, string(output))
	}
	progress(progressOut, "→ Container started\n")

	// Run the up command if specified
	if cfg.Commands.Up != "" {
		progress(progressOut, "→ Running setup command...\n")
		execArgs := []string{"exec", name, "sh", "-c", cfg.Commands.Up}
		cmd := exec.CommandContext(ctx, "docker", execArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("up command failed: %w: %s", err, string(output))
		}
	}

	return nil
}

// Down stops and removes the Docker container.
func (d *Docker) Down(ctx context.Context, projectDir string) error {
	containerName := containerName(projectDir)

	// Stop the container
	stopCmd := exec.CommandContext(ctx, "docker", "stop", containerName)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		// Ignore error if container is not running
		if !strings.Contains(string(output), "No such container") {
			return fmt.Errorf("docker stop failed: %w: %s", err, string(output))
		}
	}

	// Remove the container
	return d.removeContainer(ctx, containerName)
}

// Exec runs a command inside the Docker container.
func (d *Docker) Exec(ctx context.Context, projectDir string, command []string) error {
	containerName := containerName(projectDir)

	// Run in workdir, non-interactive
	args := []string{"exec", "-w", "/workspace", containerName}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status returns the current status of the Docker container.
func (d *Docker) Status(ctx context.Context, projectDir string) (ContainerStatus, error) {
	containerName := containerName(projectDir)

	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}}",
		containerName)

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
		Name:      strings.TrimPrefix(parts[2], "/"),
		Image:     parts[3],
		StartedAt: parts[4],
	}, nil
}

// removeContainer removes a container by name.
func (d *Docker) removeContainer(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "No such container") {
			return nil
		}
		return fmt.Errorf("docker rm failed: %w: %s", err, string(output))
	}
	return nil
}

// Reload re-applies configuration by recreating the container.
// This is an experimental feature that stops and restarts the container with updated mounts.
func (d *Docker) Reload(ctx context.Context, cfg *config.Config, projectDir string) error {
	status, err := d.Status(ctx, projectDir)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return ErrNotRunning
	}

	// Stop and remove existing container
	if err := d.Down(ctx, projectDir); err != nil {
		return fmt.Errorf("failed to stop container for reload: %w", err)
	}

	// Start new container with updated configuration (silent reload)
	if err := d.Up(ctx, cfg, projectDir, nil); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}

	return nil
}

// containerName generates a unique container name based on project directory.
// Format: alca-{first 12 chars of sha256 hash}
func containerName(projectDir string) string {
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		absPath = projectDir
	}
	hash := sha256.Sum256([]byte(absPath))
	return "alca-" + hex.EncodeToString(hash[:])[:12]
}
