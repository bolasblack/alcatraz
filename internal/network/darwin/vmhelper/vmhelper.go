// Package vmhelper manages the alcatraz-network-helper container that runs
// nftables inside the container runtime VM on macOS.
// It has no build constraints for testability — all platform-specific behavior
// is injected via DI.
package vmhelper

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

//go:embed entry.sh
var entryScript string

const (
	entryFileName = "entry.sh"
)

// VMHelperEnv provides dependency injection for vmhelper operations.
type VMHelperEnv struct {
	Fs  afero.Fs
	Cmd util.CommandRunner
}

// NewVMHelperEnv creates a VMHelperEnv with externally provided dependencies.
func NewVMHelperEnv(fs afero.Fs, cmd util.CommandRunner) *VMHelperEnv {
	return &VMHelperEnv{
		Fs:  fs,
		Cmd: cmd,
	}
}

// homeDir returns the user's home directory.
func homeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("vmhelper: cannot determine home directory: %w", err)
	}
	return home, nil
}

// entryScriptPath returns the full path to the installed entry.sh.
func entryScriptPath(home string) string {
	return filepath.Join(home, HelperDir, entryFileName)
}

// WriteEntryScript writes entry.sh and creates required directories via Fs.
// This is the pre-commit phase: changes are staged in TransactFs and will
// be flushed to disk when the caller commits.
func WriteEntryScript(env *VMHelperEnv) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	// Create directories
	nftDirPath := filepath.Join(home, shared.NftDirRel)
	helperDirPath := filepath.Join(home, HelperDir)
	for _, dir := range []string{nftDirPath, helperDirPath} {
		if err := env.Fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("vmhelper: failed to create directory %s: %w", dir, err)
		}
	}

	// Write entry.sh
	scriptPath := entryScriptPath(home)
	if err := afero.WriteFile(env.Fs, scriptPath, []byte(entryScript), 0755); err != nil {
		return fmt.Errorf("vmhelper: failed to write entry script: %w", err)
	}

	return nil
}

// InstallHelper creates and starts the alcatraz-network-helper container.
// entry.sh must already be on disk (via WriteEntryScript + commit) before calling this.
func InstallHelper(env *VMHelperEnv, platform runtime.RuntimePlatform, progress shared.ProgressFunc) error {
	progress = shared.SafeProgress(progress)

	// Detect Enhanced Container Isolation (ECI) on Docker Desktop.
	// ECI blocks --pid=host and --net=host, causing silent failures.
	if platform == runtime.PlatformMacDockerDesktop {
		progress("Checking for Enhanced Container Isolation (ECI)...\n")
		_, eciErr := env.Cmd.RunQuiet("docker", "run", "--rm", "--privileged", "--pid=host", "alpine:latest", "true")
		if eciErr != nil {
			return fmt.Errorf("vmhelper: Enhanced Container Isolation (ECI) is enabled in Docker Desktop. ECI blocks --pid=host and --net=host which are required for the network helper. Disable ECI in Docker Desktop settings, or use OrbStack instead")
		}
	}

	home, err := homeDir()
	if err != nil {
		return err
	}

	// Remove existing container (ignore errors if it doesn't exist)
	progress("Removing existing helper container...\n")
	_, _ = env.Cmd.RunQuiet("docker", "rm", "-f", ContainerName)

	// Start container
	progress("Starting helper container...\n")
	filesMount := filepath.Join(home, util.FilesDir) + ":/files"
	platformEnv := "ALCA_PLATFORM=" + string(platform)

	_, err = env.Cmd.Run("docker", "run", "-d",
		"--restart=always",
		"--privileged",
		"--pid=host",
		"--net=host",
		"--name", ContainerName,
		"-e", platformEnv,
		"-v", filesMount,
		ContainerImage,
		"sh", "/files/alcatraz_network_helper/entry.sh",
	)
	if err != nil {
		return fmt.Errorf("vmhelper: failed to start helper container: %w", err)
	}

	// Verify container is running
	progress("Verifying helper container...\n")
	running, err := IsInstalled(env)
	if err != nil {
		return fmt.Errorf("vmhelper: failed to verify container status: %w", err)
	}
	if !running {
		return fmt.Errorf("vmhelper: helper container is not running after start")
	}

	progress("Helper container installed successfully\n")
	return nil
}

// UninstallHelper stops and removes the helper container.
func UninstallHelper(env *VMHelperEnv, progress shared.ProgressFunc) error {
	progress = shared.SafeProgress(progress)

	progress("Stopping helper container...\n")
	_, err := env.Cmd.RunQuiet("docker", "rm", "-f", ContainerName)
	if err != nil {
		return fmt.Errorf("vmhelper: failed to remove helper container: %w", err)
	}

	progress("Helper container removed\n")
	return nil
}

// Reload triggers a rule reload in the helper container via SIGHUP.
func Reload(env *VMHelperEnv) error {
	_, err := env.Cmd.RunQuiet("docker", "exec", ContainerName, "sh", "-c", "kill -HUP 1")
	if err != nil {
		return fmt.Errorf("vmhelper: failed to reload helper: %w", err)
	}
	return nil
}

// IsInstalled checks if the helper container exists and is running.
func IsInstalled(env *VMHelperEnv) (bool, error) {
	output, err := env.Cmd.RunQuiet("docker", "inspect",
		"--format", "{{.State.Running}}", ContainerName)
	if err != nil {
		// Container doesn't exist
		return false, nil
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

// NeedsUpdate checks if the installed entry.sh differs from the embedded one.
func NeedsUpdate(env *VMHelperEnv) (bool, error) {
	home, err := homeDir()
	if err != nil {
		return false, err
	}

	scriptPath := entryScriptPath(home)
	existing, err := afero.ReadFile(env.Fs, scriptPath)
	if err != nil {
		// File doesn't exist or can't be read — needs update
		return true, nil
	}

	return string(existing) != entryScript, nil
}
