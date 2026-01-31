package runtime

import (
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
func (r *dockerCLICompatibleRuntime) Available(env *RuntimeEnv) bool {
	var versionFormat string
	if r.command == "docker" {
		versionFormat = "{{.Server.Version}}"
	} else {
		versionFormat = "{{.Version}}"
	}

	_, err := env.Cmd.Run(r.command, "version", "--format", versionFormat)
	return err == nil
}

// Up creates and starts a container.
func (r *dockerCLICompatibleRuntime) Up(env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State, progressOut io.Writer) error {
	// Validate mount excludes compatibility (blocks rootless Podman + excludes)
	// See AGD-025 for rationale
	if err := ValidateMountExcludes(env, r, cfg); err != nil {
		return fmt.Errorf("%w: remove exclude config, use rootful Podman, or use Docker", err)
	}

	name := st.ContainerName

	// Check if container already exists
	status, err := r.Status(env, projectDir, st)
	if err == nil && status.State == StateRunning {
		util.ProgressStep(progressOut, "Container already running: %s\n", name)
		return nil
	}

	// Start existing stopped container (no config drift - see up.go flow)
	// If there was config drift, rebuildContainerIfNeeded() would have removed
	// the container before calling Up(), so StateStopped means no drift.
	if status.State == StateStopped {
		util.ProgressStep(progressOut, "Starting stopped container: %s\n", status.Name)
		if err := r.startContainer(env, status.Name); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}
		util.ProgressStep(progressOut, "Container started\n")

		// Re-setup Mutagen syncs for stopped container restart
		// Container ID may have changed, need to refresh syncs
		if err := r.setupMutagenSyncs(env, cfg, st, name, projectDir, progressOut); err != nil {
			return fmt.Errorf("failed to setup Mutagen syncs: %w", err)
		}

		return nil
	}

	util.ProgressStep(progressOut, "Pulling image: %s\n", cfg.Image)

	args := r.buildRunArgs(env, cfg, projectDir, st, name)

	util.ProgressStep(progressOut, "Creating container: %s\n", name)
	output, err := env.Cmd.Run(r.command, args...)
	if err != nil {
		return fmt.Errorf("%s run failed: %w: %s", r.command, err, string(output))
	}
	util.ProgressStep(progressOut, "Container started\n")

	// Setup Mutagen syncs for mounts that require it
	// See AGD-025 for platform-specific mount optimization
	if err := r.setupMutagenSyncs(env, cfg, st, name, projectDir, progressOut); err != nil {
		return fmt.Errorf("failed to setup Mutagen syncs: %w", err)
	}

	// Run the up command if specified
	if cfg.Commands.Up != "" {
		if err := r.executeUpCommand(env, name, cfg.Commands.Up, progressOut); err != nil {
			return err
		}
	}

	return nil
}

