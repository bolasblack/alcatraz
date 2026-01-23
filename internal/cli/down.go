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

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the sandbox environment",
	Long:  `Stop the running Alcatraz sandbox environment.`,
	RunE:  runDown,
}

// runDown stops and removes the container.
// See AGD-009 for CLI workflow design.
func runDown(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Load config to respect runtime setting
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, err := config.LoadConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
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
		fmt.Println("No state file found. Container may not exist.")
		return nil
	}

	// Stop container
	ctx := context.Background()
	if err := rt.Down(ctx, cwd, st); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	fmt.Println("Container stopped successfully.")
	return nil
}
