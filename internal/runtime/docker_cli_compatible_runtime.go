package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
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

	// Start existing stopped container (no config drift - see up.go flow)
	// If there was config drift, rebuildContainerIfNeeded() would have removed
	// the container before calling Up(), so StateStopped means no drift.
	if status.State == StateStopped {
		util.ProgressStep(progressOut, "Starting stopped container: %s\n", status.Name)
		if err := r.startContainer(ctx, status.Name); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}
		util.ProgressStep(progressOut, "Container started\n")
		return nil
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
//
// Dual lookup strategy:
//  1. Label-based lookup (preferred): Searches for containers with alca.project.id label
//     matching st.ProjectID. This is the authoritative method since labels survive
//     container renames and provide reliable project association.
//  2. Name-based fallback: If label lookup fails, falls back to inspecting by
//     st.ContainerName. This handles:
//     - Legacy containers created before labels were introduced
//     - Edge cases where label filter fails but container exists
//
// This dual approach ensures backward compatibility while preferring the more robust
// label-based identification for newer containers.
func (r *dockerCLICompatibleRuntime) Status(ctx context.Context, projectDir string, st *state.State) (ContainerStatus, error) {
	if st == nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Primary: find by label (authoritative for labeled containers)
	status, err := r.findContainerByLabel(ctx, st.ProjectID)
	if err == nil && status.State != StateNotFound {
		return status, nil
	}

	// Fallback: inspect by name (backward compatibility)
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
// Uses batch inspect to avoid N+1 query pattern (single docker inspect call for all containers).
func (r *dockerCLICompatibleRuntime) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	// Get names of all alca-managed containers
	psCmd := exec.CommandContext(ctx, r.command, "ps", "-a",
		"--filter", "label="+state.LabelProjectID,
		"--format", "{{.Names}}")

	output, err := psCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	names := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(names) == 1 && names[0] == "" {
		return []ContainerInfo{}, nil
	}

	// Filter empty names
	validNames := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" {
			validNames = append(validNames, name)
		}
	}

	if len(validNames) == 0 {
		return []ContainerInfo{}, nil
	}

	// Batch inspect all containers in a single call
	return r.batchInspectContainers(ctx, validNames)
}

// batchInspectContainers inspects multiple containers in a single docker/podman call.
// This avoids the N+1 query pattern where we'd call inspect separately for each container.
func (r *dockerCLICompatibleRuntime) batchInspectContainers(ctx context.Context, names []string) ([]ContainerInfo, error) {
	// Build format string for inspect output
	// Using a unique separator (|||) to avoid conflicts with data values
	format := fmt.Sprintf("{{.Name}}|||{{.State.Status}}|||{{.Created}}|||{{.Config.Image}}|||{{index .Config.Labels \"%s\"}}|||{{index .Config.Labels \"%s\"}}",
		state.LabelProjectID, state.LabelProjectPath)

	// Build args: inspect --format <format> name1 name2 name3 ...
	args := []string{"inspect", "--format", format}
	args = append(args, names...)

	cmd := exec.CommandContext(ctx, r.command, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect containers: %w", err)
	}

	// Parse output - one line per container
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	containers := make([]ContainerInfo, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|||")
		if len(parts) < 6 {
			// Log warning and skip malformed entries
			util.ProgressStep(os.Stderr, "Warning: unexpected inspect output format: %s\n", line)
			continue
		}

		containers = append(containers, ContainerInfo{
			Name:        strings.TrimPrefix(parts[0], "/"),
			State:       parseContainerState(parts[1]),
			CreatedAt:   parts[2],
			Image:       parts[3],
			ProjectID:   parts[4],
			ProjectPath: parts[5],
		})
	}

	return containers, nil
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

// startContainer starts a stopped container by name.
func (r *dockerCLICompatibleRuntime) startContainer(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, r.command, "start", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s start failed: %w: %s", r.command, err, string(output))
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
