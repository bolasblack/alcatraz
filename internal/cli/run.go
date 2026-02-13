package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/sync"
	"github.com/bolasblack/alcatraz/internal/util"
)

var runCmd = &cobra.Command{
	Use:   "run [command]",
	Short: "Run a command inside the sandbox",
	Long:  `Execute a command inside the Alcatraz sandbox environment.`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runRun,
}

func init() {
	// Stop flag parsing after the first positional argument
	// This allows: alca run ls -la (without needing --)
	runCmd.Flags().SetInterspersed(false)
}

// runRun executes a command inside the container.
// See AGD-009 for CLI workflow design.
func runRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create shared dependencies once
	cmdRunner := util.NewCommandRunner()
	env := &util.Env{Fs: afero.NewReadOnlyFs(afero.NewOsFs()), Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

	// Load configuration and runtime
	cfg, rt, err := loadConfigAndRuntime(ctx, env, runtimeEnv, cwd)
	if err != nil {
		return err
	}

	// Load state (required)
	st, err := loadRequiredState(env, cwd)
	if err != nil {
		return err
	}

	// Check if project directory has moved since container was created
	if err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, cwd, cfg); err != nil {
		return err
	}

	// Check if container is running
	status, err := rt.Status(ctx, runtimeEnv, cwd, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State != runtime.StateRunning {
		return errors.New(ErrMsgNotRunning)
	}

	// SWR: show stale cache banner immediately, refresh periodically in background.
	syncFs := afero.NewOsFs()
	syncEnv := sync.NewSyncEnv(syncFs, cmdRunner, runtime.NewMutagenSyncClient(runtimeEnv))
	if cache, err := sync.ReadCache(syncFs, cwd); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to read sync conflict cache: %v\n", err)
	} else if cache != nil && len(cache.Conflicts) > 0 {
		sync.RenderBanner(cache.Conflicts, os.Stderr)
	}
	stopRefresh := sync.StartPeriodicRefresh(ctx, syncEnv, st.ProjectID, cwd)

	// Build command with optional enter prefix
	// If commands.enter is set, use it as command wrapper/prefix
	var execCmd []string
	if cfg.Commands.Enter.Command != "" {
		// Enter may contain shell syntax (&&, |, etc.), so wrap with sh -c
		// Quote each arg to preserve spaces and special characters
		quotedArgs := make([]string, len(args))
		for i, arg := range args {
			quotedArgs[i] = shellQuote(arg)
		}
		fullCmd := cfg.Commands.Enter.Command + " " + strings.Join(quotedArgs, " ")
		execCmd = []string{"sh", "-c", fullCmd}
	} else {
		// Run command directly
		execCmd = args
	}

	err = rt.Exec(ctx, runtimeEnv, cfg, cwd, st, execCmd)

	// Show exit banner if conflicts exist
	if conflicts := stopRefresh(); len(conflicts) > 0 {
		sync.RenderBanner(conflicts, os.Stderr)
	}

	if err != nil {
		// Pass through exit codes instead of reporting as error
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}

// shellQuote quotes a string for safe use in shell commands.
// It wraps the string in single quotes and escapes internal single quotes.
func shellQuote(s string) string {
	// Replace ' with '\'' (end quote, escaped quote, start quote)
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
