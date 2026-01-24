package runtime

import (
	"fmt"
	"io"
	"runtime"

	"github.com/bolasblack/alcatraz/internal/config"
)

// SelectRuntime returns a runtime based on config and availability.
// Implements AGD-011 (fallback strategy) and AGD-012 (runtime config).
//
// When runtime="docker": always use Docker
// When runtime="auto" (default):
//   - macOS: Apple Containerization > Docker (with AGD-011 fallback rules)
//   - Linux: Podman > Docker
//
// Returns error if:
//   - runtime="docker" but Docker not available
//   - Apple container CLI installed but not ready (guides user to complete setup)
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
	case "darwin":
		return selectDarwinRuntime(progressOut)
	case "linux":
		return selectLinuxRuntime(progressOut)
	default:
		return selectDefaultRuntime(progressOut)
	}
}

// selectDarwinRuntime implements AGD-011 fallback strategy for macOS.
// - container CLI not installed → silent fallback to Docker
// - container CLI installed but not ready → error with setup guidance
// - container CLI ready → use Apple Containerization
func selectDarwinRuntime(progressOut io.Writer) (Runtime, error) {
	apple := NewAppleContainerization()
	state := apple.SetupState()

	switch state {
	case AppleContainerizationStateReady:
		return apple, nil

	case AppleContainerizationStateNotInstalled:
		// Silent fallback to Docker (user hasn't chosen Apple Containerization)
		docker := NewDocker()
		if docker.Available() {
			progress(progressOut, "→ Using Docker (Apple container CLI not installed)\n")
			return docker, nil
		}
		return nil, fmt.Errorf("no container runtime available: Docker not found")

	default:
		// Installed but not ready - user chose Apple Containerization but setup incomplete
		// Return error with guidance per AGD-011
		return nil, fmt.Errorf(
			"Apple container CLI installed but not ready: %s\n\n"+
				"Or to use Docker instead, set in .alca.toml:\n"+
				"  runtime = \"docker\"",
			apple.UnavailableReason())
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
		progress(progressOut, "→ Using Docker (Podman not available)\n")
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

// Detect returns the first available container runtime for the current platform.
// Detection order (see AGD-009 for CLI design decisions):
//   - macOS: Apple Containerization (if available) > Docker
//   - Linux: Podman > Docker
//
// Deprecated: Use SelectRuntime with config for AGD-011 compliant behavior.
// Returns nil if no runtime is available.
func Detect() Runtime {
	return DetectWithOutput(nil)
}

// DetectWithOutput returns the first available container runtime with optional progress output.
// When progressOut is non-nil, prints informative messages about fallback decisions.
// Detection order (see AGD-009 for CLI design decisions):
//   - macOS: Apple Containerization (if available) > Docker
//   - Linux: Podman > Docker
//
// Deprecated: Use SelectRuntimeWithOutput with config for AGD-011 compliant behavior.
// Returns nil if no runtime is available.
func DetectWithOutput(progressOut io.Writer) Runtime {
	switch runtime.GOOS {
	case "darwin":
		return detectDarwinWithOutput(progressOut)
	case "linux":
		return detectLinux()
	default:
		// For other platforms, try Docker as fallback
		docker := NewDocker()
		if docker.Available() {
			return docker
		}
		return nil
	}
}

// detectDarwin returns the preferred runtime for macOS.
// Prefers Apple Containerization (macOS 26+) over Docker.
func detectDarwin() Runtime {
	return detectDarwinWithOutput(nil)
}

// detectDarwinWithOutput returns the preferred runtime for macOS with optional progress output.
// When progressOut is non-nil, prints informative messages about fallback decisions.
func detectDarwinWithOutput(progressOut io.Writer) Runtime {
	// Try Apple Containerization first (macOS 26+)
	apple := NewAppleContainerization()
	if apple.Available() {
		return apple
	}

	// Get reason why Apple Containerization is not available
	reason := apple.UnavailableReason()

	// Fall back to Docker
	docker := NewDocker()
	if docker.Available() {
		if progressOut != nil && reason != "" {
			fmt.Fprintf(progressOut, "→ Apple Containerization not available (%s)\n", reason)
			fmt.Fprintf(progressOut, "→ Falling back to Docker\n")
		}
		return docker
	}

	return nil
}

// detectLinux returns the preferred runtime for Linux.
// Prefers Podman over Docker for rootless container support.
func detectLinux() Runtime {
	// Try Podman first (preferred on Linux)
	podman := NewPodman()
	if podman.Available() {
		return podman
	}

	// Fall back to Docker
	docker := NewDocker()
	if docker.Available() {
		return docker
	}

	return nil
}

// All returns all supported runtime implementations.
// Useful for listing available runtimes or for testing.
func All() []Runtime {
	return []Runtime{
		NewAppleContainerization(),
		NewDocker(),
		NewPodman(),
	}
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
