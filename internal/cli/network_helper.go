package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

var networkHelperCmd = &cobra.Command{
	Use:   "network-helper",
	Short: "Manage network helper for LAN access",
	Long: `Manage the network helper for container LAN access.

On macOS: Installs a helper container that runs nftables inside the
container runtime VM for network isolation.

On Linux: Configures nftables to include alcatraz rule files from
/etc/nftables.d/alcatraz/ for persistent firewall rules.`,
}

var networkHelperInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the network helper",
	Long: `Install the network helper for automatic firewall rule management.

On macOS:
1. Create ~/.alcatraz/files/alcatraz_nft/ directory
2. Start the alcatraz-network-helper container

On Linux:
1. Create /etc/nftables.d/alcatraz/ directory
2. Add include line to /etc/nftables.conf
3. Reload nftables configuration

Requires sudo privileges on Linux.`,
	RunE: runNetworkHelperInstall,
}

var networkHelperUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the network helper",
	Long: `Uninstall the network helper and clean up all rules.

On macOS:
1. Stop and remove the alcatraz-network-helper container

On Linux:
1. Remove all rule files from /etc/nftables.d/alcatraz/
2. Remove include line from /etc/nftables.conf
3. Delete all alca-* nftables tables
4. Remove /etc/nftables.d/alcatraz/ directory

Requires sudo privileges on Linux.`,
	RunE: runNetworkHelperUninstall,
}

var networkHelperStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show network helper status",
	Long:  `Display the current status of the network helper, including LaunchDaemon state and active rules.`,
	RunE:  runNetworkHelperStatus,
}

func init() {
	networkHelperCmd.AddCommand(networkHelperInstallCmd)
	networkHelperCmd.AddCommand(networkHelperUninstallCmd)
	networkHelperCmd.AddCommand(networkHelperStatusCmd)
}

// networkHelperSetup holds the shared dependencies for network-helper subcommands.
type networkHelperSetup struct {
	deps       cliDeps
	platform   runtime.RuntimePlatform
	nh         network.NetworkHelper
	networkEnv *network.NetworkEnv
}

// newNetworkHelperSetup creates shared deps for network-helper install/uninstall.
// Returns nil if the network helper is not applicable on this platform.
func newNetworkHelperSetup(ctx context.Context) *networkHelperSetup {
	deps := newCLIDeps()
	platform := runtime.DetectPlatform(ctx, deps.RuntimeEnv)
	nh := network.NewNetworkHelper(config.Network{LANAccess: []string{"*"}}, platform)
	if nh == nil {
		return nil
	}
	networkEnv := network.NewNetworkEnv(deps.Env.Fs, deps.Env.Cmd, "", platform)
	return &networkHelperSetup{
		deps:       deps,
		platform:   platform,
		nh:         nh,
		networkEnv: networkEnv,
	}
}

// runNetworkHelperInstall installs the LaunchDaemon.
// See AGD-030 for lifecycle details.
func runNetworkHelperInstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	setup := newNetworkHelperSetup(ctx)
	if setup == nil {
		fmt.Println("Network helper not needed on this platform/runtime.")
		return nil
	}

	nh, networkEnv := setup.nh, setup.networkEnv

	status := nh.HelperStatus(ctx, networkEnv)
	if status.Installed && !status.NeedsUpdate {
		fmt.Println("Network helper already installed and up to date.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will install the network helper to manage firewall rules.")
	if !promptConfirm("Continue?") {
		return nil
	}

	progress := progressFunc(os.Stdout)
	action, err := nh.InstallHelper(networkEnv, progress)
	if err != nil {
		return err
	}

	if err := commitIfNeeded(ctx, setup.deps.Env, setup.deps.Tfs, os.Stdout, "Writing system files"); err != nil {
		return err
	}

	if action.Run != nil {
		if err := action.Run(ctx, progress); err != nil {
			return err
		}
	}

	util.ProgressDone(os.Stdout, "Network helper installed.\n")
	return nil
}

// runNetworkHelperUninstall removes the LaunchDaemon and cleans up.
// See AGD-030 for lifecycle details.
func runNetworkHelperUninstall(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	setup := newNetworkHelperSetup(ctx)
	if setup == nil {
		fmt.Println("Network helper not installed.")
		return nil
	}

	nh, networkEnv := setup.nh, setup.networkEnv

	status := nh.HelperStatus(ctx, networkEnv)
	if !status.Installed {
		fmt.Println("Network helper not installed.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will remove the network helper and all rules.")
	if !promptConfirm("Continue?") {
		return nil
	}

	progress := progressFunc(os.Stdout)
	action, err := nh.UninstallHelper(networkEnv, progress)
	if err != nil {
		return err
	}

	if err := commitIfNeeded(ctx, setup.deps.Env, setup.deps.Tfs, os.Stdout, "Removing system files"); err != nil {
		return err
	}

	if action.Run != nil {
		if err := action.Run(ctx, progress); err != nil {
			return err
		}
	}

	util.ProgressDone(os.Stdout, "Network helper uninstalled.\n")
	return nil
}

// runNetworkHelperStatus shows the current status.
func runNetworkHelperStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	deps := newCLIReadDeps()
	platform := runtime.DetectPlatform(ctx, deps.RuntimeEnv)
	nh := network.NewNetworkHelper(config.Network{LANAccess: []string{"*"}}, platform)
	if nh == nil {
		fmt.Println("Network helper not applicable on this platform/runtime.")
		return nil
	}

	networkEnv := network.NewNetworkEnv(deps.Env.Fs, deps.Env.Cmd, "", platform)

	status := nh.HelperStatus(ctx, networkEnv)

	fmt.Println("Network Helper Status")
	fmt.Println("=====================")
	if status.Installed {
		fmt.Println("Status: Installed")
		if status.NeedsUpdate {
			fmt.Println("Update: Available")
		}
	} else {
		fmt.Println("Status: Not installed")
	}

	// Detailed status from the implementation
	detailed := nh.DetailedStatus(networkEnv)
	printHelperSummary(status, detailed)
	printRuleFiles(detailed)

	return nil
}

func printRuleFiles(status network.DetailedStatusInfo) {
	fmt.Println("")
	fmt.Println("Rule files:")
	if len(status.RuleFiles) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, f := range status.RuleFiles {
			fmt.Printf("  - %s\n", f.Name)
		}
	}
}

func printHelperSummary(helperStatus network.HelperStatus, detailedStatus network.DetailedStatusInfo) {
	fmt.Println("")
	fmt.Println("Helper Summary:")

	// (1) Installation status
	if helperStatus.Installed {
		fmt.Println("  Installed: Yes")
	} else {
		fmt.Println("  Installed: No")
	}

	// (2) Rules applied status
	rulesApplied := len(detailedStatus.RuleFiles) > 0
	if rulesApplied {
		fmt.Printf("  Rules applied: Yes (%d rule files)\n", len(detailedStatus.RuleFiles))
	} else {
		fmt.Println("  Rules applied: No")
	}
}
