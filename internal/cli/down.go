package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
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

	// Create shared dependencies once
	tfs := transact.New()
	cmdRunner := util.NewCommandRunner()

	env := &util.Env{Fs: tfs, Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

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

	// Cleanup firewall rules before stopping container (need container ID)
	// See AGD-027 for design decisions
	cleanupFirewall(runtimeEnv, cmdRunner, rt, st, cwd, out)

	// Stop container
	util.ProgressStep(out, "Stopping container...\n")
	if err := rt.Down(runtimeEnv, cwd, st); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Network cleanup
	nh := network.NewNetworkHelper(cfg.Network, rt.Name())
	if nh != nil {
		isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
		networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, cwd, isOrbStack)
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

// cleanupFirewall removes firewall rules for the container.
// See AGD-027 for design decisions.
func cleanupFirewall(runtimeEnv *runtime.RuntimeEnv, cmdRunner util.CommandRunner, rt runtime.Runtime, st *state.State, projectDir string, out io.Writer) {
	// Get firewall implementation using shared command runner
	isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
	networkEnv := network.NewNetworkEnv(afero.NewOsFs(), cmdRunner, projectDir, isOrbStack)
	fw, fwType := network.New(networkEnv)
	if fwType == network.TypeNone || fw == nil {
		return
	}

	// Get container status to find the container ID
	status, err := rt.Status(runtimeEnv, "", st)
	if err != nil || status.State == runtime.StateNotFound {
		return
	}

	// Cleanup firewall rules
	if err := fw.Cleanup(status.ID); err != nil {
		util.ProgressStep(out, "Warning: failed to cleanup firewall rules: %v\n", err)
	}
}
