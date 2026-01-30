package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the sandbox environment",
	Long:  `Start the Alcatraz sandbox environment based on the current configuration.`,
	RunE:  runUp,
}

func init() {
	upCmd.Flags().BoolP("quiet", "q", false, "Suppress progress output")
	upCmd.Flags().BoolP("force", "f", false, "Force rebuild without confirmation on config change")
}

// runUp starts the container environment.
// See AGD-009 for CLI workflow design.
func runUp(cmd *cobra.Command, args []string) error {
	quiet, _ := cmd.Flags().GetBool("quiet")
	force, _ := cmd.Flags().GetBool("force")

	var out io.Writer = os.Stdout
	if quiet {
		out = nil
	}

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create TransactFs for file operations
	tfs := transact.New()
	env := util.NewEnv(tfs)

	// Load configuration
	util.ProgressStep(out, "Loading config from %s\n", ConfigFilename)
	cfg, _, err := loadConfigFromCwd(env, cwd)
	if err != nil {
		return err
	}

	// Select runtime based on config
	util.ProgressStep(out, "Detecting runtime...\n")
	rt, err := runtime.SelectRuntimeWithOutput(cfg, out)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}
	util.ProgressStep(out, "Detected runtime: %s\n", rt.Name())

	// Network helper (handles all platform-specific logic)
	nh := network.New(cfg.Network, rt.Name())
	if nh != nil {
		if err := setupNetwork(nh, env, tfs, cwd, out); err != nil {
			return err
		}
		// Reset tfs/env for subsequent operations since network setup may have committed
		tfs = transact.New()
		env = util.NewEnv(tfs)
	}

	// Load or create state
	st, isNew, err := state.LoadOrCreate(env, cwd, rt.Name())
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if isNew {
		util.ProgressStep(out, "Created new state file: %s\n", state.StateFilePath(cwd))
	}

	ctx := context.Background()

	// Check for configuration drift and handle rebuild
	needsRebuild, err := handleConfigDrift(ctx, cfg, st, rt, out, force)
	if err != nil {
		return err
	}

	// If rebuild needed, remove existing container first
	if needsRebuild {
		if err := rebuildContainerIfNeeded(ctx, cfg, st, rt, cwd, out); err != nil {
			return err
		}
	}

	// Update state with current config only when rebuilding or first time
	if needsRebuild || isNew {
		st.UpdateConfig(cfg)
		if err := state.Save(env, cwd, st); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
		// Commit state changes (project dir, normally no sudo needed)
		if err := commitWithSudo(env, tfs, out, ""); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Start container
	if err := rt.Up(ctx, cfg, cwd, st, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	util.ProgressDone(out, "Environment ready\n")
	return nil
}

// handleConfigDrift checks for configuration drift and prompts user if needed.
// Returns true if rebuild is needed.
func handleConfigDrift(ctx context.Context, cfg *config.Config, st *state.State, rt runtime.Runtime, out io.Writer, force bool) (bool, error) {
	runtimeChanged := st.Runtime != rt.Name()
	drift := st.DetectConfigDrift(cfg)

	if drift == nil && !runtimeChanged {
		return false, nil
	}

	if force {
		util.ProgressStep(out, "Configuration changed, rebuilding container (-f)\n")
		return true, nil
	}

	// Show drift and ask for confirmation
	displayConfigDrift(out, drift, runtimeChanged, st.Runtime, rt.Name())
	fmt.Print("Rebuild container with new configuration? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("Keeping existing container.")
		return false, nil
	}

	return true, nil
}

// rebuildContainerIfNeeded removes the existing container for rebuild.
func rebuildContainerIfNeeded(ctx context.Context, cfg *config.Config, st *state.State, rt runtime.Runtime, cwd string, out io.Writer) error {
	// Determine which runtime to use for cleanup
	cleanupRt := rt
	runtimeChanged := st.Runtime != rt.Name()

	if runtimeChanged {
		// Runtime changed - use old runtime to remove old container
		if oldRt := runtime.ByName(st.Runtime); oldRt != nil {
			cleanupRt = oldRt
			util.ProgressStep(out, "Runtime changed: %s → %s\n", st.Runtime, rt.Name())
		}
	}

	status, _ := cleanupRt.Status(ctx, cwd, st)
	if status.State != runtime.StateNotFound {
		util.ProgressStep(out, "Removing existing container for rebuild...\n")
		if err := cleanupRt.Down(ctx, cwd, st); err != nil {
			return fmt.Errorf("failed to remove container for rebuild: %w", err)
		}
	}

	return nil
}

// setupNetwork configures network helper for LAN access.
// See AGD-023 for design decisions.
func setupNetwork(nh network.NetworkHelper, env *util.Env, tfs *transact.TransactFs, projectDir string, out io.Writer) error {
	progress := func(format string, args ...any) {
		util.ProgressStep(out, format, args...)
	}

	// Check and install helper if needed
	status := nh.HelperStatus(env)
	if !status.Installed {
		util.ProgressStep(out, "Network helper required for LAN access.\n")
		if !promptConfirm("Install now?") {
			return fmt.Errorf("LAN access requires network helper")
		}
	}

	if !status.Installed || status.NeedsUpdate {
		action, err := nh.InstallHelper(env, progress)
		if err != nil {
			return fmt.Errorf("failed to install network helper: %w", err)
		}

		if err := commitIfNeeded(env, tfs, out, "Writing network helper to system directories"); err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}

		if action.Run != nil {
			if err := action.Run(progress); err != nil {
				return fmt.Errorf("post-install failed: %w", err)
			}
		}
	}

	// Setup project rules
	action, err := nh.Setup(env, projectDir, progress)
	if err != nil {
		return fmt.Errorf("failed to setup network: %w", err)
	}

	if err := commitIfNeeded(env, tfs, out, "Writing firewall rules to system directories"); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if action.Run != nil {
		if err := action.Run(progress); err != nil {
			return fmt.Errorf("post-setup failed: %w", err)
		}
	}

	util.ProgressStep(out, "LAN access configured\n")
	return nil
}
