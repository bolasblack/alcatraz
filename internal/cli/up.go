package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"
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

	// Create shared dependencies once
	tfs := transact.New()
	cmdRunner := util.NewCommandRunner()

	env := &util.Env{Fs: tfs, Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

	// Load configuration
	util.ProgressStep(out, "Loading config from %s\n", ConfigFilename)
	cfg, _, err := loadConfigFromCwd(env, cwd)
	if err != nil {
		return err
	}

	// Select runtime based on config
	util.ProgressStep(out, "Detecting runtime...\n")
	rt, err := runtime.SelectRuntimeWithOutput(runtimeEnv, cfg, out)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}
	util.ProgressStep(out, "Detected runtime: %s\n", rt.Name())

	// Validate Mutagen is available if any mount requires it
	if err := runtime.ValidateMutagenAvailable(runtimeEnv, cfg); err != nil {
		return err
	}

	// Validate mount excludes compatibility with runtime
	// See AGD-025 for rootless Podman + Mutagen limitations
	if err := runtime.ValidateMountExcludes(runtimeEnv, rt, cfg); err != nil {
		return fmt.Errorf("%w\n\nAlternatives:\n"+
			"  1. Remove 'exclude' from mount configuration\n"+
			"  2. Use rootful Podman (sudo podman)\n"+
			"  3. Use Docker instead", err)
	}

	// Network helper (handles all platform-specific logic)
	nh := network.NewNetworkHelper(cfg.Network, rt.Name())
	if nh != nil {
		if err := setupNetwork(nh, env, tfs, cwd, runtimeEnv, out); err != nil {
			return err
		}
		// Reset tfs/env for subsequent operations since network setup may have committed
		tfs = transact.New()
		env = &util.Env{Fs: tfs, Cmd: cmdRunner}
	}

	// Load or create state
	st, isNew, err := state.LoadOrCreate(env, cwd, rt.Name())
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if isNew {
		util.ProgressStep(out, "Created new state file: %s\n", state.StateFilePath(cwd))
	}

	// Check for configuration drift and handle rebuild
	needsRebuild, err := handleConfigDrift(cfg, st, rt, out, force)
	if err != nil {
		return err
	}

	// If rebuild needed, remove existing container first
	if needsRebuild {
		if err := rebuildContainerIfNeeded(runtimeEnv, cfg, st, rt, cwd, out); err != nil {
			return err
		}
	}

	// Update state with current config only when rebuilding or first time
	if needsRebuild || isNew {
		st.UpdateConfig(cfg)
		if err := state.Save(env, cwd, st); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
		// Commit state changes (project dir, normally no sudo needed)
		if err := commitWithSudo(env, tfs, out, ""); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Start container
	if err := rt.Up(runtimeEnv, cfg, cwd, st, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Setup firewall rules for network isolation
	// See AGD-027 for design decisions
	if err := setupFirewall(runtimeEnv, cmdRunner, cfg, rt, st, cwd, out); err != nil {
		// Firewall errors are warnings, not fatal - container is already running
		util.ProgressStep(out, "Warning: %v\n", err)
	}

	util.ProgressDone(out, "Environment ready\n")
	return nil
}

// handleConfigDrift checks for configuration drift and prompts user if needed.
// Returns true if rebuild is needed.
func handleConfigDrift(cfg *config.Config, st *state.State, rt runtime.Runtime, out io.Writer, force bool) (bool, error) {
	runtimeChanged := st.Runtime != rt.Name()
	drift := st.DetectConfigDrift(cfg)

	if drift == nil && !runtimeChanged {
		return false, nil
	}

	if force {
		util.ProgressStep(out, "Configuration changed, rebuilding container (-f)\n")
		return true, nil
	}

	// Show drift and ask for confirmation
	displayConfigDrift(out, drift, runtimeChanged, st.Runtime, rt.Name())

	if !promptConfirm("Rebuild container with new configuration?") {
		fmt.Println("Keeping existing container.")
		return false, nil
	}

	return true, nil
}

// rebuildContainerIfNeeded removes the existing container for rebuild.
func rebuildContainerIfNeeded(runtimeEnv *runtime.RuntimeEnv, cfg *config.Config, st *state.State, rt runtime.Runtime, cwd string, out io.Writer) error {
	// Determine which runtime to use for cleanup
	cleanupRt := rt
	runtimeChanged := st.Runtime != rt.Name()

	if runtimeChanged {
		// Runtime changed - use old runtime to remove old container
		if oldRt := runtime.ByName(st.Runtime); oldRt != nil {
			cleanupRt = oldRt
			util.ProgressStep(out, "Runtime changed: %s → %s\n", st.Runtime, rt.Name())
		}
	}
	status, _ := cleanupRt.Status(runtimeEnv, cwd, st)
	if status.State != runtime.StateNotFound {
		util.ProgressStep(out, "Removing existing container for rebuild...\n")
		if err := cleanupRt.Down(runtimeEnv, cwd, st); err != nil {
			return fmt.Errorf("failed to remove container for rebuild: %w", err)
		}
	}

	return nil
}

// setupNetwork configures network helper for LAN access.
// See AGD-023 for design decisions.
func setupNetwork(nh network.NetworkHelper, env *util.Env, tfs *transact.TransactFs, projectDir string, runtimeEnv *runtime.RuntimeEnv, out io.Writer) error {
	progress := func(format string, args ...any) {
		util.ProgressStep(out, format, args...)
	}

	isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
	networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, projectDir, isOrbStack)

	// Check and install helper if needed
	status := nh.HelperStatus(networkEnv)
	if !status.Installed {
		util.ProgressStep(out, "Network helper required for LAN access.\n")
		if !promptConfirm("Install now?") {
			return fmt.Errorf("LAN access requires network helper")
		}
	}

	if !status.Installed || status.NeedsUpdate {
		action, err := nh.InstallHelper(networkEnv, progress)
		if err != nil {
			return fmt.Errorf("failed to install network helper: %w", err)
		}

		if err := commitIfNeeded(env, tfs, out, "Writing network helper to system directories"); err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}

		if action.Run != nil {
			if err := action.Run(progress); err != nil {
				return fmt.Errorf("post-install failed: %w", err)
			}
		}
	}

	// Setup project rules
	action, err := nh.Setup(networkEnv, projectDir, progress)
	if err != nil {
		return fmt.Errorf("failed to setup network: %w", err)
	}

	if err := commitIfNeeded(env, tfs, out, "Writing firewall rules to system directories"); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if action.Run != nil {
		if err := action.Run(progress); err != nil {
			return fmt.Errorf("post-setup failed: %w", err)
		}
	}

	util.ProgressStep(out, "LAN access configured\n")
	return nil
}

