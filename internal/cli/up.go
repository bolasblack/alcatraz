package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
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
	ctx := cmd.Context()
	quiet, _ := cmd.Flags().GetBool("quiet")
	force, _ := cmd.Flags().GetBool("force")

	var out io.Writer = os.Stdout
	if quiet {
		out = nil
	}

	cwd, err := findProjectDir()
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
	rt, err := runtime.SelectRuntimeWithOutput(ctx, runtimeEnv, cfg, out)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}
	util.ProgressStep(out, "Detected runtime: %s\n", rt.Name())

	// TODO: extract to validateMounts(runtimeEnv, rt, cfg) — mount-related validations
	// Validate Mutagen is available if any mount requires it
	if err := runtime.ValidateMutagenAvailable(ctx, runtimeEnv, cfg); err != nil {
		return err
	}

	// Validate mount excludes compatibility with runtime
	// See AGD-025 for rootless Podman + Mutagen limitations
	if err := runtime.ValidateMountExcludes(ctx, runtimeEnv, rt, cfg); err != nil {
		return fmt.Errorf("%w\n\nAlternatives:\n"+
			"  1. Remove 'exclude' from mount configuration\n"+
			"  2. Use rootful Podman (sudo podman)\n"+
			"  3. Use Docker instead", err)
	}

	// Detect platform once for all network operations
	platform := runtime.DetectPlatform(ctx, runtimeEnv)

	// Load or create state early — ProjectID is needed by network env
	st, isNew, err := state.LoadOrCreate(env, cwd, rt.Name())
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if isNew {
		util.ProgressStep(out, "Created new state file: %s\n", state.StateFilePath(cwd))
	}

	// Create shared network env once for all network operations (AGD-029)
	networkEnv := network.NewNetworkEnv(tfs, deps.CmdRunner, cwd, st.ProjectID, platform)

	// Network helper (handles all platform-specific logic)
	nh := network.NewNetworkHelperForProject(cfg.Network, platform)
	if nh != nil {
		if err := setupNetwork(ctx, nh, networkEnv, env, tfs, out); err != nil {
			return err
		}
	}

	// Check for configuration drift and handle rebuild.
	// Only relevant when a container exists — after 'alca down' there's
	// nothing to rebuild, so skip drift detection and create fresh.
	needsRebuild, err := handleConfigDrift(ctx, cfg, st, rt, runtimeEnv, cwd, out, force)
	if err != nil {
		return err
	}

	// If rebuild needed, remove existing container first
	if needsRebuild {
		if err := rebuildContainerIfNeeded(ctx, runtimeEnv, cfg, st, rt, cwd, out); err != nil {
			return err
		}
	}

	// TODO: extract to saveStateIfNeeded(env, tfs, cfg, st, cwd, out) — state persistence
	// Update state with current config when creating fresh, rebuilding, or first time.
	// "Creating fresh" = container was removed (e.g., alca down) but state.json persists.
	if needsRebuild || isNew || containerMissing(ctx, rt, runtimeEnv, cwd, st) {
		st.UpdateConfig(cfg)
		if err := state.Save(env, cwd, st); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
		// Commit state changes (project dir, normally no sudo needed)
		if err := commitWithSudo(ctx, env, tfs, out, ""); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Start container
	if err := rt.Up(ctx, runtimeEnv, cfg, cwd, st, out); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Setup firewall rules for network isolation
	// See AGD-027 for design decisions
	// Files written via tfs, committed to real disk before nft loads them.
	fw, fwType := network.New(ctx, networkEnv)

	expandedNet, fwErr := setupFirewall(ctx, fw, fwType, networkEnv, env, tfs, runtimeEnv, cfg.Network, rt, st, nh, out)
	if fwErr != nil {
		if errors.Is(fwErr, errSkipFirewall) {
			// User declined helper install — already messaged, not an error
		} else {
			// Firewall errors are warnings, not fatal - container is already running
			util.ProgressStep(out, "Warning: %v\n", fwErr)
		}
	} else {
		// Persist expanded network config (tokens resolved to IPs) to state.
		// Always runs on success — even after rebuild, because UpdateConfig saves
		// raw tokens and we need the resolved values in state.
		if err := saveNetworkState(ctx, env, tfs, cwd, expandedNet, st, out); err != nil {
			return err
		}
	}

	// Show sync conflict banner if any (best-effort, errors ignored).
	syncEnv := sync.NewSyncEnv(afero.NewOsFs(), deps.CmdRunner, runtime.NewMutagenSyncClient(runtimeEnv))
	showSyncBanner(ctx, syncEnv, st.ProjectID, cwd, os.Stderr)

	util.ProgressDone(out, "Environment ready\n")
	return nil
}

