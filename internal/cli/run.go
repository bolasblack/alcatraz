package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
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

	ctx := context.Background()

	// Check if container is running
	status, err := rt.Status(ctx, cwd)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	if status.State != runtime.StateRunning {
		return fmt.Errorf("container is not running: run 'alca up' first")
	}

	// Build command with flake.nix detection
	// If flake.nix exists, wrap command with nix develop --command
	userCmd := shellQuoteArgs(args)
	wrappedCmd := fmt.Sprintf(
		"if [ -f flake.nix ]; then nix --extra-experimental-features 'nix-command flakes' develop --command sh -c %s; else sh -c %s; fi",
		userCmd, userCmd)

	execCmd := []string{"sh", "-c", wrappedCmd}
	if err := rt.Exec(ctx, cwd, execCmd); err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}

// shellQuoteArgs quotes arguments for safe shell execution.
func shellQuoteArgs(args []string) string {
	if len(args) == 0 {
		return "''"
	}
	// Join args and wrap in single quotes, escaping existing quotes
	var quoted []string
	for _, arg := range args {
		// Escape single quotes by ending quote, adding escaped quote, starting new quote
		escaped := strings.ReplaceAll(arg, "'", "'\"'\"'")
		quoted = append(quoted, escaped)
	}
	return "'" + strings.Join(quoted, " ") + "'"
}
