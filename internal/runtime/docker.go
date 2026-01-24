package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
	"golang.org/x/term"
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
func (d *Docker) Up(ctx context.Context, cfg *config.Config, projectDir string, st *state.State, progressOut io.Writer) error {
	name := st.ContainerName

	// Check if container already exists
	status, err := d.Status(ctx, projectDir, st)
	if err == nil && status.State == StateRunning {
		progress(progressOut, "→ Container already running: %s\n", name)
		return nil // Already running
	}

	// Remove existing stopped container if any
	if status.State == StateStopped {
		progress(progressOut, "→ Removing stopped container: %s\n", status.Name)
		if err := d.removeContainer(ctx, status.Name); err != nil {
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

	// Add resource limits if configured
	if cfg.Resources.Memory != "" {
		args = append(args, "-m", cfg.Resources.Memory)
	}
	if cfg.Resources.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%d", cfg.Resources.CPUs))
	}

	// Add environment variables (all merged envs at container creation)
	for key, env := range cfg.MergedEnvs() {
		expanded := env.Expand(os.Getenv)
		if expanded != "" {
			args = append(args, "-e", key+"="+expanded)
		}
	}

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
func (d *Docker) Down(ctx context.Context, projectDir string, st *state.State) error {
	// Find the container first
	status, err := d.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return nil // Nothing to stop
	}

	containerName := status.Name

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
// For interactive commands, this uses syscall.Exec to replace the current process,
// ensuring proper signal handling (Ctrl+C, etc.) and TTY behavior.
// See AGD-017 for environment variable design.
func (d *Docker) Exec(ctx context.Context, cfg *config.Config, projectDir string, st *state.State, command []string) error {
	// Find the container first
	status, err := d.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State != StateRunning {
		return ErrNotRunning
	}

	containerName := status.Name

	args := []string{"docker", "exec", "-i"}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		args = append(args, "-t")
	}

	// Add environment variables with override_on_enter=true
	for key, env := range cfg.MergedEnvs() {
		if env.OverrideOnEnter {
			expanded := env.Expand(os.Getenv)
			if expanded != "" {
				args = append(args, "-e", key+"="+expanded)
			}
		}
	}

	args = append(args, "-w", cfg.Workdir, containerName)
	args = append(args, command...)

	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found: %w", err)
	}

	// Debug output
	if os.Getenv("ALCA_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "→ Executing: %s\n", strings.Join(args, " "))
	}

	// Replace current process with docker exec for proper signal handling
	return syscall.Exec(dockerPath, args, os.Environ())
}

// Status returns the current status of the Docker container.
func (d *Docker) Status(ctx context.Context, projectDir string, st *state.State) (ContainerStatus, error) {
	if st == nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Try to find by label first
	status, err := d.findContainerByLabel(ctx, st.ProjectID)
	if err == nil && status.State != StateNotFound {
		return status, nil
	}

	// Also try by container name from state
	return d.inspectContainer(ctx, st.ContainerName)
}

// inspectContainer gets container status by name.
func (d *Docker) inspectContainer(ctx context.Context, containerName string) (ContainerStatus, error) {
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
		Name:      strings.TrimPrefix(parts[2], "/"),
		Image:     parts[3],
		StartedAt: parts[4],
	}, nil
}

// findContainerByLabel finds a container by its project label.
func (d *Docker) findContainerByLabel(ctx context.Context, projectID string) (ContainerStatus, error) {
	labelFilter := fmt.Sprintf("label=%s=%s", state.LabelProjectID, projectID)
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a",
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
	return d.inspectContainer(ctx, name)
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
func (d *Docker) Reload(ctx context.Context, cfg *config.Config, projectDir string, st *state.State) error {
	status, err := d.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return ErrNotRunning
	}

	// Stop and remove existing container
	if err := d.Down(ctx, projectDir, st); err != nil {
		return fmt.Errorf("failed to stop container for reload: %w", err)
	}

	// Start new container with updated configuration (silent reload)
	if err := d.Up(ctx, cfg, projectDir, st, nil); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}

	return nil
}

// ListContainers returns all containers managed by alca.
func (d *Docker) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	// Find all containers with alca.project.id label
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a",
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
		info, err := d.getContainerInfo(ctx, name)
		if err != nil {
			continue // Skip containers we can't inspect
		}
		containers = append(containers, info)
	}

	return containers, nil
}

// getContainerInfo gets detailed container information including labels.
func (d *Docker) getContainerInfo(ctx context.Context, name string) (ContainerInfo, error) {
	// Get container details with labels
	format := fmt.Sprintf("{{.State.Status}}|{{.Created}}|{{.Config.Image}}|{{index .Config.Labels \"%s\"}}|{{index .Config.Labels \"%s\"}}",
		state.LabelProjectID, state.LabelProjectPath)

	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", format, name)
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
func (d *Docker) RemoveContainer(ctx context.Context, name string) error {
	return d.removeContainer(ctx, name)
}

