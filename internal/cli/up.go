package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/sync"
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
	deps := newCLIDeps()
	tfs, env, runtimeEnv := deps.Tfs, deps.Env, deps.RuntimeEnv

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

	// TODO: extract to validateMounts(runtimeEnv, rt, cfg) — mount-related validations
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

	// Detect platform once for all network operations
	platform := runtime.DetectPlatform(runtimeEnv)

	// Network helper (handles all platform-specific logic)
	nh := network.NewNetworkHelper(cfg.Network, platform)
	if nh != nil {
		if err := setupNetwork(nh, env, tfs, cwd, platform, out); err != nil {
			return err
		}
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

	// TODO: extract to saveStateIfNeeded(env, tfs, cfg, st, cwd, out) — state persistence
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
	// Files written via tfs, committed to real disk before nft loads them.
	firewallEnv := network.NewNetworkEnv(tfs, deps.CmdRunner, cwd, platform)
	if err := setupFirewall(firewallEnv, env, tfs, runtimeEnv, cfg.Network, rt, st, out); err != nil {
		if errors.Is(err, errSkipFirewall) {
			// User declined helper install — already messaged, not an error
		} else {
			// Firewall errors are warnings, not fatal - container is already running
			util.ProgressStep(out, "Warning: %v\n", err)
		}
	}

	// Show sync conflict banner if any (best-effort, errors ignored).
	syncEnv := sync.NewSyncEnv(afero.NewOsFs(), deps.CmdRunner, runtime.NewMutagenSyncClient(runtimeEnv))
	showSyncBanner(syncEnv, st.ProjectID, cwd, os.Stderr)

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
// See AGD-030 for design decisions.
func setupNetwork(nh network.NetworkHelper, env *util.Env, tfs *transact.TransactFs, projectDir string, platform runtime.RuntimePlatform, out io.Writer) error {
	progress := progressFunc(out)

	networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, projectDir, platform)

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
// - A firewall backend is available (nftables on Linux, nftables via VM on macOS)
// - lan-access is NOT ["*"] (user wants network isolation)
// See AGD-027 for design decisions.
func setupFirewall(networkEnv *network.NetworkEnv, env *util.Env, tfs *transact.TransactFs, runtimeEnv *runtime.RuntimeEnv, netCfg config.Network, rt runtime.Runtime, st *state.State, out io.Writer) error {
	// Parse lan-access rules
	rules, err := network.ParseLANAccessRules(netCfg.LANAccess)
	if err != nil {
		return fmt.Errorf("invalid lan-access configuration: %w", err)
	}

	// lan-access = ["*"] means allow all, no firewall needed
	if network.HasAllLAN(rules) {
		return nil
	}

	// On darwin, the VM helper must be installed for nft reload.
	// If setupNetwork didn't run (e.g. lan-access is empty), the helper may not be installed.
	if runtime.IsDarwin(networkEnv.Runtime) {
		if err := ensureVMHelper(networkEnv, env, tfs, out); err != nil {
			return err
		}
	}

	// Detect firewall availability
	fw, fwType := network.New(networkEnv)

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

	// Apply firewall rules with lan-access allowlist (writes files via tfs)
	action, err := fw.ApplyRules(status.ID, containerIP, rules)
	if err != nil {
		return fmt.Errorf("failed to apply firewall rules: %w", err)
	}

	// Commit tfs to write files to real disk
	if err := commitIfNeeded(env, tfs, out, "Writing firewall rules"); err != nil {
		return fmt.Errorf("commit firewall files: %w", err)
	}

	// Run post-commit action (nft -f or reload)
	if action != nil && action.Run != nil {
		if err := action.Run(nil); err != nil {
			return fmt.Errorf("load firewall rules: %w", err)
		}
	}

	util.ProgressStep(out, "Network isolation enabled\n")
	return nil
}

// ensureVMHelper checks if the VM helper is installed on darwin and prompts to install if needed.
// Returns nil if the helper is already installed or was successfully installed.
// Returns a non-nil error sentinel to signal the caller to skip firewall setup (not a real error).
func ensureVMHelper(networkEnv *network.NetworkEnv, env *util.Env, tfs *transact.TransactFs, out io.Writer) error {
	vmEnv := vmhelper.NewVMHelperEnv(networkEnv.Fs, networkEnv.Cmd)

	installed, err := vmhelper.IsInstalled(vmEnv)
	if err != nil {
		return fmt.Errorf("failed to check VM helper status: %w", err)
	}
	if installed {
		return nil
	}

	util.ProgressStep(out, "Network helper required for network isolation.\n")
	if !promptConfirm("Install now?") {
		util.ProgressStep(out, "Skipping network isolation — helper not installed\n")
		return errSkipFirewall
	}

	// Install the helper (same flow as setupNetwork)
	progress := progressFunc(out)
	nh := network.NewNetworkHelper(
		config.Network{LANAccess: []string{"_placeholder"}},
		networkEnv.Runtime,
	)
	if nh == nil {
		return fmt.Errorf("failed to create network helper for install")
	}

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

	return nil
}

// errSkipFirewall is a sentinel error used to skip firewall setup without reporting an error.
var errSkipFirewall = errors.New("skip firewall")
