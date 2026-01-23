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
	if st.Config != nil {
		drift := st.DetectConfigDrift(state.NewConfigSnapshot(cfg.Image, cfg.Workdir, rt.Name(), cfg.Mounts, cfg.Commands.Up, cfg.Commands.Enter))
		if drift != nil && drift.HasDrift() {
			if !upForce {
				// Show drift and ask for confirmation
				fmt.Println("Configuration has changed since last container creation:")
				if drift.Old.Image != drift.New.Image {
					fmt.Printf("  Image: %s → %s\n", drift.Old.Image, drift.New.Image)
				}
				if !slicesEqual(drift.Old.Mounts, drift.New.Mounts) {
					fmt.Printf("  Mounts: changed\n")
				}
				if drift.Old.Workdir != drift.New.Workdir {
					fmt.Printf("  Workdir: %s → %s\n", drift.Old.Workdir, drift.New.Workdir)
				}
				if drift.Old.Runtime != drift.New.Runtime {
					fmt.Printf("  Runtime: %s → %s\n", drift.Old.Runtime, drift.New.Runtime)
				}
				if drift.Old.CmdUp != drift.New.CmdUp {
					fmt.Printf("  Commands.up: changed\n")
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
	}

	// If rebuild needed, remove existing container first
	if needsRebuild {
		status, _ := rt.Status(ctx, cwd, st)
		if status.State != runtime.StateNotFound {
			progress(out, "→ Removing existing container for rebuild...\n")
			if err := rt.Down(ctx, cwd, st); err != nil {
				return fmt.Errorf("failed to remove container for rebuild: %w", err)
			}
		}
	}

	// Update state with current config
	st.UpdateConfig(state.NewConfigSnapshot(cfg.Image, cfg.Workdir, rt.Name(), cfg.Mounts, cfg.Commands.Up, cfg.Commands.Enter))
	if err := state.Save(cwd, st); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	// Start container
	if err := rt.Up(ctx, &cfg, cwd, st, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	progress(out, "✓ Environment ready\n")
	return nil
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
