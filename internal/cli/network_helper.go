package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

var networkHelperCmd = &cobra.Command{
	Use:   "network-helper",
	Short: "Manage network helper for LAN access",
	Long: `Manage the network helper for container LAN access.

On macOS: Installs a LaunchDaemon that watches pf anchor files and
automatically reloads firewall rules when they change.

On Linux: Configures nftables to include alcatraz rule files from
/etc/nftables.d/alcatraz/ for persistent firewall rules.`,
}

var networkHelperInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the network helper",
	Long: `Install the network helper for automatic firewall rule management.

On macOS:
1. Create /etc/pf.anchors/alcatraz/ directory
2. Install LaunchDaemon plist to /Library/LaunchDaemons/
3. Load the LaunchDaemon

On Linux:
1. Create /etc/nftables.d/alcatraz/ directory
2. Add include line to /etc/nftables.conf
3. Reload nftables configuration

Requires sudo privileges.`,
	RunE: runNetworkHelperInstall,
}

var networkHelperUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the network helper",
	Long: `Uninstall the network helper and clean up all rules.

On macOS:
1. Unload the LaunchDaemon
2. Remove the plist file
3. Flush all alcatraz pf rules
4. Remove /etc/pf.anchors/alcatraz/ directory

On Linux:
1. Remove all rule files from /etc/nftables.d/alcatraz/
2. Remove include line from /etc/nftables.conf
3. Delete all alca-* nftables tables
4. Remove /etc/nftables.d/alcatraz/ directory

Requires sudo privileges.`,
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

// runNetworkHelperInstall installs the LaunchDaemon.
// See AGD-023 for lifecycle details.
func runNetworkHelperInstall(cmd *cobra.Command, args []string) error {
	tfs := transact.New()
	cmdRunner := util.NewCommandRunner()

	env := &util.Env{Fs: tfs, Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

	// Detect runtime for network helper
	runtimeName, err := detectRuntimeForNetworkHelper(runtimeEnv)
	if err != nil {
		return err
	}

	nh := network.NewNetworkHelper(config.Network{LANAccess: []string{"*"}}, runtimeName)
	if nh == nil {
		fmt.Println("Network helper not needed on this platform/runtime.")
		return nil
	}

	isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
	networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, "", isOrbStack)

	status := nh.HelperStatus(networkEnv)
	if status.Installed && !status.NeedsUpdate {
		fmt.Println("Network helper already installed and up to date.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will install the network helper to manage firewall rules.")
	if !promptConfirm("Continue?") {
		return nil
	}

	progress := stdoutProgressFunc()
	action, err := nh.InstallHelper(networkEnv, progress)
	if err != nil {
		return err
	}

	if err := commitIfNeeded(env, tfs, os.Stdout, "Writing system files"); err != nil {
		return err
	}

	if action.Run != nil {
		if err := action.Run(progress); err != nil {
			return err
		}
	}

	util.ProgressDone(os.Stdout, "Network helper installed.\n")
	return nil
}

// runNetworkHelperUninstall removes the LaunchDaemon and cleans up.
// See AGD-023 for lifecycle details.
func runNetworkHelperUninstall(cmd *cobra.Command, args []string) error {
	tfs := transact.New()
	cmdRunner := util.NewCommandRunner()

	env := &util.Env{Fs: tfs, Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

	runtimeName, err := detectRuntimeForNetworkHelper(runtimeEnv)
	if err != nil {
		return err
	}

	nh := network.NewNetworkHelper(config.Network{LANAccess: []string{"*"}}, runtimeName)
	if nh == nil {
		fmt.Println("Network helper not installed.")
		return nil
	}

	isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
	networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, "", isOrbStack)

	status := nh.HelperStatus(networkEnv)
	if !status.Installed {
		fmt.Println("Network helper not installed.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will remove the network helper and all rules.")
	if !promptConfirm("Continue?") {
		return nil
	}

	progress := stdoutProgressFunc()
	action, err := nh.UninstallHelper(networkEnv, progress)
	if err != nil {
		return err
	}

	if err := commitIfNeeded(env, tfs, os.Stdout, "Removing system files"); err != nil {
		return err
	}

	if action.Run != nil {
		if err := action.Run(progress); err != nil {
			return err
		}
	}

	util.ProgressDone(os.Stdout, "Network helper uninstalled.\n")
	return nil
}

// runNetworkHelperStatus shows the current status.
func runNetworkHelperStatus(cmd *cobra.Command, args []string) error {
	cmdRunner := util.NewCommandRunner()

	env := &util.Env{Fs: afero.NewReadOnlyFs(afero.NewOsFs()), Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

	runtimeName, err := detectRuntimeForNetworkHelper(runtimeEnv)
	if err != nil {
		return err
	}

	nh := network.NewNetworkHelper(config.Network{LANAccess: []string{"*"}}, runtimeName)
	if nh == nil {
		fmt.Println("Network helper not applicable on this platform/runtime.")
		return nil
	}

	isOrbStack := runtime.DetectPlatform(runtimeEnv) == runtime.PlatformMacOrbStack
	networkEnv := network.NewNetworkEnv(env.Fs, env.Cmd, "", isOrbStack)

	status := nh.HelperStatus(networkEnv)

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
	printRuleFiles(detailed)
	printActivePfRules(detailed)

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

func printActivePfRules(status network.DetailedStatusInfo) {
	fmt.Println("")
	fmt.Println("Active firewall rules:")

	if !status.DaemonLoaded {
		fmt.Println("  (daemon not loaded - rules may be stale)")
	}

	if len(status.RuleFiles) == 0 {
		fmt.Println("  (none)")
		return
	}

	for _, f := range status.RuleFiles {
		fmt.Printf("  [%s]\n", f.Name)
		for _, line := range strings.Split(f.Content, "\n") {
			if line != "" {
				fmt.Printf("    %s\n", line)
			}
		}
	}
}

// detectRuntimeForNetworkHelper returns the runtime name for network helper operations.
// Returns "orbstack" if OrbStack is detected, "docker" otherwise.
func detectRuntimeForNetworkHelper(runtimeEnv *runtime.RuntimeEnv) (string, error) {
	isOrbStack, err := runtime.IsOrbStack(runtimeEnv)
	if err != nil {
		return "", fmt.Errorf("failed to detect runtime: %w", err)
	}
	if isOrbStack {
		return "orbstack", nil
	}
	return "docker", nil
}
