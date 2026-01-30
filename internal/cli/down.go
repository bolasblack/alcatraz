package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
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

	// Create TransactFs for file operations
	tfs := transact.New()
	env := util.NewEnv(tfs)

	// Create runtime environment once for all runtime operations
	runtimeEnv := runtime.NewRuntimeEnv()

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

	// Stop container
	util.ProgressStep(out, "Stopping container...\n")
	if err := rt.Down(runtimeEnv, cwd, st); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Network cleanup
	nh := network.New(cfg.Network, rt.Name())
	if nh != nil {
		if err := nh.Teardown(env, cwd); err != nil {
			util.ProgressStep(out, "Warning: failed to cleanup network: %v\n", err)
		}

		if err := commitIfNeeded(env, tfs, out, "Removing firewall rules"); err != nil {
			util.ProgressStep(out, "Warning: failed to commit: %v\n", err)
		}
	}

	util.ProgressDone(out, "Container stopped\n")
	return nil
}
