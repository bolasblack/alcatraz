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
	"github.com/bolasblack/alcatraz/internal/util"
	"golang.org/x/term"
)

// Constants for container runtime commands.
const (
	// KeepAliveCommand is the command used to keep containers running.
	KeepAliveCommand = "sleep"
	// KeepAliveArg is the argument for the keep-alive command.
	KeepAliveArg = "infinity"
	// EnvDebug is the environment variable for debug mode.
	EnvDebug = "ALCA_DEBUG"
)

// dockerCLICompatibleRuntime provides a common implementation for Docker CLI-compatible container runtimes.
// Both Docker and Podman share this implementation with different command names.
type dockerCLICompatibleRuntime struct {
	displayName string // Human-readable name (e.g., "Docker", "Podman")
	command     string // CLI command (e.g., "docker", "podman")
}

// Name returns the runtime name.
func (r *dockerCLICompatibleRuntime) Name() string {
	return r.displayName
}

// Available checks if the CLI is installed and accessible.
func (r *dockerCLICompatibleRuntime) Available() bool {
	var versionFormat string
	if r.command == "docker" {
		versionFormat = "{{.Server.Version}}"
	} else {
		versionFormat = "{{.Version}}"
	}
	cmd := exec.Command(r.command, "version", "--format", versionFormat)
	return cmd.Run() == nil
}

// Up creates and starts a container.
func (r *dockerCLICompatibleRuntime) Up(ctx context.Context, cfg *config.Config, projectDir string, st *state.State, progressOut io.Writer) error {
	name := st.ContainerName

	// Check if container already exists
	status, err := r.Status(ctx, projectDir, st)
	if err == nil && status.State == StateRunning {
		util.ProgressStep(progressOut, "Container already running: %s\n", name)
		return nil
	}

	// Remove existing stopped container if any
	if status.State == StateStopped {
		util.ProgressStep(progressOut, "Removing stopped container: %s\n", status.Name)
		if err := r.removeContainer(ctx, status.Name); err != nil {
			return fmt.Errorf("failed to remove stopped container: %w", err)
		}
	}

	util.ProgressStep(progressOut, "Pulling image: %s\n", cfg.Image)

	args := r.buildRunArgs(cfg, projectDir, st, name)

	util.ProgressStep(progressOut, "Creating container: %s\n", name)
	cmd := exec.CommandContext(ctx, r.command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s run failed: %w: %s", r.command, err, string(output))
	}
	util.ProgressStep(progressOut, "Container started\n")

	// Run the up command if specified
	if cfg.Commands.Up != "" {
		if err := r.executeUpCommand(ctx, name, cfg.Commands.Up, progressOut); err != nil {
			return err
		}
	}

	return nil
}

// buildRunArgs constructs the arguments for the container run command.
func (r *dockerCLICompatibleRuntime) buildRunArgs(cfg *config.Config, projectDir string, st *state.State, name string) []string {
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

	// Add image and keep-alive command
	args = append(args, cfg.Image, KeepAliveCommand, KeepAliveArg)

	return args
}

// executeUpCommand runs the post-creation setup command.
func (r *dockerCLICompatibleRuntime) executeUpCommand(ctx context.Context, containerName, command string, progressOut io.Writer) error {
	util.ProgressStep(progressOut, "Running setup command...\n")
	execArgs := []string{"exec", containerName, "sh", "-c", command}
	cmd := exec.CommandContext(ctx, r.command, execArgs...)
	cmd.Stdout = progressOut
	cmd.Stderr = progressOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("up command failed: %w", err)
	}
	return nil
}

// Down stops and removes the container.
func (r *dockerCLICompatibleRuntime) Down(ctx context.Context, projectDir string, st *state.State) error {
	status, err := r.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return nil
	}

	containerName := status.Name

	// Stop the container
	stopCmd := exec.CommandContext(ctx, r.command, "stop", containerName)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		if !containsNoSuchContainer(string(output)) {
			return fmt.Errorf("%s stop failed: %w: %s", r.command, err, string(output))
		}
	}

	return r.removeContainer(ctx, containerName)
}

// Exec runs a command inside the container.
// For interactive commands, this uses syscall.Exec to replace the current process.
// See AGD-017 for environment variable design.
func (r *dockerCLICompatibleRuntime) Exec(ctx context.Context, cfg *config.Config, projectDir string, st *state.State, command []string) error {
	status, err := r.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State != StateRunning {
		return ErrNotRunning
	}

	args := r.buildExecArgs(cfg, status.Name, command)

	cliPath, err := exec.LookPath(r.command)
	if err != nil {
		return fmt.Errorf("%s not found: %w", r.command, err)
	}

	if os.Getenv(EnvDebug) != "" {
		fmt.Fprintf(os.Stderr, "→ Executing: %s\n", strings.Join(args, " "))
	}

	return syscall.Exec(cliPath, args, os.Environ())
}

