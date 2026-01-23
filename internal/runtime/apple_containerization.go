package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
)

// AppleContainerizationState represents the setup state of Apple Containerization.
// See AGD-011 for fallback strategy based on these states.
type AppleContainerizationState int

const (
	// AppleContainerizationStateReady means Apple Containerization is fully configured and working.
	AppleContainerizationStateReady AppleContainerizationState = iota
	// AppleContainerizationStateNotInstalled means container CLI is not installed.
	// This triggers silent fallback to Docker per AGD-011.
	AppleContainerizationStateNotInstalled
	// AppleContainerizationStateSystemNotRunning means CLI installed but system not started.
	// User needs to run: container system start
	AppleContainerizationStateSystemNotRunning
	// AppleContainerizationStateKernelNotConfigured means system running but kernel not configured.
	// User needs to complete the kernel setup prompt.
	AppleContainerizationStateKernelNotConfigured
	// AppleContainerizationStateNotOnMacOS means we're not running on macOS.
	AppleContainerizationStateNotOnMacOS
)

// Apple implements the Runtime interface using Apple Containerization (macOS 26+).
// See AGD-001 for macOS isolation solution rationale, AGD-010 for naming convention.
type AppleContainerization struct{}

// NewAppleContainerization creates a new Apple Containerization runtime instance.
func NewAppleContainerization() *AppleContainerization {
	return &AppleContainerization{}
}

// Name returns the runtime name.
func (d *AppleContainerization) Name() string {
	return "Apple Containerization"
}

// Available checks if the container CLI is installed and working.
// This is only available on macOS 26+ (Tahoe).
func (d *AppleContainerization) Available() bool {
	reason := d.UnavailableReason()
	return reason == ""
}

// SetupState returns the current setup state of Apple Containerization.
// This provides fine-grained detection for AGD-011 fallback logic.
func (d *AppleContainerization) SetupState() AppleContainerizationState {
	// Only available on macOS
	if runtime.GOOS != "darwin" {
		return AppleContainerizationStateNotOnMacOS
	}

	// Check if container CLI exists
	if _, err := exec.LookPath("container"); err != nil {
		return AppleContainerizationStateNotInstalled
	}

	// Check if container CLI works (version check)
	cmd := exec.Command("container", "--version")
	if err := cmd.Run(); err != nil {
		// CLI exists but doesn't work - treat as not installed
		return AppleContainerizationStateNotInstalled
	}

	// Check system status
	cmd = exec.Command("container", "system", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "not running") || strings.Contains(outputStr, "stopped") {
			return AppleContainerizationStateSystemNotRunning
		}
		// Unknown error checking system status - try container list as fallback
	}

	// Check if containerization is properly configured by trying a simple operation
	cmd = exec.Command("container", "image", "list")
	output, err = cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "kernel") || strings.Contains(outputStr, "not configured") {
			return AppleContainerizationStateKernelNotConfigured
		}
		// System might not be running
		return AppleContainerizationStateSystemNotRunning
	}

	return AppleContainerizationStateReady
}

// UnavailableReason returns a human-readable reason why Apple Containerization is not available.
// Returns empty string if Apple Containerization is available and working.
func (d *AppleContainerization) UnavailableReason() string {
	state := d.SetupState()
	switch state {
	case AppleContainerizationStateReady:
		return ""
	case AppleContainerizationStateNotOnMacOS:
		return "not on macOS"
	case AppleContainerizationStateNotInstalled:
		return "container CLI not installed"
	case AppleContainerizationStateSystemNotRunning:
		return "system not running (run: container system start)"
	case AppleContainerizationStateKernelNotConfigured:
		return "kernel not configured (run: container system kernel set --recommended)"
	default:
		return "unknown state"
	}
}

