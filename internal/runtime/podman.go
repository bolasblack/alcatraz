package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
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
func (p *Podman) Up(ctx context.Context, cfg *config.Config, projectDir string, st *state.State, progressOut io.Writer) error {
	name := st.ContainerName

	// Check if container already exists
	status, err := p.Status(ctx, projectDir, st)
	if err == nil && status.State == StateRunning {
		progress(progressOut, "→ Container already running: %s\n", name)
		return nil // Already running
	}

	// Remove existing stopped container if any
	if status.State == StateStopped {
		progress(progressOut, "→ Removing stopped container: %s\n", status.Name)
		if err := p.removeContainer(ctx, status.Name); err != nil {
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

	// Add labels for container identity
	for key, value := range st.ContainerLabels(projectDir) {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
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
func (p *Podman) Down(ctx context.Context, projectDir string, st *state.State) error {
	// Find the container first
	status, err := p.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return nil // Nothing to stop
	}

	containerName := status.Name

	// Stop the container
	stopCmd := exec.CommandContext(ctx, "podman", "stop", containerName)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		// Ignore error if container is not running
		if !strings.Contains(string(output), "no such container") &&
			!strings.Contains(string(output), "No such container") {
			return fmt.Errorf("podman stop failed: %w: %s", err, string(output))
		}
	}

	// Remove the container
	return p.removeContainer(ctx, containerName)
}

// Exec runs a command inside the Podman container.
func (p *Podman) Exec(ctx context.Context, projectDir string, st *state.State, command []string) error {
	// Find the container first
	status, err := p.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State != StateRunning {
		return ErrNotRunning
	}

	containerName := status.Name

	// For non-interactive exec, don't use -it
	args := []string{"exec", containerName}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status returns the current status of the Podman container.
func (p *Podman) Status(ctx context.Context, projectDir string, st *state.State) (ContainerStatus, error) {
	if st == nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Try to find by label first
	status, err := p.findContainerByLabel(ctx, st.ProjectID)
	if err == nil && status.State != StateNotFound {
		return status, nil
	}

	// Also try by container name from state
	return p.inspectContainer(ctx, st.ContainerName)
}

// inspectContainer gets container status by name.
func (p *Podman) inspectContainer(ctx context.Context, containerName string) (ContainerStatus, error) {
	cmd := exec.CommandContext(ctx, "podman", "inspect",
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

	st := StateUnknown
	switch parts[0] {
	case "running":
		st = StateRunning
	case "exited", "stopped":
		st = StateStopped
	}

	return ContainerStatus{
		State:     st,
		ID:        parts[1],
		Name:      parts[2],
		Image:     parts[3],
		StartedAt: parts[4],
	}, nil
}

// findContainerByLabel finds a container by its project label.
func (p *Podman) findContainerByLabel(ctx context.Context, projectID string) (ContainerStatus, error) {
	labelFilter := fmt.Sprintf("label=%s=%s", state.LabelProjectID, projectID)
	cmd := exec.CommandContext(ctx, "podman", "ps", "-a",
		"--filter", labelFilter,
		"--format", "{{.Names}}")

	output, err := cmd.Output()
	if err != nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	name := strings.TrimSpace(string(output))
	if name == "" {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Get full status
	return p.inspectContainer(ctx, name)
}

// Reload re-applies configuration by recreating the container.
// This is an experimental feature that stops and restarts the container with updated mounts.
func (p *Podman) Reload(ctx context.Context, cfg *config.Config, projectDir string, st *state.State) error {
	status, err := p.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return ErrNotRunning
	}

	// Stop and remove existing container
	if err := p.Down(ctx, projectDir, st); err != nil {
		return fmt.Errorf("failed to stop container for reload: %w", err)
	}

	// Start new container with updated configuration (silent reload)
	if err := p.Up(ctx, cfg, projectDir, st, nil); err != nil {
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

// ListContainers returns all containers managed by alca.
func (p *Podman) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	// Find all containers with alca.project.id label
	cmd := exec.CommandContext(ctx, "podman", "ps", "-a",
		"--filter", "label="+state.LabelProjectID,
		"--format", "{{.Names}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	names := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(names) == 1 && names[0] == "" {
		return []ContainerInfo{}, nil
	}

	var containers []ContainerInfo
	for _, name := range names {
		if name == "" {
			continue
		}
		info, err := p.getContainerInfo(ctx, name)
		if err != nil {
			continue // Skip containers we can't inspect
		}
		containers = append(containers, info)
	}

	return containers, nil
}

// getContainerInfo gets detailed container information including labels.
func (p *Podman) getContainerInfo(ctx context.Context, name string) (ContainerInfo, error) {
	// Get container details with labels
	format := fmt.Sprintf("{{.State.Status}}|{{.Created}}|{{.Config.Image}}|{{index .Config.Labels \"%s\"}}|{{index .Config.Labels \"%s\"}}",
		state.LabelProjectID, state.LabelProjectPath)

	cmd := exec.CommandContext(ctx, "podman", "inspect", "--format", format, name)
	output, err := cmd.Output()
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("failed to inspect container: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 5 {
		return ContainerInfo{}, fmt.Errorf("unexpected inspect output")
	}

	st := StateUnknown
	switch parts[0] {
	case "running":
		st = StateRunning
	case "exited", "stopped":
		st = StateStopped
	}

	return ContainerInfo{
		Name:        name,
		State:       st,
		CreatedAt:   parts[1],
		Image:       parts[2],
		ProjectID:   parts[3],
		ProjectPath: parts[4],
	}, nil
}

// RemoveContainer removes a container by name.
func (p *Podman) RemoveContainer(ctx context.Context, name string) error {
	return p.removeContainer(ctx, name)
}
