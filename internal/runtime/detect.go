package runtime

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/util"
)

// platformCache caches DetectPlatform results per RuntimeEnv instance.
// This avoids repeated shell calls to "docker info" during a single operation.
var (
	platformCacheMu sync.RWMutex
	platformCache   = make(map[*RuntimeEnv]RuntimePlatform)
)

// RuntimePlatform represents the detected platform for mount strategy decisions.
// See AGD-025 for platform-specific mount optimization.
type RuntimePlatform string

const (
	// PlatformLinux represents native Linux (Docker Engine or Podman).
	PlatformLinux RuntimePlatform = "linux"
	// PlatformMacDockerDesktop represents macOS with Docker Desktop.
	PlatformMacDockerDesktop RuntimePlatform = "docker-desktop"
	// PlatformMacOrbStack represents macOS with OrbStack.
	PlatformMacOrbStack RuntimePlatform = "orbstack"
)

// DetectPlatform returns the current runtime platform.
// Used for deciding mount strategy (bind mount vs Mutagen sync).
// See AGD-025 for platform detection rationale.
// Results are cached per RuntimeEnv instance to avoid repeated shell calls.
func DetectPlatform(env *RuntimeEnv) RuntimePlatform {
	// Fast path for Linux - no shell calls needed
	if runtime.GOOS == "linux" {
		return PlatformLinux
	}

	// Check cache first
	platformCacheMu.RLock()
	if cached, ok := platformCache[env]; ok {
		platformCacheMu.RUnlock()
		return cached
	}
	platformCacheMu.RUnlock()

	// Detect platform (requires shell call)
	var platform RuntimePlatform
	isOrb, err := IsOrbStack(env)
	if err == nil && isOrb {
		platform = PlatformMacOrbStack
	} else {
		platform = PlatformMacDockerDesktop
	}

	// Cache the result
	platformCacheMu.Lock()
	platformCache[env] = platform
	platformCacheMu.Unlock()

	return platform
}

// IsDarwin returns true if the platform is macOS (OrbStack or Docker Desktop).
func IsDarwin(platform RuntimePlatform) bool {
	return platform == PlatformMacOrbStack || platform == PlatformMacDockerDesktop
}

// ShouldUseMutagen determines if Mutagen sync should be used for a mount.
// Decision table from AGD-025:
//
// | Platform              | Condition    | Use Mutagen |
// |-----------------------|--------------|-------------|
// | Linux                 | Has excludes | Yes         |
// | Linux                 | No excludes  | No          |
// | macOS + Docker Desktop| Always       | Yes         |
// | macOS + OrbStack      | Has excludes | Yes         |
// | macOS + OrbStack      | No excludes  | No          |
//
// Rationale:
// - Docker Desktop has poor bind mount performance (~35%), Mutagen brings it to ~90-95%
// - OrbStack already achieves 75-95% native performance, Mutagen overhead unnecessary without excludes
// - Linux bind mounts are native performance (100%), Mutagen adds sync latency (50-200ms)
func ShouldUseMutagen(platform RuntimePlatform, hasExcludes bool) bool {
	switch platform {
	case PlatformMacDockerDesktop:
		// Always use Mutagen on Docker Desktop for performance
		return true
	case PlatformMacOrbStack, PlatformLinux:
		// Only use Mutagen when excludes are needed
		return hasExcludes
	default:
		// Unknown platform: use Mutagen if excludes needed
		return hasExcludes
	}
}

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
func SelectRuntime(env *RuntimeEnv, cfg *config.Config) (Runtime, error) {
	return SelectRuntimeWithOutput(env, cfg, nil)
}

// SelectRuntimeWithOutput returns a runtime with optional progress output.
func SelectRuntimeWithOutput(env *RuntimeEnv, cfg *config.Config, progressOut io.Writer) (Runtime, error) {
	runtimeType := cfg.NormalizeRuntime()

	// Handle explicit runtime configuration
	if runtimeType == config.RuntimeDocker {
		docker := NewDocker()
		if !docker.Available(env) {
			return nil, fmt.Errorf("Docker not available (configured runtime=docker)")
		}
		return docker, nil
	}

	// Auto-detect mode
	switch runtime.GOOS {
	case "linux":
		return selectLinuxRuntime(env, progressOut)
	default:
		return selectDefaultRuntime(env, progressOut)
	}
}

// selectLinuxRuntime detects runtime for Linux (Podman > Docker).
func selectLinuxRuntime(env *RuntimeEnv, progressOut io.Writer) (Runtime, error) {
	// Try Podman first (preferred on Linux)
	podman := NewPodman()
	if podman.Available(env) {
		return podman, nil
	}

	// Fall back to Docker
	docker := NewDocker()
	if docker.Available(env) {
		util.ProgressStep(progressOut, "Using Docker (Podman not available)\n")
		return docker, nil
	}

	return nil, fmt.Errorf("no container runtime available: neither Podman nor Docker found")
}

