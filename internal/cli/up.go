package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
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

	// Early check - fail fast if network-helper needed but user declines
	// Callback creates NAT rules while sudo is cached after install
	if err := ensureNetworkHelper(&cfg, out, func() error {
		subnet, err := network.GetOrbStackSubnet()
		if err != nil {
			return fmt.Errorf("failed to get OrbStack subnet: %w", err)
		}
		interfaces, err := network.GetPhysicalInterfaces()
		if err != nil {
			return fmt.Errorf("failed to get physical interfaces: %w", err)
		}
		rules := network.GenerateNATRules(subnet, interfaces)
		if err := WriteSharedRuleWithSudo(rules); err != nil {
			return fmt.Errorf("failed to create NAT rules: %w", err)
		}
		progress(out, "→ NAT rules created for all interfaces\n")
		return nil
	}); err != nil {
		return err
	}

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

	// Configure NAT rules if LAN access enabled (macOS only)
	// Network-helper was already ensured early in the function.
	// See AGD-023 for design decisions.
	if goruntime.GOOS == "darwin" && network.HasLANAccess(cfg.Network.LANAccess) {
		if err := configureNATRules(cwd, out); err != nil {
			return fmt.Errorf("failed to configure LAN access: %w", err)
		}
	}

	progress(out, "✓ Environment ready\n")
	return nil
}

// ensureNetworkHelper checks if network-helper is needed and installed.
// Call EARLY in runUp(), before drift check.
// Returns error if user declines install.
// The onInstall callback is called after successful install while sudo is still cached.
// See AGD-023 for design decisions.
func ensureNetworkHelper(cfg *config.Config, out io.Writer, onInstall func() error) error {
	// Only applies to macOS with LAN access configured
	if goruntime.GOOS != "darwin" || !network.HasLANAccess(cfg.Network.LANAccess) {
		return nil
	}

	// Check runtime
	isOrbStack, err := runtime.IsOrbStack()
	if err != nil {
		return fmt.Errorf("failed to detect runtime: %w", err)
	}
	if !isOrbStack {
		progress(out, "→ LAN access: Docker Desktop detected, works natively\n")
		return nil
	}

	progress(out, "→ LAN access: OrbStack detected\n")

	// Check if network helper is installed and up to date
	installed := network.IsNetworkHelperInstalled()
	needsUpdate := IsNetworkHelperNeedsUpdate()

	if !installed {
		progress(out, "→ Network configuration requires network-helper.\n")
		if !promptConfirm("Install now?") {
			return fmt.Errorf("LAN access requires network-helper. Either:\n  - Run 'alca network-helper install' manually\n  - Remove network.lan-access from your config")
		}
	} else if needsUpdate {
		progress(out, "→ Network helper needs update.\n")
	} else {
		return nil // Already installed and up to date
	}

	// Install or update network helper
	if err := InstallNetworkHelper(); err != nil {
		return fmt.Errorf("failed to install network-helper: %w", err)
	}

	// Call onInstall callback while sudo is still cached
	if onInstall != nil {
		if err := onInstall(); err != nil {
			return err
		}
	}
	return nil
}

// configureNATRules sets up the NAT rules for LAN access.
// Call AFTER container start. Assumes network-helper is already installed.
// See AGD-023 for implementation details.
func configureNATRules(projectDir string, out io.Writer) error {
	// Check runtime - skip for Docker Desktop
	isOrbStack, err := runtime.IsOrbStack()
	if err != nil {
		return fmt.Errorf("failed to detect runtime: %w", err)
	}
	if !isOrbStack {
		return nil // Docker Desktop works natively, no NAT rules needed
	}

	// Get OrbStack subnet
	subnet, err := network.GetOrbStackSubnet()
	if err != nil {
		return fmt.Errorf("failed to get OrbStack subnet: %w", err)
	}

	progress(out, "→ OrbStack subnet: %s\n", subnet)

	// Check if rule update is needed (new interfaces detected)
	needsUpdate, newInterfaces, err := network.NeedsRuleUpdate()
	if err != nil {
		return fmt.Errorf("failed to check rule update: %w", err)
	}

	if needsUpdate {
		if len(newInterfaces) > 0 {
			progress(out, "→ New network interfaces detected: %s\n", strings.Join(newInterfaces, ", "))
		}
		// Create/update shared NAT rule for all physical interfaces
		interfaces, err := network.GetPhysicalInterfaces()
		if err != nil {
			return fmt.Errorf("failed to get physical interfaces: %w", err)
		}
		rules := network.GenerateNATRules(subnet, interfaces)
		if err := WriteSharedRuleWithSudo(rules); err != nil {
			return fmt.Errorf("failed to create shared NAT rule: %w", err)
		}
		progress(out, "→ NAT rules updated for all interfaces\n")
	}

	// Create project-specific file
	projectContent := "# Project-specific rules for " + projectDir + "\n"
	if err := WriteProjectFileWithSudo(projectDir, projectContent); err != nil {
		return fmt.Errorf("failed to create project file: %w", err)
	}

	progress(out, "→ LAN access configured\n")
	return nil
}
