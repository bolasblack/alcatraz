package runtime

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
)

// SelectRuntime returns a runtime based on config and availability.
// Implements AGD-011 (fallback strategy) and AGD-012 (runtime config).
//
// When runtime="docker": always use Docker
// When runtime="auto" (default):
//   - macOS: Docker
//   - Linux: Podman > Docker
//
// Returns error if:
//   - runtime="docker" but Docker not available
//   - No runtime available
func SelectRuntime(cfg *config.Config) (Runtime, error) {
	return SelectRuntimeWithOutput(cfg, nil)
}

// SelectRuntimeWithOutput returns a runtime with optional progress output.
func SelectRuntimeWithOutput(cfg *config.Config, progressOut io.Writer) (Runtime, error) {
	runtimeType := cfg.NormalizeRuntime()

	// Handle explicit runtime configuration
	if runtimeType == config.RuntimeDocker {
		docker := NewDocker()
		if !docker.Available() {
			return nil, fmt.Errorf("Docker not available (configured runtime=docker)")
		}
		return docker, nil
	}

	// Auto-detect mode
	switch runtime.GOOS {
	case "linux":
		return selectLinuxRuntime(progressOut)
	default:
		return selectDefaultRuntime(progressOut)
	}
}

// selectLinuxRuntime detects runtime for Linux (Podman > Docker).
func selectLinuxRuntime(progressOut io.Writer) (Runtime, error) {
	// Try Podman first (preferred on Linux)
	podman := NewPodman()
	if podman.Available() {
		return podman, nil
	}

	// Fall back to Docker
	docker := NewDocker()
	if docker.Available() {
		progressStep(progressOut, "Using Docker (Podman not available)\n")
		return docker, nil
	}

	return nil, fmt.Errorf("no container runtime available: neither Podman nor Docker found")
}

// selectDefaultRuntime tries Docker as fallback for unsupported platforms.
func selectDefaultRuntime(progressOut io.Writer) (Runtime, error) {
	docker := NewDocker()
	if docker.Available() {
		return docker, nil
	}
	return nil, fmt.Errorf("no container runtime available: Docker not found")
}

// All returns all supported runtime implementations.
// Useful for listing available runtimes or for testing.
func All() []Runtime {
	return []Runtime{
		NewDocker(),
		NewPodman(),
	}
}

// ByName returns a runtime instance by its display name.
// Returns nil if the name is not recognized.
func ByName(name string) Runtime {
	for _, rt := range All() {
		if rt.Name() == name {
			return rt
		}
	}
	return nil
}

// Available returns all currently available runtimes on this system.
func Available() []Runtime {
	var available []Runtime
	for _, rt := range All() {
		if rt.Available() {
			available = append(available, rt)
		}
	}
	return available
}

// IsOrbStack returns true if Docker is running on OrbStack.
// It checks the Docker info output for "OrbStack" in the OperatingSystem field.
func IsOrbStack() (bool, error) {
	cmd := exec.Command("docker", "info", "--format", "{{.OperatingSystem}}")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get docker info: %w", err)
	}
	return strings.Contains(string(output), "OrbStack"), nil
}


