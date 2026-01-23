package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/spf13/cobra"
)

var upQuiet bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the sandbox environment",
	Long:  `Start the Alcatraz sandbox environment based on the current configuration.`,
	RunE:  runUp,
}

func init() {
	upCmd.Flags().BoolVarP(&upQuiet, "quiet", "q", false, "Suppress progress output")
}

// progress writes a progress message if not in quiet mode.
func progress(w io.Writer, format string, args ...any) {
	if w != nil {
		fmt.Fprintf(w, format, args...)
	}
}

// runUp starts the container environment.
// See AGD-009 for CLI workflow design.
func runUp(cmd *cobra.Command, args []string) error {
	var out io.Writer = os.Stdout
	if upQuiet {
		out = nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(cwd, ConfigFilename)

	// Load configuration
	progress(out, "→ Loading config from %s\n", ConfigFilename)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration not found: run 'alca init' first")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select runtime based on config
	progress(out, "→ Detecting runtime...\n")
	rt, err := runtime.SelectRuntimeWithOutput(&cfg, out)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}
	progress(out, "→ Detected runtime: %s\n", rt.Name())

	// Start container
	ctx := context.Background()
	if err := rt.Up(ctx, &cfg, cwd, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	progress(out, "✓ Environment ready\n")
	return nil
}
