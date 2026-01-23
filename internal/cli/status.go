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

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current Alcatraz status",
	Long:  `Display the current status of Alcatraz sandbox configuration and running processes.`,
	RunE:  runStatus,
}

// runStatus displays container status.
// See AGD-009 for CLI workflow design.
func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(cwd, ConfigFilename)

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("Status: Not initialized")
		fmt.Println("")
		fmt.Println("Run 'alca init' to create a configuration file.")
		return nil
	}

	fmt.Println("Status: Initialized")
	fmt.Printf("Config: %s\n", configPath)
	fmt.Println("")

	// Load config to respect runtime setting
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select runtime based on config
	rt, err := runtime.SelectRuntime(&cfg)
	if err != nil {
		fmt.Println("Runtime: None available")
		fmt.Println("")
		fmt.Printf("Error: %v\n", err)
		return nil
	}

	fmt.Printf("Runtime: %s\n", rt.Name())
	fmt.Println("")

	// Load state
	st, err := state.Load(cwd)
	if err != nil {
		fmt.Printf("State: Error loading state: %v\n", err)
		return nil
	}

	if st == nil {
		fmt.Println("State: Not created")
		fmt.Println("")
		fmt.Println("Run 'alca up' to create the container.")
		return nil
	}

	fmt.Printf("Project ID: %s\n", st.ProjectID)
	fmt.Println("")

	// Get container status
	ctx := context.Background()
	status, err := rt.Status(ctx, cwd, st)
	if err != nil {
		fmt.Println("Container: Error getting status")
		return nil
	}

	switch status.State {
	case runtime.StateRunning:
		fmt.Println("Container: Running")
		fmt.Printf("  ID:    %s\n", status.ID)
		fmt.Printf("  Name:  %s\n", status.Name)
		fmt.Printf("  Image: %s\n", status.Image)
		if status.StartedAt != "" {
			fmt.Printf("  Started: %s\n", status.StartedAt)
		}
		fmt.Println("")

		// Check for configuration drift
		if st.Config != nil {
			drift := st.DetectConfigDrift(state.NewConfigSnapshot(cfg.Image, cfg.Workdir, rt.Name(), cfg.Mounts, cfg.Commands.Up, cfg.Commands.Enter))
			if drift != nil && drift.HasDrift() {
				fmt.Println("⚠️  Configuration drift detected:")
				if drift.Old.Image != drift.New.Image {
					fmt.Printf("  Image: %s → %s\n", drift.Old.Image, drift.New.Image)
				}
				if drift.Old.Workdir != drift.New.Workdir {
					fmt.Printf("  Workdir: %s → %s\n", drift.Old.Workdir, drift.New.Workdir)
				}
				if drift.Old.Runtime != drift.New.Runtime {
					fmt.Printf("  Runtime: %s → %s\n", drift.Old.Runtime, drift.New.Runtime)
				}
				fmt.Println("")
				fmt.Println("Run 'alca up -f' to rebuild with new configuration.")
				fmt.Println("")
			}
		}

		fmt.Println("Run 'alca run <command>' to execute commands.")
	case runtime.StateStopped:
		fmt.Println("Container: Stopped")
		fmt.Println("")
		fmt.Println("Run 'alca up' to start the container.")
	case runtime.StateNotFound:
		fmt.Println("Container: Not created")
		fmt.Println("")
		fmt.Println("Run 'alca up' to create and start the container.")
	default:
		fmt.Println("Container: Unknown state")
	}

	return nil
}
