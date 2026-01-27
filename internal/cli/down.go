package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	goruntime "runtime"

	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the sandbox environment",
	Long:  `Stop the running Alcatraz sandbox environment.`,
	RunE:  runDown,
}

// runDown stops and removes the container.
// See AGD-009 for CLI workflow design.
func runDown(cmd *cobra.Command, args []string) error {
	var out io.Writer = os.Stdout

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Load config (optional) and select runtime
	cfg, rt, err := loadConfigAndRuntimeOptional(cwd)
	if err != nil {
		return err
	}

	progressStep(out, "Using runtime: %s\n", rt.Name())

	// Load state (optional - missing state is not an error for down)
	st, err := loadStateOptional(cwd)
	if err != nil {
		return err
	}

	if st == nil {
		progressStep(out, "No state file found. Container may not exist.\n")
		return nil
	}

	// Stop container
	progressStep(out, "Stopping container...\n")
	ctx := context.Background()
	if err := rt.Down(ctx, cwd, st); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Clean up LAN access if configured (macOS + OrbStack only)
	// See AGD-023 for design decisions.
	if goruntime.GOOS == "darwin" && network.HasLANAccess(cfg.Network.LANAccess) {
		if err := cleanupLANAccess(cwd, out); err != nil {
			// Don't fail on cleanup errors, just warn
			progressStep(out, "Warning: failed to clean up LAN access: %v\n", err)
		}
	}

	progressDone(out, "Container stopped\n")
	return nil
}

// cleanupLANAccess removes LAN access configuration for the project.
// See AGD-023 for implementation details.
func cleanupLANAccess(projectDir string, out io.Writer) error {
	// Only clean up for OrbStack
	isOrbStack, err := runtime.IsOrbStack()
	if err != nil {
		return fmt.Errorf("failed to detect runtime: %w", err)
	}
	if !isOrbStack {
		return nil
	}

	// Delete project file and check if shared should be removed
	removeShared, flushWarning, err := network.DeleteProjectFile(projectDir)
	if flushWarning != nil {
		progressStep(out, "Warning: failed to flush anchor: %v\n", flushWarning)
	}
	if err != nil {
		return err
	}

	if removeShared {
		progressStep(out, "No other LAN access projects, removing shared NAT rule\n")
		flushWarning, err := network.DeleteSharedRule()
		if flushWarning != nil {
			progressStep(out, "Warning: failed to flush anchor: %v\n", flushWarning)
		}
		if err != nil {
			return err
		}
	}

	progressStep(out, "LAN access cleaned up\n")
	return nil
}