// setupFirewall applies firewall rules for network isolation.
// Only applies when:
// - A firewall backend is available (nftables on Linux, pf on macOS)
// - lan-access is NOT ["*"] (user wants network isolation)
// See AGD-027 for design decisions.
func setupFirewall(runtimeEnv *runtime.RuntimeEnv, cmdRunner util.CommandRunner, cfg *config.Config, rt runtime.Runtime, st *state.State, projectDir string, out io.Writer) error {
	// Parse lan-access rules
	rules, err := network.ParseLANAccessRules(cfg.Network.LANAccess)
	if err != nil {
		return fmt.Errorf("invalid lan-access configuration: %w", err)
	}

	// lan-access = ["*"] means allow all, no firewall needed
	if network.HasAllLAN(rules) {
		return nil
	}

	// Detect firewall availability using shared command runner
	isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
	networkEnv := network.NewNetworkEnv(afero.NewOsFs(), cmdRunner, projectDir, isOrbStack)
	fw, fwType := network.New(networkEnv)

	// OrbStack warning: partial lan-access (specific IPs/ports) doesn't work due to NAT
	// Only lan-access=["*"] (allow all) or lan-access=[] (block all) are effective
	if fwType == network.TypePF && len(rules) > 0 {
		isOrbStack, _ := runtime.IsOrbStack(runtimeEnv)
		if isOrbStack {
			util.ProgressStep(out, `
⚠️  OrbStack partial lan-access limitation

OrbStack's NAT architecture prevents pf from filtering by container IP.
Only these lan-access modes work reliably with OrbStack:
  - lan-access: ["*"]  (allow all LAN access)
  - lan-access: []     (block all LAN access)

Your current config has partial access rules which may not be enforced.
For full lan-access control, use Docker Desktop instead of OrbStack.

`)
		}
	}

	if fwType == network.TypeNone {
		// No firewall available - emit warning per AGD-027
		util.ProgressStep(out, `
⚠️  Network isolation not available

Your system does not have a supported firewall backend.
The container will start WITHOUT network isolation - it can access LAN.

On Linux, install nftables:
  1. Install nftables: sudo apt install nftables  # or yum/dnf/pacman
  2. Ensure kernel version >= 3.13: uname -r
  3. Restart Alcatraz

`)
		return nil
	}

	if fw == nil {
		return nil
	}

	// Get container status to find the container name
	status, err := rt.Status(runtimeEnv, "", st)
	if err != nil || status.State != runtime.StateRunning {
		return fmt.Errorf("container not running, cannot apply firewall rules")
	}

	// Get container IP
	containerIP, err := rt.GetContainerIP(runtimeEnv, status.Name)
	if err != nil {
		return fmt.Errorf("failed to get container IP: %w", err)
	}

	util.ProgressStep(out, "Applying network isolation rules...\n")

	// Apply firewall rules with lan-access allowlist
	if err := fw.ApplyRules(status.ID, containerIP, rules); err != nil {
		return fmt.Errorf("failed to apply firewall rules: %w", err)
	}

	util.ProgressStep(out, "Network isolation enabled\n")
	return nil
}
