package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
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

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Load config to respect runtime setting
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, err := config.LoadConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select runtime based on config
	rt, err := runtime.SelectRuntime(&cfg)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}

	progress(out, "→ Using runtime: %s\n", rt.Name())

	// Load state
	st, err := state.Load(cwd)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if st == nil {
		progress(out, "→ No state file found. Container may not exist.\n")
		return nil
	}

	// Stop container
	progress(out, "→ Stopping container...\n")
	ctx := context.Background()
	if err := rt.Down(ctx, cwd, st); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Clean up LAN access if configured (macOS + OrbStack only)
	// See AGD-023 for design decisions.
	if goruntime.GOOS == "darwin" && network.HasLANAccess(cfg.Network.LANAccess) {
		if err := cleanupLANAccess(cwd, out); err != nil {
			// Don't fail on cleanup errors, just warn
			progress(out, "→ Warning: failed to clean up LAN access: %v\n", err)
		}
	}

	progress(out, "✓ Container stopped\n")
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
	removeShared, err := network.DeleteProjectFile(projectDir)
	if err != nil {
		return err
	}

	if removeShared {
		progress(out, "→ No other LAN access projects, removing shared NAT rule\n")
		if err := network.DeleteSharedRule(); err != nil {
			return err
		}
	}

	progress(out, "→ LAN access cleaned up\n")
	return nil
}