func containerMissing(ctx context.Context, rt runtime.Runtime, runtimeEnv *runtime.RuntimeEnv, cwd string, st *state.State) bool {
	s, _ := rt.Status(ctx, runtimeEnv, cwd, st)
	return s.State == runtime.StateNotFound
}

// handleConfigDrift checks for configuration drift and prompts user if needed.
// Returns true if rebuild is needed.
// Skips drift detection when no container exists (e.g., after 'alca down') —
// there's nothing to rebuild, just create fresh with current config.
func handleConfigDrift(ctx context.Context, cfg *config.Config, st *state.State, rt runtime.Runtime, runtimeEnv *runtime.RuntimeEnv, cwd string, out io.Writer, force bool) (bool, error) {
	// No container → no drift. Create fresh.
	if containerMissing(ctx, rt, runtimeEnv, cwd, st) {
		return false, nil
	}

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
func rebuildContainerIfNeeded(ctx context.Context, runtimeEnv *runtime.RuntimeEnv, cfg *config.Config, st *state.State, rt runtime.Runtime, cwd string, out io.Writer) error {
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
	status, _ := cleanupRt.Status(ctx, runtimeEnv, cwd, st)
	if status.State != runtime.StateNotFound {
		util.ProgressStep(out, "Removing existing container for rebuild...\n")
		if err := cleanupRt.Down(ctx, runtimeEnv, cwd, st); err != nil {
			return fmt.Errorf("failed to remove container for rebuild: %w", err)
		}
	}

	return nil
}

// setupNetwork configures network helper for LAN access.
// See AGD-030 for design decisions.
func setupNetwork(ctx context.Context, nh network.NetworkHelper, networkEnv *network.NetworkEnv, env *util.Env, tfs *transact.TransactFs, out io.Writer) error {
	progress := progressFunc(out)

	// Check and install helper if needed
	status := nh.HelperStatus(ctx, networkEnv)
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

		if err := commitIfNeeded(ctx, env, tfs, out, "Writing network helper to system directories"); err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}

		if action.Run != nil {
			if err := action.Run(ctx, progress); err != nil {
				return fmt.Errorf("post-install failed: %w", err)
			}
		}
	}

	// Setup project rules
	action, err := nh.Setup(networkEnv, networkEnv.ProjectDir, progress)
	if err != nil {
		return fmt.Errorf("failed to setup network: %w", err)
	}

	if err := commitIfNeeded(ctx, env, tfs, out, "Writing firewall rules to system directories"); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if action.Run != nil {
		if err := action.Run(ctx, progress); err != nil {
			return fmt.Errorf("post-setup failed: %w", err)
		}
	}

	util.ProgressStep(out, "LAN access configured\n")
	return nil
}

