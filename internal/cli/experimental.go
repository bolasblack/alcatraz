package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

// experimentalWarning is displayed before executing experimental commands.
const experimentalWarning = `Warning: EXPERIMENTAL COMMAND
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

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync conflict management",
}

func init() {
	experimentalCmd.AddCommand(reloadCmd)
	experimentalCmd.AddCommand(syncCmd)
	syncCmd.AddCommand(syncCheckCmd)
	syncCmd.AddCommand(syncResolveCmd)
}

// runReload re-applies the configuration to the running container.
func runReload(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// Show experimental warning
	_, _ = fmt.Fprint(cmd.OutOrStderr(), experimentalWarning)
	_, _ = fmt.Fprintln(cmd.OutOrStderr())

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create shared dependencies once
	deps := newCLIDeps()
	tfs, env, runtimeEnv := deps.Tfs, deps.Env, deps.RuntimeEnv

	// Load configuration and runtime
	cfg, rt, err := loadConfigAndRuntime(ctx, env, runtimeEnv, cwd)
	if err != nil {
		return err
	}

	util.ProgressStep(os.Stdout, "Using runtime: %s\n", rt.Name())

	// Load state (required)
	st, err := loadRequiredState(env, cwd)
	if err != nil {
		return err
	}

	// Check current status
	status, err := rt.Status(ctx, runtimeEnv, cwd, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State == runtime.StateNotFound {
		return fmt.Errorf("container not found: run 'alca up' first to create the container")
	}

	util.ProgressStep(os.Stdout, "Reloading configuration...\n")

	// Reload the container
	if err := rt.Reload(ctx, runtimeEnv, cfg, cwd, st); err != nil {
		if errors.Is(err, runtime.ErrNotRunning) {
			return errors.New(ErrMsgNotRunning)
		}
		return fmt.Errorf("failed to reload container: %w", err)
	}

	// Update state with current config
	st.UpdateConfig(cfg)
	if err := state.Save(env, cwd, st); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	// Commit file operations (project dir, normally no sudo needed)
	if err := commitWithSudo(ctx, env, tfs, os.Stdout, ""); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	util.ProgressDone(os.Stdout, "Configuration reloaded successfully.\n")
	return nil
}
