package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/sync"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

func init() {
	downCmd.Flags().Bool("force", false, "Skip sync conflict check and proceed anyway")
}

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

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create shared dependencies once
	deps := newCLIDeps()
	tfs, env, runtimeEnv := deps.Tfs, deps.Env, deps.RuntimeEnv

	// Load config (optional) and select runtime
	cfg, rt, err := loadConfigAndRuntimeOptional(env, runtimeEnv, cwd)
	if err != nil {
		return err
	}

	util.ProgressStep(out, "Using runtime: %s\n", rt.Name())

	// Load state (optional - missing state is not an error for down)
	st, err := loadStateOptional(env, cwd)
	if err != nil {
		return err
	}

	if st == nil {
		util.ProgressStep(out, "No state file found. Container may not exist.\n")
		return nil
	}

	// Check for sync conflicts before destroying container (AGD-031).
	// Once the container is gone, conflicts become unresolvable.
	syncFs := afero.NewOsFs()
	syncEnv := sync.NewSyncEnv(syncFs, deps.CmdRunner, runtime.NewMutagenSyncClient(runtimeEnv))
	if err := guardSyncConflicts(syncFs, syncEnv, cwd, st.ProjectID, force, os.Stderr); err != nil {
		return err
	}

	platform := runtime.DetectPlatform(runtimeEnv)

	// Cleanup firewall rules before stopping container (need container ID)
	// See AGD-027 for design decisions
	// Files removed via tfs, committed to real disk before nft cleanup commands run.
	firewallEnv := network.NewNetworkEnv(tfs, deps.CmdRunner, cwd, platform)
	if err := cleanupFirewall(firewallEnv, env, tfs, runtimeEnv, rt, st, out); err != nil {
		util.ProgressStep(out, "Warning: firewall cleanup: %v\n", err)
	}

	// Stop container
	util.ProgressStep(out, "Stopping container...\n")
	if err := rt.Down(runtimeEnv, cwd, st); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Network cleanup
	nh := network.NewNetworkHelper(cfg.Network, platform)
	if nh != nil {
		networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, cwd, platform)
		if err := nh.Teardown(networkEnv, cwd); err != nil {
			util.ProgressStep(out, "Warning: failed to cleanup network: %v\n", err)
		}

		if err := commitIfNeeded(env, tfs, out, "Removing firewall rules"); err != nil {
			util.ProgressStep(out, "Warning: failed to commit: %v\n", err)
		}
	}

	util.ProgressDone(out, "Container stopped\n")
	return nil
}

// guardSyncConflicts checks for unresolved sync conflicts and blocks if found.
// When force is true, prints a warning and allows the operation to proceed.
// Best-effort: detection errors are logged to stderr to avoid blocking 'down'.
func guardSyncConflicts(fs afero.Fs, syncEnv *sync.SyncEnv, projectRoot string, projectID string, force bool, w io.Writer) error {
	if force {
		_, _ = fmt.Fprintln(w, "Warning: --force used, skipping sync conflict check. Conflicts may become unresolvable after container is destroyed.")
		return nil
	}

	conflicts := checkSyncConflictsBeforeDown(fs, syncEnv, projectRoot, projectID)
	if len(conflicts) > 0 {
		sync.RenderBanner(conflicts, w)
		return fmt.Errorf("resolve sync conflicts before stopping the container, or use --force to skip this check")
	}
	return nil
}

// checkSyncConflictsBeforeDown reads cached conflicts (fast path) or polls
// mutagen synchronously (slow path) when no cache exists.
// Best-effort: detection errors are logged to stderr to avoid blocking 'down'.
func checkSyncConflictsBeforeDown(fs afero.Fs, syncEnv *sync.SyncEnv, projectRoot string, projectID string) []sync.ConflictInfo {
	// Fast path: read from cache
	cache, err := sync.ReadCache(fs, projectRoot)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to read sync conflict cache: %v\n", err)
	}
	if cache != nil {
		return cache.Conflicts
	}

	// Slow path: cache missing, do synchronous poll
	cache, err = sync.SyncUpdateCache(context.Background(), syncEnv, projectID, projectRoot)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to check sync conflicts: %v\n", err)
	}
	if cache != nil {
		return cache.Conflicts
	}
	return nil
}

// cleanupFirewall removes firewall rules for the container.
// See AGD-027 for design decisions.
func cleanupFirewall(networkEnv *network.NetworkEnv, env *util.Env, tfs *transact.TransactFs, runtimeEnv *runtime.RuntimeEnv, rt runtime.Runtime, st *state.State, out io.Writer) error {
	fw, fwType := network.New(networkEnv)
	if fwType == network.TypeNone || fw == nil {
		return nil
	}

	// Get container status to find the container ID
	status, err := rt.Status(runtimeEnv, "", st)
	if err != nil || status.State == runtime.StateNotFound {
		return nil
	}

	// Cleanup firewall rules (removes files via tfs)
	action, err := fw.Cleanup(status.ID)
	if err != nil {
		return fmt.Errorf("cleanup firewall rules: %w", err)
	}

	// Commit tfs to remove files from real disk
	if err := commitIfNeeded(env, tfs, out, "Removing firewall rules"); err != nil {
		return fmt.Errorf("commit firewall cleanup: %w", err)
	}

	// Run post-commit action (nft delete table or reload)
	if action != nil && action.Run != nil {
		if err := action.Run(nil); err != nil {
			return fmt.Errorf("execute firewall cleanup: %w", err)
		}
	}

	return nil
}