// setupFirewall applies firewall rules for network isolation and transparent proxy.
// Handles both lan-access isolation (AGD-027) and proxy DNAT (AGD-037) in one call.
//
// On success, returns a Network with expanded fields (alca tokens resolved to IPs).
// The caller should persist this expanded config — not the raw cfg.Network — so that
// state reflects what was actually applied.
func setupFirewall(ctx context.Context, fw network.Firewall, fwType network.Type, networkEnv *network.NetworkEnv, env *util.Env, tfs *transact.TransactFs, runtimeEnv *runtime.RuntimeEnv, netCfg config.Network, rt runtime.Runtime, st *state.State, nh network.NetworkHelper, out io.Writer) (config.Network, error) {
	// Clean up stale rule files unconditionally — must run even when
	// HasAllLAN or TypeNone would cause early returns below.
	if fw != nil {
		if staleCount, err := fw.CleanupStaleFiles(ctx); err != nil {
			util.ProgressStep(out, "Warning: stale rule cleanup: %v\n", err)
		} else if staleCount > 0 {
			util.ProgressStep(out, "Cleaned up %d stale firewall rule file(s)\n", staleCount)
		}
	}

	// Create token resolver once for both lan-access and proxy expansion.
	// HOST_IP is resolved at most once via the resolver's internal cache,
	// avoiding duplicate resolution when both lan-access and proxy are configured.
	resolver := newAlcaTokenResolver(ctx, runtimeEnv, rt)

	// Expand alca tokens in lan-access rules before parsing (AGD-036)
	expandedLANAccess, err := config.ExpandAlcaTokensInStrings(netCfg.LANAccess, resolver)
	if err != nil {
		return config.Network{}, fmt.Errorf("expanding lan-access tokens: %w", err)
	}

	// Mirror type ensures all Network fields are carried forward (AGD-015).
	// Missing a field here causes false drift detection on every `alca up`.
	type networkFields struct {
		LANAccess []string
		Ports     []config.PortConfig
		Proxy     string
	}

	expandedNet := config.Network{
		LANAccess: expandedLANAccess,
		Ports:     netCfg.Ports,
		Proxy:     netCfg.Proxy,
	}
	_ = networkFields(expandedNet) // AGD-015: compile-time check on actual value

	// Parse lan-access rules
	rules, err := network.ParseLANAccessRules(expandedLANAccess)
	if err != nil {
		return config.Network{}, fmt.Errorf("invalid lan-access configuration: %w", err)
	}

	// Expand and parse proxy config (AGD-037)
	var proxy *network.ProxyConfig
	if netCfg.Proxy != "" {
		expandedProxy, err := config.ExpandAlcaTokens(netCfg.Proxy, resolver)
		if err != nil {
			return config.Network{}, fmt.Errorf("expanding proxy address tokens: %w", err)
		}
		expandedNet.Proxy = expandedProxy
		proxyHost, proxyPort, err := config.ParseProxyAddress(expandedProxy)
		if err != nil {
			return config.Network{}, fmt.Errorf("invalid proxy address after expansion: %w", err)
		}
		// Mirror type ensures ProxyConfig field drift is caught at compile time (AGD-015).
		type proxyFields struct {
			Host string
			Port int
		}
		_ = proxyFields(network.ProxyConfig{})
		proxy = &network.ProxyConfig{Host: proxyHost, Port: proxyPort}
	}

	// Determine if any nftables work is needed
	hasIsolation := !network.HasAllLAN(rules)
	hasProxy := proxy != nil
	if !hasIsolation && !hasProxy {
		return expandedNet, nil
	}

	// The network helper must be installed for nft reload.
	if err := ensureNetworkHelper(ctx, nh, networkEnv, env, tfs, out); err != nil {
		return config.Network{}, err
	}

	if fwType == network.TypeNone {
		feature := "Network isolation"
		if hasProxy && hasIsolation {
			feature = "Network isolation and transparent proxy"
		} else if hasProxy {
			feature = "Transparent proxy"
		}
		proxyFallbackHint := ""
		if hasProxy {
			proxyFallbackHint = `
Alternatively, set HTTP_PROXY/HTTPS_PROXY environment variables in your
config to route traffic through your proxy without nftables.
`
		}
		util.ProgressStep(out, `
⚠️  %s not available

Your system does not have a supported firewall backend.
The container will start WITHOUT network rules.

On Linux, install nftables:
  1. Install nftables: sudo apt install nftables  # or yum/dnf/pacman
  2. Ensure kernel version >= 3.13: uname -r
  3. Restart Alcatraz
%s
`, feature, proxyFallbackHint)
		return expandedNet, nil
	}

	if fw == nil {
		return expandedNet, nil
	}

	// Get container status to find the container name
	status, err := rt.Status(ctx, runtimeEnv, "", st)
	if err != nil || status.State != runtime.StateRunning {
		return config.Network{}, fmt.Errorf("container not running, cannot apply firewall rules")
	}

	// Get container IP
	containerIP, err := rt.GetContainerIP(ctx, runtimeEnv, status.Name)
	if err != nil {
		return config.Network{}, fmt.Errorf("failed to get container IP: %w", err)
	}

	if hasIsolation {
		util.ProgressStep(out, "Applying network isolation rules...\n")
	}
	if hasProxy {
		util.ProgressStep(out, "Applying transparent proxy rules (→ %s:%d)...\n", proxy.Host, proxy.Port)
	}

	// Apply all firewall rules — isolation + proxy (writes files via tfs)
	// NOTE: ApplyRules has 4 positional params (containerID, containerIP, rules, proxy).
	// If more params are added, consider a params struct to improve readability and
	// reduce positional coupling. Not refactored now to avoid cross-module churn.
	action, err := fw.ApplyRules(status.ID, containerIP, rules, proxy)
	if err != nil {
		return config.Network{}, fmt.Errorf("failed to apply firewall rules: %w", err)
	}

	// Commit tfs to write files to real disk
	if err := commitIfNeeded(ctx, env, tfs, out, "Writing firewall rules"); err != nil {
		return config.Network{}, fmt.Errorf("commit firewall files: %w", err)
	}

	// Run post-commit action (nft -f or reload)
	if action != nil && action.Run != nil {
		if err := action.Run(ctx, nil); err != nil {
			return config.Network{}, fmt.Errorf("load firewall rules: %w", err)
		}
	}

	if hasIsolation {
		util.ProgressStep(out, "Network isolation enabled\n")
	}
	if hasProxy {
		util.ProgressStep(out, "Transparent proxy enabled\n")
	}
	return expandedNet, nil
}

