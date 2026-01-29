package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
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
	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create env for read-only file operations
	env := util.NewReadonlyOsEnv()

	configPath := filepath.Join(cwd, ConfigFilename)

	// Check if config exists
	if _, err := env.Fs.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("Status: Not initialized")
		fmt.Println("")
		fmt.Println("Run 'alca init' to create a configuration file.")
		return nil
	}

	fmt.Println("Status: Initialized")
	fmt.Printf("Config: %s\n", configPath)
	fmt.Println("")

	// Load config
	cfg, err := config.LoadConfig(env, configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select runtime
	rt, err := runtime.SelectRuntime(&cfg)
	if err != nil {
		fmt.Println("Runtime: None available")
		fmt.Println("")
		fmt.Printf("Error: %v\n", err)
		return nil
	}

	fmt.Printf("Runtime: %s\n", rt.Name())
	fmt.Println("")

	// Load state (optional for status)
	st, err := state.Load(env, cwd)
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

	printContainerStatus(status, st, &cfg, rt)

	return nil
}

// printContainerStatus prints container status with drift detection.
func printContainerStatus(status runtime.ContainerStatus, st *state.State, cfg *config.Config, rt runtime.Runtime) {
	switch status.State {
	case runtime.StateRunning:
		printRunningContainerStatus(status, st, cfg, rt)
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
}

// printRunningContainerStatus prints status for a running container.
func printRunningContainerStatus(status runtime.ContainerStatus, st *state.State, cfg *config.Config, rt runtime.Runtime) {
	fmt.Println("Container: Running")
	fmt.Printf("  ID:    %s\n", status.ID)
	fmt.Printf("  Name:  %s\n", status.Name)
	fmt.Printf("  Image: %s\n", status.Image)
	if status.StartedAt != "" {
		fmt.Printf("  Started: %s\n", status.StartedAt)
	}
	fmt.Println("")

	// Check for configuration drift
	runtimeChanged := st.Runtime != rt.Name()
	drift := st.DetectConfigDrift(cfg)
	if displayConfigDrift(os.Stdout, drift, runtimeChanged, st.Runtime, rt.Name()) {
		fmt.Println("")
		fmt.Println("Run 'alca up -f' to rebuild with new configuration.")
		fmt.Println("")
	}

	fmt.Println("Run 'alca run <command>' to execute commands.")
}
