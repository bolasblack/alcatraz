package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/spf13/cobra"
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(cwd, ConfigFilename)

	// Load configuration to get enter command
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

	// Load state
	st, err := state.Load(cwd)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if st == nil {
		return fmt.Errorf("no state file found: run 'alca up' first")
	}

	ctx := context.Background()

	// Check if container is running
	status, err := rt.Status(ctx, cwd, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State != runtime.StateRunning {
		return fmt.Errorf("container is not running: run 'alca up' first")
	}

	// Build command with optional enter prefix
	// If commands.enter is set, use it as command wrapper/prefix
	var execCmd []string
	if cfg.Commands.Enter != "" {
		// Enter may contain shell syntax (&&, |, etc.), so wrap with sh -c
		// Quote each arg to preserve spaces and special characters
		quotedArgs := make([]string, len(args))
		for i, arg := range args {
			quotedArgs[i] = shellQuote(arg)
		}
		fullCmd := cfg.Commands.Enter + " " + strings.Join(quotedArgs, " ")
		execCmd = []string{"sh", "-c", fullCmd}
	} else {
		// Run command directly
		execCmd = args
	}

	err = rt.Exec(ctx, &cfg, cwd, st, execCmd)
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