// buildExecArgs constructs the arguments for the container exec command.
func (r *dockerCLICompatibleRuntime) buildExecArgs(cfg *config.Config, containerName string, command []string) []string {
	args := []string{r.command, "exec", "-i"}
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
	return args
}

// Status returns the current status of the container.
func (r *dockerCLICompatibleRuntime) Status(ctx context.Context, projectDir string, st *state.State) (ContainerStatus, error) {
	if st == nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Try to find by label first
	status, err := r.findContainerByLabel(ctx, st.ProjectID)
	if err == nil && status.State != StateNotFound {
		return status, nil
	}

	return r.inspectContainer(ctx, st.ContainerName)
}

// inspectContainer gets container status by name.
func (r *dockerCLICompatibleRuntime) inspectContainer(ctx context.Context, containerName string) (ContainerStatus, error) {
	cmd := exec.CommandContext(ctx, r.command, "inspect",
		"--format", "{{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}}",
		containerName)

	output, err := cmd.Output()
	if err != nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 5 {
		return ContainerStatus{State: StateUnknown}, nil
	}

	return ContainerStatus{
		State:     parseContainerState(parts[0]),
		ID:        parts[1],
		Name:      strings.TrimPrefix(parts[2], "/"),
		Image:     parts[3],
		StartedAt: parts[4],
	}, nil
}

// findContainerByLabel finds a container by its project label.
func (r *dockerCLICompatibleRuntime) findContainerByLabel(ctx context.Context, projectID string) (ContainerStatus, error) {
	labelFilter := state.LabelFilter(projectID)
	cmd := exec.CommandContext(ctx, r.command, "ps", "-a",
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

	return r.inspectContainer(ctx, name)
}

// Reload re-applies configuration by recreating the container.
func (r *dockerCLICompatibleRuntime) Reload(ctx context.Context, cfg *config.Config, projectDir string, st *state.State) error {
	status, err := r.Status(ctx, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return ErrNotRunning
	}

	if err := r.Down(ctx, projectDir, st); err != nil {
		return fmt.Errorf("failed to stop container for reload: %w", err)
	}

	if err := r.Up(ctx, cfg, projectDir, st, nil); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}

	return nil
}

// ListContainers returns all containers managed by alca.
func (r *dockerCLICompatibleRuntime) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, r.command, "ps", "-a",
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
		info, err := r.getContainerInfo(ctx, name)
		if err != nil {
			// Log warning instead of silently ignoring
			util.ProgressStep(os.Stderr, "Warning: failed to inspect container %s: %v\n", name, err)
			continue
		}
		containers = append(containers, info)
	}

	return containers, nil
}

// getContainerInfo gets detailed container information including labels.
func (r *dockerCLICompatibleRuntime) getContainerInfo(ctx context.Context, name string) (ContainerInfo, error) {
	format := fmt.Sprintf("{{.State.Status}}|{{.Created}}|{{.Config.Image}}|{{index .Config.Labels \"%s\"}}|{{index .Config.Labels \"%s\"}}",
		state.LabelProjectID, state.LabelProjectPath)

	cmd := exec.CommandContext(ctx, r.command, "inspect", "--format", format, name)
	output, err := cmd.Output()
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("failed to inspect container: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 5 {
		return ContainerInfo{}, fmt.Errorf("unexpected inspect output")
	}

	return ContainerInfo{
		Name:        name,
		State:       parseContainerState(parts[0]),
		CreatedAt:   parts[1],
		Image:       parts[2],
		ProjectID:   parts[3],
		ProjectPath: parts[4],
	}, nil
}

// RemoveContainer removes a container by name.
func (r *dockerCLICompatibleRuntime) RemoveContainer(ctx context.Context, name string) error {
	return r.removeContainer(ctx, name)
}

// removeContainer removes a container by name (internal).
func (r *dockerCLICompatibleRuntime) removeContainer(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, r.command, "rm", "-f", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if containsNoSuchContainer(string(output)) {
			return nil
		}
		return fmt.Errorf("%s rm failed: %w: %s", r.command, err, string(output))
	}
	return nil
}

// parseContainerState converts a status string to ContainerState.
func parseContainerState(status string) ContainerState {
	switch status {
	case "running":
		return StateRunning
	case "exited", "stopped":
		return StateStopped
	default:
		return StateUnknown
	}
}

// containsNoSuchContainer checks if the output contains "no such container" error.
// Handles both lowercase and capitalized variants from Docker/Podman.
func containsNoSuchContainer(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "no such container")
}
