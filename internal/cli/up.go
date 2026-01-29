package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	goruntime "runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the sandbox environment",
	Long:  `Start the Alcatraz sandbox environment based on the current configuration.`,
	RunE:  runUp,
}

func init() {
	upCmd.Flags().BoolP("quiet", "q", false, "Suppress progress output")
	upCmd.Flags().BoolP("force", "f", false, "Force rebuild without confirmation on config change")
}

// runUp starts the container environment.
// See AGD-009 for CLI workflow design.
func runUp(cmd *cobra.Command, args []string) error {
	quiet, _ := cmd.Flags().GetBool("quiet")
	force, _ := cmd.Flags().GetBool("force")

	var out io.Writer = os.Stdout
	if quiet {
		out = nil
	}

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create TransactFs for file operations
	tfs := transact.New()
	env := util.NewEnv(tfs)

	// Load configuration
	progressStep(out, "Loading config from %s\n", ConfigFilename)
	cfg, _, err := loadConfigFromCwd(env, cwd)
	if err != nil {
		return err
	}

	// Select runtime based on config
	progressStep(out, "Detecting runtime...\n")
	rt, err := runtime.SelectRuntimeWithOutput(cfg, out)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}
	progressStep(out, "Detected runtime: %s\n", rt.Name())

	// Early check - fail fast if network-helper needed but user declines
	if err := ensureNetworkHelper(cfg, out); err != nil {
		return err
	}

	// Load or create state
	st, isNew, err := state.LoadOrCreate(env, cwd, rt.Name())
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if isNew {
		progressStep(out, "Created new state file: %s\n", state.StateFilePath(cwd))
	}

	ctx := context.Background()

	// Check for configuration drift and handle rebuild
	needsRebuild, err := handleConfigDrift(ctx, cfg, st, rt, out, force)
	if err != nil {
		return err
	}

	// If rebuild needed, remove existing container first
	if needsRebuild {
		if err := rebuildContainerIfNeeded(ctx, cfg, st, rt, cwd, out); err != nil {
			return err
		}
	}

	// Update state with current config only when rebuilding or first time
	if needsRebuild || isNew {
		st.UpdateConfig(cfg)
		if err := state.Save(env, cwd, st); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
		// Commit state changes (state file is in project dir, no sudo needed)
		if err := commitWithSudo(tfs); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Start container
	if err := rt.Up(ctx, cfg, cwd, st, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Configure NAT rules if LAN access enabled (macOS only)
	// See AGD-023 for design decisions.
	if goruntime.GOOS == "darwin" && network.HasLANAccess(cfg.Network.LANAccess) {
		if err := configureNATRules(cwd, out); err != nil {
			return fmt.Errorf("failed to configure LAN access: %w", err)
		}
	}

	progressDone(out, "Environment ready\n")
	return nil
}

// handleConfigDrift checks for configuration drift and prompts user if needed.
// Returns true if rebuild is needed.
func handleConfigDrift(ctx context.Context, cfg *config.Config, st *state.State, rt runtime.Runtime, out io.Writer, force bool) (bool, error) {
	runtimeChanged := st.Runtime != rt.Name()
	drift := st.DetectConfigDrift(cfg)

	if drift == nil && !runtimeChanged {
		return false, nil
	}

	if force {
		progressStep(out, "Configuration changed, rebuilding container (-f)\n")
		return true, nil
	}

	// Show drift and ask for confirmation
	displayConfigDrift(os.Stdout, drift, runtimeChanged, st.Runtime, rt.Name())
	fmt.Print("Rebuild container with new configuration? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("Keeping existing container.")
		return false, nil
	}

	return true, nil
}

// rebuildContainerIfNeeded removes the existing container for rebuild.
func rebuildContainerIfNeeded(ctx context.Context, cfg *config.Config, st *state.State, rt runtime.Runtime, cwd string, out io.Writer) error {
	// Determine which runtime to use for cleanup
	cleanupRt := rt
	runtimeChanged := st.Runtime != rt.Name()

	if runtimeChanged {
		// Runtime changed - use old runtime to remove old container
		if oldRt := runtime.ByName(st.Runtime); oldRt != nil {
			cleanupRt = oldRt
			progressStep(out, "Runtime changed: %s → %s\n", st.Runtime, rt.Name())
		}
	}

	status, _ := cleanupRt.Status(ctx, cwd, st)
	if status.State != runtime.StateNotFound {
		progressStep(out, "Removing existing container for rebuild...\n")
		if err := cleanupRt.Down(ctx, cwd, st); err != nil {
			return fmt.Errorf("failed to remove container for rebuild: %w", err)
		}
	}

	return nil
}

// ensureNetworkHelper checks if network-helper is needed and installed.
// Call EARLY in runUp(), before drift check.
// Returns error if user declines install.
// See AGD-023 for design decisions.
func ensureNetworkHelper(cfg *config.Config, out io.Writer) error {
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
		progressStep(out, "LAN access: Docker Desktop detected, works natively\n")
		return nil
	}

	progressStep(out, "LAN access: OrbStack detected\n")

	// Create TransactFs for all file operations
	tfs := transact.New()
	env := util.NewEnv(tfs)

	// Check if network helper is installed and up to date
	installed := network.IsHelperInstalled(env)
	needsUpdate := network.IsHelperNeedsUpdate(env)

	if !installed {
		progressStep(out, "Network configuration requires network-helper.\n")
		if !promptConfirm("Install now?") {
			return fmt.Errorf("LAN access requires network-helper. Either:\n  - Run 'alca network-helper install' manually\n  - Remove network.lan-access from your config")
		}
	} else if needsUpdate {
		progressStep(out, "Network helper needs update.\n")
	} else {
		return nil // Already installed and up to date
	}

	// Install or update network helper (stages files)
	if err := network.InstallHelper(env, func(format string, args ...any) {
		progressStep(out, format, args...)
	}); err != nil {
		return fmt.Errorf("failed to install network-helper: %w", err)
	}

	subnet, err := network.GetOrbStackSubnet()
	if err != nil {
		return fmt.Errorf("failed to get OrbStack subnet: %w", err)
	}
	interfaces, err := network.GetPhysicalInterfaces()
	if err != nil {
		return fmt.Errorf("failed to get physical interfaces: %w", err)
	}
	rules := network.GenerateNATRules(subnet, interfaces)
	if err := network.WriteSharedRule(env, rules); err != nil {
		return fmt.Errorf("failed to create NAT rules: %w", err)
	}

	// Commit staged file operations with sudo
	if tfs.NeedsCommit() {
		if err := commitWithSudo(tfs); err != nil {
			return fmt.Errorf("failed to commit NAT rules: %w", err)
		}
	}
	progressStep(out, "NAT rules created for all interfaces\n")

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

	// Create TransactFs for batched file operations
	tfs := transact.New()
	env := util.NewEnv(tfs)

	// Get OrbStack subnet
	subnet, err := network.GetOrbStackSubnet()
	if err != nil {
		return fmt.Errorf("failed to get OrbStack subnet: %w", err)
	}

	progressStep(out, "OrbStack subnet: %s\n", subnet)

	// Check if rule update is needed (new interfaces detected)
	needsUpdate, newInterfaces, err := network.NeedsRuleUpdate(env)
	if err != nil {
		return fmt.Errorf("failed to check rule update: %w", err)
	}

	if needsUpdate {
		if len(newInterfaces) > 0 {
			progressStep(out, "New network interfaces detected: %s\n", strings.Join(newInterfaces, ", "))
		}
		// Create/update shared NAT rule for all physical interfaces
		interfaces, err := network.GetPhysicalInterfaces()
		if err != nil {
			return fmt.Errorf("failed to get physical interfaces: %w", err)
		}
		rules := network.GenerateNATRules(subnet, interfaces)
		if err := network.WriteSharedRule(env, rules); err != nil {
			return fmt.Errorf("failed to create shared NAT rule: %w", err)
		}
		progressStep(out, "NAT rules updated for all interfaces\n")
	}

	// Create project-specific file
	projectContent := "# Project-specific rules for " + projectDir + "\n"
	if err := network.WriteProjectFile(env, projectDir, projectContent); err != nil {
		return fmt.Errorf("failed to create project file: %w", err)
	}

	// Commit all staged file operations with sudo
	if tfs.NeedsCommit() {
		if err := commitWithSudo(tfs); err != nil {
			return fmt.Errorf("failed to commit NAT rules: %w", err)
		}
	}

	progressStep(out, "LAN access configured\n")
	return nil
}
