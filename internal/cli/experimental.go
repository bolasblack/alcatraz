package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/spf13/cobra"
)

// experimentalWarning is displayed before executing experimental commands.
const experimentalWarning = `⚠️  EXPERIMENTAL COMMAND
This feature is experimental and may change or be removed in future versions.
Use with caution in production environments.
`

var experimentalCmd = &cobra.Command{
	Use:   "experimental",
	Short: "Experimental commands (use with caution)",
	Long:  `Experimental commands that may change or be removed in future versions.`,
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload sandbox configuration",
	Long: `Reload the sandbox configuration without rebuilding from scratch.

This command re-applies mounts and configuration by recreating the container
with the updated settings. Running processes inside the container will be
terminated.

This is an experimental feature and its behavior may change in future versions.`,
	RunE: runReload,
}

func init() {
	experimentalCmd.AddCommand(reloadCmd)
}

// runReload re-applies the configuration to the running container.
func runReload(cmd *cobra.Command, args []string) error {
	// Show experimental warning
	fmt.Fprint(cmd.OutOrStderr(), experimentalWarning)
	fmt.Fprintln(cmd.OutOrStderr())

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(cwd, ConfigFilename)

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration not found: run 'alca init' first")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select runtime based on config
	rt, err := runtime.SelectRuntime(&cfg)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}

	fmt.Printf("Using runtime: %s\n", rt.Name())

	// Load state
	st, err := state.Load(cwd)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if st == nil {
		return fmt.Errorf("no state file found: run 'alca up' first")
	}

	// Check current status
	ctx := context.Background()
	status, err := rt.Status(ctx, cwd, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == runtime.StateNotFound {
		return fmt.Errorf("container not found: run 'alca up' first to create the container")
	}

	fmt.Println("Reloading configuration...")

	// Reload the container
	if err := rt.Reload(ctx, &cfg, cwd, st); err != nil {
		if err == runtime.ErrNotRunning {
			return fmt.Errorf("container is not running: run 'alca up' first")
		}
		return fmt.Errorf("failed to reload container: %w", err)
	}

	// Update state with current config
	st.UpdateConfig(state.NewConfigSnapshot(cfg.Image, cfg.Workdir, rt.Name(), cfg.Mounts, cfg.Commands.Up, cfg.Commands.Enter))
	if err := state.Save(cwd, st); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Println("Configuration reloaded successfully.")
	return nil
}
