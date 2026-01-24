package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/spf13/cobra"
)

var (
	upQuiet bool
	upForce bool
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the sandbox environment",
	Long:  `Start the Alcatraz sandbox environment based on the current configuration.`,
	RunE:  runUp,
}

func init() {
	upCmd.Flags().BoolVarP(&upQuiet, "quiet", "q", false, "Suppress progress output")
	upCmd.Flags().BoolVarP(&upForce, "force", "f", false, "Force rebuild without confirmation on config change")
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

	// Load or create state
	st, isNew, err := state.LoadOrCreate(cwd, rt.Name())
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if isNew {
		progress(out, "→ Created new state file: %s\n", state.StateFilePath(cwd))
	}

	ctx := context.Background()

	// Check for configuration drift
	needsRebuild := false
	runtimeChanged := st.Runtime != rt.Name()
	drift := st.DetectConfigDrift(&cfg)
	if drift != nil || runtimeChanged {
		if !upForce {
			// Show drift and ask for confirmation
			fmt.Println("Configuration has changed since last container creation:")
			if runtimeChanged {
				fmt.Printf("  Runtime: %s → %s\n", st.Runtime, rt.Name())
			}
			if drift != nil {
				if drift.Image != nil {
					fmt.Printf("  Image: %s → %s\n", drift.Image[0], drift.Image[1])
				}
				if drift.Mounts {
					fmt.Printf("  Mounts: changed\n")
				}
				if drift.Workdir != nil {
					fmt.Printf("  Workdir: %s → %s\n", drift.Workdir[0], drift.Workdir[1])
				}
				if drift.CommandUp != nil {
					fmt.Printf("  Commands.up: changed\n")
				}
				if drift.Memory != nil {
					fmt.Printf("  Resources.memory: %s → %s\n", drift.Memory[0], drift.Memory[1])
				}
				if drift.CPUs != nil {
					fmt.Printf("  Resources.cpus: %d → %d\n", drift.CPUs[0], drift.CPUs[1])
				}
				if drift.Envs {
					fmt.Printf("  Envs: changed\n")
				}
			}
			fmt.Print("Rebuild container with new configuration? [y/N] ")

			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))

			if answer != "y" && answer != "yes" {
				fmt.Println("Keeping existing container.")
			} else {
				needsRebuild = true
			}
		} else {
			needsRebuild = true
			progress(out, "→ Configuration changed, rebuilding container (-f)\n")
		}
	}

	// If rebuild needed, remove existing container first
	if needsRebuild {
		// Determine which runtime to use for cleanup
		cleanupRt := rt
		if runtimeChanged {
			// Runtime changed - use old runtime to remove old container
			if oldRt := runtime.ByName(st.Runtime); oldRt != nil {
				cleanupRt = oldRt
				progress(out, "→ Runtime changed: %s → %s\n", st.Runtime, rt.Name())
			}
		}

		status, _ := cleanupRt.Status(ctx, cwd, st)
		if status.State != runtime.StateNotFound {
			progress(out, "→ Removing existing container for rebuild...\n")
			if err := cleanupRt.Down(ctx, cwd, st); err != nil {
				return fmt.Errorf("failed to remove container for rebuild: %w", err)
			}
		}
	}

	// Update state with current config only when rebuilding or first time
	if needsRebuild || isNew {
		st.UpdateConfig(&cfg)
		if err := state.Save(cwd, st); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Start container
	if err := rt.Up(ctx, &cfg, cwd, st, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	progress(out, "✓ Environment ready\n")
	return nil
}