// ensureNetworkHelper checks if the network helper is installed and prompts to install if needed.
// Returns nil if the helper is already installed or was successfully installed.
// Returns a non-nil error sentinel to signal the caller to skip firewall setup (not a real error).
func ensureNetworkHelper(ctx context.Context, nh network.NetworkHelper, networkEnv *network.NetworkEnv, env *util.Env, tfs *transact.TransactFs, out io.Writer) error {
	if nh == nil {
		return fmt.Errorf("no network helper available for this platform")
	}

	status := nh.HelperStatus(ctx, networkEnv)
	if status.Installed {
		return nil
	}

	util.ProgressStep(out, "Network helper required for network isolation.\n")
	if !promptConfirm("Install now?") {
		util.ProgressStep(out, "Skipping network isolation — helper not installed\n")
		return errSkipFirewall
	}

	// Install the helper (same flow as setupNetwork)
	progress := progressFunc(out)
	action, err := nh.InstallHelper(networkEnv, progress)
	if err != nil {
		return fmt.Errorf("failed to install network helper: %w", err)
	}

	if err := commitIfNeeded(ctx, env, tfs, out, "Writing network helper to system directories"); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if action.Run != nil {
		if err := action.Run(ctx, progress); err != nil {
			return fmt.Errorf("post-install failed: %w", err)
		}
	}

	return nil
}

// newAlcaTokenResolver creates a resolver func for ${alca:...} tokens.
// Caches HOST_IP resolution so it's resolved at most once per call site.
func newAlcaTokenResolver(ctx context.Context, runtimeEnv *runtime.RuntimeEnv, rt runtime.Runtime) func(string) (string, error) {
	var hostIP string
	var hostIPErr error
	var hostIPResolved bool

	return func(name string) (string, error) {
		if name != "HOST_IP" {
			return "", fmt.Errorf("unhandled alca token %q", name)
		}
		if !hostIPResolved {
			hostIP, hostIPErr = rt.GetHostIP(ctx, runtimeEnv)
			hostIPResolved = true
		}
		return hostIP, hostIPErr
	}
}

// expandAlcaTokensInRules expands ${alca:...} tokens in lan-access rule strings.
// Resolves HOST_IP once (cached) via rt.GetHostIP, then expands all rules.
// Rules without alca tokens are passed through unchanged.
func expandAlcaTokensInRules(ctx context.Context, runtimeEnv *runtime.RuntimeEnv, rt runtime.Runtime, rules []string) ([]string, error) {
	return config.ExpandAlcaTokensInStrings(rules, newAlcaTokenResolver(ctx, runtimeEnv, rt))
}

// saveNetworkState selectively persists the network config section to state.
// Only updates Network — preserves drift signals for image, mounts, etc.
// LANAccess is excluded from drift detection in compareConfigs, so without this
// the state becomes stale when only Network.LANAccess changes.
func saveNetworkState(ctx context.Context, env *util.Env, tfs *transact.TransactFs, cwd string, netCfg config.Network, st *state.State, out io.Writer) error {
	st.Config.Network = netCfg
	if err := state.Save(env, cwd, st); err != nil {
		return fmt.Errorf("failed to save network state: %w", err)
	}
	if err := commitWithSudo(ctx, env, tfs, out, ""); err != nil {
		return fmt.Errorf("failed to save network state: %w", err)
	}
	return nil
}
