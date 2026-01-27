package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	launchDaemonLabel = "com.alcatraz.pf-watcher"
	launchDaemonPath  = "/Library/LaunchDaemons/com.alcatraz.pf-watcher.plist"
	pfAnchorDir       = "/etc/pf.anchors/alcatraz"
	pfAnchorName      = "alcatraz"
)

var launchDaemonPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.alcatraz.pf-watcher</string>
    <key>WatchPaths</key>
    <array>
        <string>/etc/pf.anchors/alcatraz</string>
    </array>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>cat /etc/pf.anchors/alcatraz/* 2>/dev/null | pfctl -a alcatraz -f -</string>
    </array>
</dict>
</plist>
`

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
	if isLaunchDaemonLoaded() {
		fmt.Println("Network helper is already installed and loaded.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will install a LaunchDaemon to manage pf firewall rules.")
	fmt.Println("")
	fmt.Println("The following changes will be made:")
	fmt.Printf("  - Create directory: %s\n", pfAnchorDir)
	fmt.Printf("  - Install plist: %s\n", launchDaemonPath)
	fmt.Println("  - Load LaunchDaemon via launchctl")
	fmt.Println("")
	if !promptConfirm("Continue?") {
		fmt.Println("Installation cancelled.")
		return nil
	}

	fmt.Println("")

	if err := InstallNetworkHelper(); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Network helper installed successfully.")
	return nil
}

// InstallNetworkHelper performs the actual installation of the network helper.
// Extracted from runNetworkHelperInstall for reuse by other commands (e.g., alca up).
// See AGD-023 for lifecycle details.
func InstallNetworkHelper() error {
	// Create anchor directory
	fmt.Printf("Creating %s...\n", pfAnchorDir)
	if err := ensurePath(pfAnchorDir); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}
	if err := ensureChmod(pfAnchorDir, 0755, false); err != nil {
		return fmt.Errorf("failed to set anchor directory permissions: %w", err)
	}

	// Write plist file (or update if content differs for migration)
	existingPlist, _ := os.ReadFile(launchDaemonPath)
	if string(existingPlist) != launchDaemonPlist {
		fmt.Printf("Installing %s...\n", launchDaemonPath)
		tmpFile, err := os.CreateTemp("", "com.alcatraz.pf-watcher.*.plist")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(launchDaemonPlist); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write plist: %w", err)
		}
		tmpFile.Close()

		if err := runSudo("cp", tmpFile.Name(), launchDaemonPath); err != nil {
			return fmt.Errorf("failed to install plist: %w", err)
		}
	} else {
		fmt.Printf("Plist %s already up to date\n", launchDaemonPath)
	}

	// Set correct ownership and permissions
	if err := runSudo("chown", "root:wheel", launchDaemonPath); err != nil {
		return fmt.Errorf("failed to set plist ownership: %w", err)
	}
	if err := runSudo("chmod", "644", launchDaemonPath); err != nil {
		return fmt.Errorf("failed to set plist permissions: %w", err)
	}

	// Add nat-anchor to /etc/pf.conf if not present
	fmt.Println("Configuring pf anchor...")
	if err := ensurePfAnchor(); err != nil {
		return fmt.Errorf("failed to configure pf anchor: %w", err)
	}

	// Load LaunchDaemon using bootstrap (macOS 10.10+)
	// If already loaded, bootout first to reload with updated plist
	fmt.Println("Loading LaunchDaemon...")
	if isLaunchDaemonLoaded() {
		// Bootout first to allow re-bootstrap
		_ = runSudo("launchctl", "bootout", "system/"+launchDaemonLabel)
	}
	if err := runSudo("launchctl", "bootstrap", "system", launchDaemonPath); err != nil {
		// Fallback to legacy load for older systems
		if err := runSudo("launchctl", "load", launchDaemonPath); err != nil {
			return fmt.Errorf("failed to load LaunchDaemon: %w", err)
		}
	}

	// Manually load initial rules - WatchPaths only triggers on changes AFTER daemon load
	fmt.Println("Loading initial pf rules...")
	output, err := runSudoQuiet("sh", "-c", "cat /etc/pf.anchors/alcatraz/* 2>/dev/null | pfctl -a alcatraz -f -")
	if err != nil {
		// Not fatal - rules may not exist yet, will be loaded when created
		if output != "" {
			fmt.Printf("Note: %s\n", output)
		} else {
			fmt.Printf("Note: No initial rules to load (this is normal on first install)\n")
		}
	}

	return nil
}

// ensurePfAnchor adds nat-anchor "alcatraz" to /etc/pf.conf if not present.
// Also handles migration from old 'alcatraz/*' wildcard format.
// IMPORTANT: nat-anchor lines must come BEFORE anchor lines in pf.conf.
func ensurePfAnchor() error {
	pfConfPath := "/etc/pf.conf"
	anchorLine := `nat-anchor "alcatraz"`
	oldAnchorLine := `nat-anchor "alcatraz/*"`

	// Read current pf.conf
	content, err := os.ReadFile(pfConfPath)
	if err != nil {
		return fmt.Errorf("failed to read pf.conf: %w", err)
	}

	contentStr := string(content)

	// Check if new anchor already exists (and no old one to migrate)
	if strings.Contains(contentStr, anchorLine) && !strings.Contains(contentStr, oldAnchorLine) {
		return nil
	}

	lines := strings.Split(contentStr, "\n")
	var newLines []string
	anchorInserted := false
	lastNatAnchorIdx := -1

	// First pass: find last nat-anchor line and remove old wildcard anchor
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "nat-anchor ") {
			if trimmed != oldAnchorLine {
				lastNatAnchorIdx = len(newLines)
			}
			// Skip old wildcard anchor (migration)
			if trimmed == oldAnchorLine {
				continue
			}
		}
		newLines = append(newLines, line)
	}

	// Insert our anchor after the last nat-anchor line
	if lastNatAnchorIdx >= 0 {
		// Insert after last nat-anchor
		result := make([]string, 0, len(newLines)+1)
		result = append(result, newLines[:lastNatAnchorIdx+1]...)
		result = append(result, anchorLine)
		result = append(result, newLines[lastNatAnchorIdx+1:]...)
		newLines = result
		anchorInserted = true
	}

	// If no nat-anchor found, insert before first "anchor" line
	if !anchorInserted {
		result := make([]string, 0, len(newLines)+1)
		for i, line := range newLines {
			trimmed := strings.TrimSpace(line)
			if !anchorInserted && strings.HasPrefix(trimmed, "anchor ") {
				result = append(result, anchorLine)
				anchorInserted = true
			}
			result = append(result, line)
			// If we reach end without finding anchor line, append
			if i == len(newLines)-1 && !anchorInserted {
				if trimmed != "" {
					result = append(result, anchorLine)
				} else {
					// Insert before trailing empty line
					result = append(result[:len(result)-1], anchorLine, "")
				}
				anchorInserted = true
			}
		}
		newLines = result
	}

	newContent := strings.Join(newLines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	// Write to temp file and sudo cp
	tmpFile, err := os.CreateTemp("", "pf.conf-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Backup original
	if err := runSudo("cp", pfConfPath, pfConfPath+".bak"); err != nil {
		return fmt.Errorf("failed to backup pf.conf: %w", err)
	}

	// Copy new content
	if err := runSudo("cp", tmpFile.Name(), pfConfPath); err != nil {
		return fmt.Errorf("failed to update pf.conf: %w", err)
	}

	// Reload pf
	if err := runSudo("pfctl", "-f", pfConfPath); err != nil {
		fmt.Printf("Warning: failed to reload pf: %v\n", err)
	}

	return nil
}

// removePfAnchor removes nat-anchor "alcatraz/*" from /etc/pf.conf.
func removePfAnchor() error {
	pfConfPath := "/etc/pf.conf"
	anchorLine := `nat-anchor "alcatraz"`

	content, err := os.ReadFile(pfConfPath)
	if err != nil {
		return fmt.Errorf("failed to read pf.conf: %w", err)
	}

	if !strings.Contains(string(content), anchorLine) {
		return nil // Already removed
	}

	// Remove anchor line
	lines := strings.Split(string(content), "\n")
	var newLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != anchorLine {
			newLines = append(newLines, line)
		}
	}
	newContent := strings.Join(newLines, "\n")

	tmpFile, err := os.CreateTemp("", "pf.conf-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	if err := runSudo("cp", tmpFile.Name(), pfConfPath); err != nil {
		return fmt.Errorf("failed to update pf.conf: %w", err)
	}

	if err := runSudo("pfctl", "-f", pfConfPath); err != nil {
		fmt.Printf("Warning: failed to reload pf: %v\n", err)
	}

	return nil
}

// WriteSharedRuleWithSudo writes the shared NAT rule file using sudo.
func WriteSharedRuleWithSudo(content string) error {
	sharedPath := filepath.Join(pfAnchorDir, "_shared")

	changed, err := ensureFileContent(sharedPath, content)
	if err != nil {
		return fmt.Errorf("failed to write shared rule: %w", err)
	}
	if changed {
		if err := ensureChmod(sharedPath, 0644, false); err != nil {
			return fmt.Errorf("failed to set shared rule permissions: %w", err)
		}
	}
	return nil
}

// WriteProjectFileWithSudo writes the project-specific rule file using sudo.
func WriteProjectFileWithSudo(projectPath, content string) error {
	filename := strings.ReplaceAll(projectPath, "/", "-")
	projectFilePath := filepath.Join(pfAnchorDir, filename)

	changed, err := ensureFileContent(projectFilePath, content)
	if err != nil {
		return fmt.Errorf("failed to write project file: %w", err)
	}
	if changed {
		if err := ensureChmod(projectFilePath, 0644, false); err != nil {
			return fmt.Errorf("failed to set project file permissions: %w", err)
		}
	}
	return nil
}

// runNetworkHelperUninstall removes the LaunchDaemon and cleans up.
// See AGD-023 for lifecycle details.
func runNetworkHelperUninstall(cmd *cobra.Command, args []string) error {
	// Check if installed
	plistExists := fileExists(launchDaemonPath)
	dirExists := fileExists(pfAnchorDir)

	if !plistExists && !dirExists && !isLaunchDaemonLoaded() {
		fmt.Println("Network helper is not installed.")
		return nil
	}

	// Confirmation prompt
	fmt.Println("This will uninstall the network helper and remove all rules.")
	fmt.Println("")
	fmt.Println("The following changes will be made:")
	if isLaunchDaemonLoaded() {
		fmt.Println("  - Unload LaunchDaemon via launchctl")
	}
	if plistExists {
		fmt.Printf("  - Remove plist: %s\n", launchDaemonPath)
	}
	fmt.Printf("  - Flush all pf rules in anchor: %s\n", pfAnchorName)
	if dirExists {
		fmt.Printf("  - Remove directory: %s\n", pfAnchorDir)
	}
	fmt.Println("")
	if !promptConfirm("Continue?") {
		fmt.Println("Uninstallation cancelled.")
		return nil
	}

	fmt.Println("")

	// Unload LaunchDaemon
	if isLaunchDaemonLoaded() {
		fmt.Println("Unloading LaunchDaemon...")
		if err := runSudo("launchctl", "unload", launchDaemonPath); err != nil {
			fmt.Printf("Warning: failed to unload LaunchDaemon: %v\n", err)
		}
	}

	// Remove plist
	if plistExists {
		fmt.Printf("Removing %s...\n", launchDaemonPath)
		if err := runSudo("rm", "-f", launchDaemonPath); err != nil {
			fmt.Printf("Warning: failed to remove plist: %v\n", err)
		}
	}

	// Flush all alcatraz pf rules
	fmt.Printf("Flushing pf rules in anchor %s...\n", pfAnchorName)
	if err := runSudo("pfctl", "-a", pfAnchorName, "-F", "all"); err != nil {
		fmt.Printf("Warning: failed to flush pf rules: %v\n", err)
	}

	// Remove anchor from pf.conf
	fmt.Println("Removing anchor from pf.conf...")
	if err := removePfAnchor(); err != nil {
		fmt.Printf("Warning: failed to remove anchor from pf.conf: %v\n", err)
	}

	// Remove anchor directory
	if dirExists {
		fmt.Printf("Removing %s...\n", pfAnchorDir)
		if err := runSudo("rm", "-rf", pfAnchorDir); err != nil {
			fmt.Printf("Warning: failed to remove anchor directory: %v\n", err)
		}
	}

	fmt.Println("")
	fmt.Println("Network helper uninstalled successfully.")
	return nil
}

// runNetworkHelperStatus shows the current status.
func runNetworkHelperStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Network Helper Status")
	fmt.Println("=====================")
	fmt.Println("")

	// Check LaunchDaemon status
	fmt.Print("LaunchDaemon: ")
	if isLaunchDaemonLoaded() {
		fmt.Println("Loaded")
	} else if fileExists(launchDaemonPath) {
		fmt.Println("Installed but not loaded")
	} else {
		fmt.Println("Not installed")
	}

	// Check anchor directory
	fmt.Print("Anchor directory: ")
	if fileExists(pfAnchorDir) {
		fmt.Printf("%s (exists)\n", pfAnchorDir)
	} else {
		fmt.Println("Not created")
	}

	// List rule files
	fmt.Println("")
	fmt.Println("Rule files:")
	if fileExists(pfAnchorDir) {
		entries, err := os.ReadDir(pfAnchorDir)
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

	// Show active pf rules
	fmt.Println("")
	fmt.Println("Active pf rules:")
	// Note: If sudo requires password input, CombinedOutput() will fail and we show "(none)".
	// This is a known limitation - status command may need to be run with sudo separately.
	output, err := exec.Command("sudo", "pfctl", "-a", pfAnchorName, "-s", "nat").CombinedOutput()
	if err != nil {
		// pfctl returns error if no rules exist (normal), or if sudo needs password
		fmt.Println("  (none)")
	} else {
		rules := strings.TrimSpace(string(output))
		if rules == "" {
			fmt.Println("  (none)")
		} else {
			for _, line := range strings.Split(rules, "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	fmt.Println("")

	// Show helpful commands
	if !isLaunchDaemonLoaded() && !fileExists(launchDaemonPath) {
		fmt.Println("Run 'alca network-helper install' to install the network helper.")
	}

	return nil
}

// isLaunchDaemonLoaded checks if the LaunchDaemon is loaded in the system domain.
// Uses 'launchctl print' to check system domain (not user domain).
func isLaunchDaemonLoaded() bool {
	cmd := exec.Command("launchctl", "print", "system/"+launchDaemonLabel)
	err := cmd.Run()
	return err == nil
}

// fileExists checks if a file or directory exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// promptConfirm prompts the user for confirmation.
func promptConfirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// IsNetworkHelperNeedsUpdate checks if plist or pf.conf anchor needs update.
func IsNetworkHelperNeedsUpdate() bool {
	// Check plist content
	existingPlist, err := os.ReadFile(launchDaemonPath)
	if err == nil && string(existingPlist) != launchDaemonPlist {
		return true
	}

	// Check pf.conf anchor (old wildcard vs new single)
	pfConf, err := os.ReadFile("/etc/pf.conf")
	if err == nil {
		content := string(pfConf)
		hasNew := strings.Contains(content, `nat-anchor "alcatraz"`)
		hasOld := strings.Contains(content, `nat-anchor "alcatraz/*"`)
		if hasOld || !hasNew {
			return true
		}
	}

	return false
}

// ensurePath checks if directory exists, runs sudo mkdir -p if needed.
func ensurePath(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := runSudo("mkdir", "-p", path); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return nil
}

// ensureChmod checks permissions and runs sudo chmod if needed.
// If recursive=true, checks all files and uses chmod -R if any differ.
func ensureChmod(path string, mode os.FileMode, recursive bool) error {
	if recursive {
		needsChange := false
		filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.Mode().Perm() != mode {
				needsChange = true
			}
			return nil
		})
		if needsChange {
			if err := runSudo("chmod", "-R", fmt.Sprintf("%o", mode), path); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	} else {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			if err := runSudo("chmod", fmt.Sprintf("%o", mode), path); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	}
	return nil
}

// ensureFileContent checks content and runs sudo cp if needed.
// Returns (false, nil) if content unchanged, (true, nil) after successful write.
func ensureFileContent(path string, content string) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return false, nil
	}

	tmpFile, err := os.CreateTemp("", "alcatraz-*.txt")
	if err != nil {
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return false, fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	if err := runSudo("cp", tmpFile.Name(), path); err != nil {
		return false, fmt.Errorf("failed to copy file: %w", err)
	}
	return true, nil
}

// runSudo runs a command with sudo.
func runSudo(name string, args ...string) error {
	cmdArgs := append([]string{name}, args...)
	cmd := exec.Command("sudo", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runSudoQuiet runs a command with sudo, suppressing output on success.
// On failure, returns the captured output along with the error.
func runSudoQuiet(name string, args ...string) (output string, err error) {
	cmdArgs := append([]string{name}, args...)
	cmd := exec.Command("sudo", cmdArgs...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// GetProjectAnchorName returns the anchor file name for a project path.
// It encodes the path by replacing "/" with "-".
func GetProjectAnchorName(projectPath string) string {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		absPath = projectPath
	}
	return strings.ReplaceAll(absPath, "/", "-")
}

// GetPfAnchorDir returns the pf anchor directory path.
func GetPfAnchorDir() string {
	return pfAnchorDir
}

// IsNetworkHelperInstalled checks if the network helper is installed and loaded.
func IsNetworkHelperInstalled() bool {
	return isLaunchDaemonLoaded() && fileExists(pfAnchorDir)
}