// buildRunArgs constructs the arguments for the container run command.
func (r *dockerCLICompatibleRuntime) buildRunArgs(env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State, name string) []string {
	args := []string{
		"run", "-d",
		"--name", name,
		"-w", cfg.Workdir,
	}

	// Add labels for container identity
	for key, value := range st.ContainerLabels(projectDir) {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	// Add mounts (only those not requiring Mutagen sync)
	// Mounts with excludes are handled separately via Mutagen.
	// See AGD-025 for mount strategy decisions.
	// Note: cfg.Mounts[0] is the workdir mount (Source="."), resolved to projectDir here.
	platform := DetectPlatform(env)
	for _, mount := range cfg.Mounts {
		if ShouldUseMutagen(platform, mount.HasExcludes()) {
			// Skip - will be handled by Mutagen sync in setupMutagenSyncs()
			continue
		}
		// Resolve "." source to projectDir (workdir mount normalized in config)
		source := mount.Source
		if source == "." {
			source = projectDir
		}
		mountStr := fmt.Sprintf("%s:%s", source, mount.Target)
		if mount.Readonly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	// Add resource limits if configured
	if cfg.Resources.Memory != "" {
		args = append(args, "-m", cfg.Resources.Memory)
	}
	if cfg.Resources.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%d", cfg.Resources.CPUs))
	}

	// Add environment variables (all merged envs at container creation)
	for key, ev := range cfg.MergedEnvs() {
		expanded := ev.Expand(os.Getenv)
		if expanded != "" {
			args = append(args, "-e", key+"="+expanded)
		}
	}

	// Add image and keep-alive command
	args = append(args, cfg.Image, KeepAliveCommand, KeepAliveArg)

	return args
}

// executeUpCommand runs the post-creation setup command.
func (r *dockerCLICompatibleRuntime) executeUpCommand(env *RuntimeEnv, containerName, command string, progressOut io.Writer) error {
	util.ProgressStep(progressOut, "Running setup command...\n")
	execArgs := []string{"exec", containerName, "sh", "-c", command}
	output, err := env.Cmd.Run(r.command, execArgs...)
	if err != nil {
		return fmt.Errorf("up command failed: %w: %s", err, string(output))
	}
	return nil
}

// setupMutagenSyncs creates Mutagen sync sessions for mounts that require it.
// See AGD-025 for platform-specific mount optimization decisions.
func (r *dockerCLICompatibleRuntime) setupMutagenSyncs(env *RuntimeEnv, cfg *config.Config, st *state.State, containerName, projectDir string, progressOut io.Writer) error {
	platform := DetectPlatform(env)

	// First, terminate any existing syncs for this project to avoid duplicates
	if err := TerminateProjectSyncs(env, st.ProjectID); err != nil {
		// Log warning but continue - old syncs may not exist
		util.ProgressStep(progressOut, "Warning: failed to clean up old Mutagen syncs: %v\n", err)
	}

	// Get container ID for Mutagen target URL
	containerID, err := r.getContainerID(env, containerName)
	if err != nil {
		return fmt.Errorf("failed to get container ID: %w", err)
	}

	// Setup syncs for mounts that require Mutagen
	for i, mount := range cfg.Mounts {
		if !ShouldUseMutagen(platform, mount.HasExcludes()) {
			continue
		}

		// Resolve "." source to projectDir (workdir mount normalized in config)
		source := mount.Source
		if source == "." {
			source = projectDir
		}

		util.ProgressStep(progressOut, "Setting up Mutagen sync for %s -> %s\n", source, mount.Target)

		sync := MutagenSync{
			Name:    MutagenSessionName(st.ProjectID, i),
			Source:  source,
			Target:  MutagenTarget(containerID, mount.Target),
			Ignores: mount.Exclude,
		}

		if err := sync.Create(env); err != nil {
			return fmt.Errorf("failed to create Mutagen sync for %s: %w", source, err)
		}
	}

	return nil
}

// getContainerID returns the container ID for a given container name.
func (r *dockerCLICompatibleRuntime) getContainerID(env *RuntimeEnv, containerName string) (string, error) {
	output, err := env.Cmd.Run(r.command, "inspect", "--format", "{{.Id}}", containerName)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Down stops and removes the container.
func (r *dockerCLICompatibleRuntime) Down(env *RuntimeEnv, projectDir string, st *state.State) error {
	status, err := r.Status(env, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		// Still try to clean up any orphaned Mutagen syncs
		if st != nil {
			_ = TerminateProjectSyncs(env, st.ProjectID)
		}
		return nil
	}

	containerName := status.Name

	// Terminate Mutagen syncs before stopping container
	// See AGD-025 for Mutagen integration design
	if st != nil {
		if err := TerminateProjectSyncs(env, st.ProjectID); err != nil {
			// Log warning but continue with container removal
			// Mutagen sessions will be orphaned but can be cleaned up manually
			util.ProgressStep(nil, "Warning: failed to terminate Mutagen syncs: %v\n", err)
		}
	}

	// Stop the container
	output, err := env.Cmd.Run(r.command, "stop", containerName)
	if err != nil {
		if !containsNoSuchContainer(string(output)) {
			return fmt.Errorf("%s stop failed: %w: %s", r.command, err, string(output))
		}
	}

	return r.removeContainer(env, containerName)
}

// Exec runs a command inside the container.
// For interactive commands, this uses syscall.Exec to replace the current process.
// See AGD-017 for environment variable design.
func (r *dockerCLICompatibleRuntime) Exec(env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State, command []string) error {
	status, err := r.Status(env, projectDir, st)
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
func (r *dockerCLICompatibleRuntime) Status(env *RuntimeEnv, projectDir string, st *state.State) (ContainerStatus, error) {
	if st == nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Primary: find by label (authoritative for labeled containers)
	status, err := r.findContainerByLabel(env, st.ProjectID)
	if err == nil && status.State != StateNotFound {
		return status, nil
	}

	// Fallback: inspect by name (backward compatibility)
	return r.inspectContainer(env, st.ContainerName)
}

// inspectContainer gets container status by name.
func (r *dockerCLICompatibleRuntime) inspectContainer(env *RuntimeEnv, containerName string) (ContainerStatus, error) {
	output, err := env.Cmd.Run(r.command, "inspect",
		"--format", "{{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}}",
		containerName)
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
func (r *dockerCLICompatibleRuntime) findContainerByLabel(env *RuntimeEnv, projectID string) (ContainerStatus, error) {
	labelFilter := state.LabelFilter(projectID)
	output, err := env.Cmd.Run(r.command, "ps", "-a",
		"--filter", labelFilter,
		"--format", "{{.Names}}")
	if err != nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	name := strings.TrimSpace(string(output))
	if name == "" {
		return ContainerStatus{State: StateNotFound}, nil
	}

	return r.inspectContainer(env, name)
}

// Reload re-applies configuration by recreating the container.
func (r *dockerCLICompatibleRuntime) Reload(env *RuntimeEnv, cfg *config.Config, projectDir string, st *state.State) error {
	status, err := r.Status(env, projectDir, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == StateNotFound {
		return ErrNotRunning
	}

	if err := r.Down(env, projectDir, st); err != nil {
		return fmt.Errorf("failed to stop container for reload: %w", err)
	}

	if err := r.Up(env, cfg, projectDir, st, nil); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}

	return nil
}

// ListContainers returns all containers managed by alca.
// Uses batch inspect to avoid N+1 query pattern (single docker inspect call for all containers).
func (r *dockerCLICompatibleRuntime) ListContainers(env *RuntimeEnv) ([]ContainerInfo, error) {
	// Get names of all alca-managed containers
	output, err := env.Cmd.Run(r.command, "ps", "-a",
		"--filter", "label="+state.LabelProjectID,
		"--format", "{{.Names}}")
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
	return r.batchInspectContainers(env, validNames)
}

// batchInspectContainers inspects multiple containers in a single docker/podman call.
// This avoids the N+1 query pattern where we'd call inspect separately for each container.
func (r *dockerCLICompatibleRuntime) batchInspectContainers(env *RuntimeEnv, names []string) ([]ContainerInfo, error) {
	// Build format string for inspect output
	// Using a unique separator (|||) to avoid conflicts with data values
	format := fmt.Sprintf("{{.Name}}|||{{.State.Status}}|||{{.Created}}|||{{.Config.Image}}|||{{index .Config.Labels \"%s\"}}|||{{index .Config.Labels \"%s\"}}",
		state.LabelProjectID, state.LabelProjectPath)

	// Build args: inspect --format <format> name1 name2 name3 ...
	args := []string{"inspect", "--format", format}
	args = append(args, names...)

	output, err := env.Cmd.Run(r.command, args...)
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
func (r *dockerCLICompatibleRuntime) RemoveContainer(env *RuntimeEnv, name string) error {
	return r.removeContainer(env, name)
}

// removeContainer removes a container by name (internal).
func (r *dockerCLICompatibleRuntime) removeContainer(env *RuntimeEnv, name string) error {
	output, err := env.Cmd.Run(r.command, "rm", "-f", name)
	if err != nil {
		if containsNoSuchContainer(string(output)) {
			return nil
		}
		return fmt.Errorf("%s rm failed: %w: %s", r.command, err, string(output))
	}
	return nil
}

// startContainer starts a stopped container by name.
func (r *dockerCLICompatibleRuntime) startContainer(env *RuntimeEnv, name string) error {
	output, err := env.Cmd.Run(r.command, "start", name)
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