// selectDefaultRuntime tries Docker as fallback for unsupported platforms.
func selectDefaultRuntime(env *RuntimeEnv, progressOut io.Writer) (Runtime, error) {
	docker := NewDocker()
	if docker.Available(env) {
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

// IsOrbStack returns true if Docker is running on OrbStack.
// It checks the Docker info output for "OrbStack" in the OperatingSystem field.
func IsOrbStack(env *RuntimeEnv) (bool, error) {
	output, err := env.Cmd.RunQuiet("docker", "info", "--format", "{{.OperatingSystem}}")
	if err != nil {
		return false, fmt.Errorf("failed to get docker info: %w", err)
	}
	return strings.Contains(string(output), "OrbStack"), nil
}

// IsRootlessPodman returns true if Podman is running in rootless mode.
// See AGD-025 for why rootless Podman blocks mount excludes.
func IsRootlessPodman(env *RuntimeEnv) (bool, error) {
	output, err := env.Cmd.RunQuiet("podman", "info", "--format", "{{.Host.Security.Rootless}}")
	if err != nil {
		return false, fmt.Errorf("failed to get podman info: %w", err)
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

// ErrMutagenNotFound is returned when Mutagen is required but not installed.
var ErrMutagenNotFound = fmt.Errorf("mutagen is required but not installed.\n\n" +
	"Install Mutagen: https://mutagen.io/documentation/introduction/installation/\n\n" +
	"Mutagen is needed when mount excludes are configured (workdir_exclude or mounts.exclude)")

// minMutagenMacOS is the minimum Mutagen version required on macOS.
// v0.18.0 has a known protocol handshake bug on macOS (mutagen-io/mutagen#531).
var minMutagenMacOS = [3]int{0, 18, 1}

// ValidateMutagenAvailable checks if Mutagen is needed and available.
// Returns ErrMutagenNotFound if not installed, or an error if the version is
// too old on macOS (v0.18.0 has a known handshake bug).
func ValidateMutagenAvailable(env *RuntimeEnv, cfg *config.Config) error {
	platform := DetectPlatform(env)

	needsMutagen := false
	for _, mount := range cfg.Mounts {
		if ShouldUseMutagen(platform, mount.HasExcludes()) {
			needsMutagen = true
			break
		}
	}
	if !needsMutagen {
		return nil
	}

	output, err := env.Cmd.RunQuiet("mutagen", "version")
	if err != nil {
		return ErrMutagenNotFound
	}

	// On macOS, enforce minimum version due to protocol handshake bug in v0.18.0
	if platform == PlatformMacDockerDesktop || platform == PlatformMacOrbStack {
		version := strings.TrimSpace(string(output))
		if err := checkMutagenMinVersion(version, minMutagenMacOS); err != nil {
			return err
		}
	}

	return nil
}

// checkMutagenMinVersion parses a semver string and checks against minimum.
func checkMutagenMinVersion(version string, min [3]int) error {
	var major, minor, patch int
	if _, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		// Can't parse version, don't block
		return nil
	}

	current := [3]int{major, minor, patch}
	for i := 0; i < 3; i++ {
		if current[i] > min[i] {
			return nil
		}
		if current[i] < min[i] {
			return fmt.Errorf("mutagen %s is not supported on macOS (protocol handshake bug).\n\n"+
				"Please upgrade: brew upgrade mutagen\n"+
				"Minimum required: %d.%d.%d\n\n"+
				"See: https://github.com/mutagen-io/mutagen/issues/531",
				version, min[0], min[1], min[2])
		}
	}
	return nil // equal
}

// ErrRootlessPodmanExcludes is returned when mount excludes are configured on rootless Podman.
var ErrRootlessPodmanExcludes = fmt.Errorf("mount excludes not supported on rootless Podman")

// ValidateMountExcludes checks if mount excludes can be used with the current runtime.
// Returns ErrRootlessPodmanExcludes if excludes are configured on rootless Podman.
// See AGD-025 for Mutagen + rootless Podman compatibility issues.
func ValidateMountExcludes(env *RuntimeEnv, rt Runtime, cfg *config.Config) error {
	// Only check for Podman
	if rt.Name() != "Podman" {
		return nil
	}

	// Check if any mount has excludes
	hasExcludes := false
	for _, mount := range cfg.Mounts {
		if mount.HasExcludes() {
			hasExcludes = true
			break
		}
	}
	if !hasExcludes {
		return nil
	}

	// Check if rootless
	isRootless, err := IsRootlessPodman(env)
	if err != nil {
		// If we can't determine, assume it's OK (fail open)
		return nil
	}

	if isRootless {
		return ErrRootlessPodmanExcludes
	}

	return nil
}