// Up creates and starts a container using Apple Containerization.
func (d *AppleContainerization) Up(ctx context.Context, cfg *config.Config, projectDir string, progressOut io.Writer) error {
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

	// Build container run command
	// Apple's container CLI uses similar syntax but with some differences
	progress(progressOut, "→ Pulling image: %s\n", cfg.Image)

	args := []string{
		"run", "-d",
		"--name", name,
	}

	// Add mounts using Apple's mount syntax (type=bind,source=...,target=...)
	for _, mount := range cfg.Mounts {
		// Convert Docker-style mount (/src:/dst) to Apple format
		parts := strings.SplitN(mount, ":", 2)
		if len(parts) == 2 {
			args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", parts[0], parts[1]))
		} else {
			// Pass through if already in correct format
			args = append(args, "--mount", mount)
		}
	}

	// Mount project directory
	args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, cfg.Workdir))

	// Add image
	args = append(args, cfg.Image)

	// Add init command to keep container running
	args = append(args, "sleep", "infinity")

	progress(progressOut, "→ Creating container: %s\n", name)
	cmd := exec.CommandContext(ctx, "container", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("container run failed: %w: %s", err, string(output))
	}
	progress(progressOut, "→ Container started\n")

	// Run the up command if specified
	if cfg.Commands.Up != "" {
		progress(progressOut, "→ Running setup command...\n")
		execArgs := []string{"exec", name, "sh", "-c", cfg.Commands.Up}
		cmd := exec.CommandContext(ctx, "container", execArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("up command failed: %w: %s", err, string(output))
		}
	}

	return nil
}

// Down stops and removes the container.
func (d *AppleContainerization) Down(ctx context.Context, projectDir string) error {
	name := containerName(projectDir)

	// Stop the container
	stopCmd := exec.CommandContext(ctx, "container", "stop", name)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		// Ignore error if container is not running
		if !strings.Contains(string(output), "not found") &&
			!strings.Contains(string(output), "No such container") {
			return fmt.Errorf("container stop failed: %w: %s", err, string(output))
		}
	}

	// Remove the container
	return d.removeContainer(ctx, name)
}

// Exec runs a command inside the container.
func (d *AppleContainerization) Exec(ctx context.Context, projectDir string, command []string) error {
	name := containerName(projectDir)

	// For non-interactive exec, don't use -it
	args := []string{"exec", name}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "container", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// containerListItem represents a container in the list output.
type containerListItem struct {
	ID    string `json:"id"`
	Image string `json:"image"`
	State string `json:"state"`
}

// containerInspectResult represents the inspect output.
type containerInspectResult struct {
	Status        string `json:"status"`
	Configuration struct {
		ID string `json:"id"`
	} `json:"configuration"`
}

// Status returns the current status of the container.
func (d *AppleContainerization) Status(ctx context.Context, projectDir string) (ContainerStatus, error) {
	name := containerName(projectDir)

	// Try to get container info using list command with JSON format
	cmd := exec.CommandContext(ctx, "container", "list", "--all", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Parse JSON array output
	var containers []containerListItem
	if err := json.Unmarshal(output, &containers); err != nil {
		// Fallback to simple string check if JSON parsing fails
		if !strings.Contains(string(output), name) {
			return ContainerStatus{State: StateNotFound}, nil
		}
	}

	// Find our container in the list
	var found *containerListItem
	for i := range containers {
		if containers[i].ID == name {
			found = &containers[i]
			break
		}
	}

	if found == nil {
		return ContainerStatus{State: StateNotFound}, nil
	}

	// Parse state
	state := StateUnknown
	switch strings.ToLower(found.State) {
	case "running":
		state = StateRunning
	case "stopped", "exited":
		state = StateStopped
	}

	return ContainerStatus{
		State: state,
		ID:    found.ID,
		Name:  name,
		Image: found.Image,
	}, nil
}

// Reload re-applies configuration by recreating the container.
// This is an experimental feature that stops and restarts the container with updated mounts.
func (d *AppleContainerization) Reload(ctx context.Context, cfg *config.Config, projectDir string) error {
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

// removeContainer removes a container by name.
func (d *AppleContainerization) removeContainer(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "container", "rm", "-f", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "not found") ||
			strings.Contains(string(output), "No such container") {
			return nil
		}
		return fmt.Errorf("container rm failed: %w: %s", err, string(output))
	}
	return nil
}
