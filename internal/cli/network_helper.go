package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/spf13/cobra"
)

var networkHelperCmd = &cobra.Command{
	Use:   "network-helper",
	Short: "Manage network helper for macOS LAN access",
	Long: `Manage the network helper LaunchDaemon for macOS LAN access.

The network helper installs a LaunchDaemon that watches pf anchor files
and automatically reloads firewall rules when they change. This is required
for OrbStack containers to access LAN hosts.`,
}

var networkHelperInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the network helper LaunchDaemon",
	Long: `Install the pf-watcher LaunchDaemon for automatic firewall rule management.

This will:
1. Create /etc/pf.anchors/alcatraz/ directory
2. Install LaunchDaemon plist to /Library/LaunchDaemons/
3. Load the LaunchDaemon

Requires sudo privileges.`,
	RunE: runNetworkHelperInstall,
}

var networkHelperUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the network helper LaunchDaemon",
	Long: `Uninstall the pf-watcher LaunchDaemon and clean up all rules.

This will:
1. Unload the LaunchDaemon
2. Remove the plist file
3. Flush all alcatraz pf rules
4. Remove /etc/pf.anchors/alcatraz/ directory

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
	// Check if already installed
	if network.IsLaunchDaemonLoaded() {
		fmt.Println("Network helper is already installed and loaded.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will install a LaunchDaemon to manage pf firewall rules.")
	fmt.Println("")
	fmt.Println("The following changes will be made:")
	fmt.Printf("  - Create directory: %s\n", network.PfAnchorDir)
	fmt.Printf("  - Install plist: %s\n", network.LaunchDaemonPath)
	fmt.Println("  - Load LaunchDaemon via launchctl")
	fmt.Println("")
	if !promptConfirm("Continue?") {
		fmt.Println("Installation cancelled.")
		return nil
	}

	fmt.Println("")

	if err := network.InstallHelper(func(format string, args ...any) {
		progressStep(os.Stdout, format, args...)
	}); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Network helper installed successfully.")
	return nil
}

// runNetworkHelperUninstall removes the LaunchDaemon and cleans up.
// See AGD-023 for lifecycle details.
func runNetworkHelperUninstall(cmd *cobra.Command, args []string) error {
	// Check if installed
	plistExists := network.FileExists(network.LaunchDaemonPath)
	dirExists := network.FileExists(network.PfAnchorDir)

	if !plistExists && !dirExists && !network.IsLaunchDaemonLoaded() {
		fmt.Println("Network helper is not installed.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will uninstall the network helper and remove all rules.")
	fmt.Println("")
	fmt.Println("The following changes will be made:")
	if network.IsLaunchDaemonLoaded() {
		fmt.Println("  - Unload LaunchDaemon via launchctl")
	}
	if plistExists {
		fmt.Printf("  - Remove plist: %s\n", network.LaunchDaemonPath)
	}
	fmt.Printf("  - Flush all pf rules in anchor: %s\n", network.PfAnchorName)
	if dirExists {
		fmt.Printf("  - Remove directory: %s\n", network.PfAnchorDir)
	}
	fmt.Println("")
	if !promptConfirm("Continue?") {
		fmt.Println("Uninstallation cancelled.")
		return nil
	}

	fmt.Println("")

	// Perform uninstallation using network package
	warnings := network.UninstallHelper(func(format string, args ...any) {
		fmt.Printf(format, args...)
	})
	for _, w := range warnings {
		fmt.Printf("Warning: %v\n", w)
	}

	fmt.Println("")
	fmt.Println("Network helper uninstalled successfully.")
	return nil
}

// runNetworkHelperStatus shows the current status.
func runNetworkHelperStatus(cmd *cobra.Command, args []string) error {
	printNetworkHelperStatus()
	return nil
}


// printNetworkHelperStatus prints the current network helper status.
func printNetworkHelperStatus() {
	fmt.Println("Network Helper Status")
	fmt.Println("=====================")
	fmt.Println("")

	printLaunchDaemonStatus()
	printAnchorDirectoryStatus()
	printRuleFiles()
	printActivePfRules()

	fmt.Println("")

	// Show helpful commands
	if !network.IsLaunchDaemonLoaded() && !network.FileExists(network.LaunchDaemonPath) {
		fmt.Println("Run 'alca network-helper install' to install the network helper.")
	}
}

func printLaunchDaemonStatus() {
	fmt.Print("LaunchDaemon: ")
	if network.IsLaunchDaemonLoaded() {
		fmt.Println("Loaded")
	} else if network.FileExists(network.LaunchDaemonPath) {
		fmt.Println("Installed but not loaded")
	} else {
		fmt.Println("Not installed")
	}
}

func printAnchorDirectoryStatus() {
	fmt.Print("Anchor directory: ")
	if network.FileExists(network.PfAnchorDir) {
		fmt.Printf("%s (exists)\n", network.PfAnchorDir)
	} else {
		fmt.Println("Not created")
	}
}

func printRuleFiles() {
	fmt.Println("")
	fmt.Println("Rule files:")
	if network.FileExists(network.PfAnchorDir) {
		entries, err := os.ReadDir(network.PfAnchorDir)
		if err != nil {
			fmt.Printf("  Error reading directory: %v\n", err)
		} else if len(entries) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, entry := range entries {
				if !entry.IsDir() {
					fmt.Printf("  - %s\n", entry.Name())
				}
			}
		}
	} else {
		fmt.Println("  (directory not created)")
	}
}

func printActivePfRules() {
	fmt.Println("")
	fmt.Println("Active pf rules:")

	// LaunchDaemon syncs file contents to kernel, so reading files is equivalent
	// to querying pfctl. This avoids sudo requirement for status command.
	if !network.IsLaunchDaemonLoaded() {
		fmt.Println("  (LaunchDaemon not loaded - rules may be stale)")
	}

	if !network.FileExists(network.PfAnchorDir) {
		fmt.Println("  (none)")
		return
	}

	entries, err := os.ReadDir(network.PfAnchorDir)
	if err != nil {
		fmt.Printf("  Error reading directory: %v\n", err)
		return
	}

	hasRules := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(network.PfAnchorDir, entry.Name()))
		if err != nil {
			continue
		}
		if len(content) > 0 {
			hasRules = true
			fmt.Printf("  [%s]\n", entry.Name())
			for _, line := range splitLines(string(content)) {
				if line != "" {
					fmt.Printf("    %s\n", line)
				}
			}
		}
	}

	if !hasRules {
		fmt.Println("  (none)")
	}
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
