// Package network provides LaunchDaemon management for macOS network helper.
// See AGD-023 for design decisions.
package network

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bolasblack/alcatraz/internal/sudo"
)

// LaunchDaemon constants.
const (
	LaunchDaemonLabel = "com.alcatraz.pf-watcher"
	LaunchDaemonPath  = "/Library/LaunchDaemons/com.alcatraz.pf-watcher.plist"
)

// launchDaemonPlist is the plist content for the LaunchDaemon.
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

// ProgressFunc is a callback for progress messages during installation.
type ProgressFunc func(format string, args ...any)

// InstallHelper installs the network helper LaunchDaemon.
// The progress callback is called with status messages during installation.
// See AGD-023 for lifecycle details.
func InstallHelper(progress ProgressFunc) error {
	if progress == nil {
		progress = func(format string, args ...any) {} // no-op
	}

	if err := createAnchorDirectory(progress); err != nil {
		return err
	}

	if err := installPlistFile(progress); err != nil {
		return err
	}

	progress("Configuring pf anchor...\n")
	if err := EnsurePfAnchor(); err != nil {
		return fmt.Errorf("failed to configure pf anchor: %w", err)
	}

	if err := loadLaunchDaemon(progress); err != nil {
		return err
	}

	return loadInitialPfRules(progress)
}

// UninstallHelper uninstalls the network helper and cleans up.
// Returns errors as warnings - caller decides whether to fail.
func UninstallHelper(progress ProgressFunc) (warnings []error) {
	if progress == nil {
		progress = func(format string, args ...any) {}
	}

	if err := unloadLaunchDaemon(progress); err != nil {
		warnings = append(warnings, err)
	}

	if err := removePlistFile(progress); err != nil {
		warnings = append(warnings, err)
	}

	if err := FlushPfRules(progress); err != nil {
		warnings = append(warnings, err)
	}

	progress("Removing anchor from pf.conf...\n")
	if err := RemovePfAnchor(); err != nil {
		warnings = append(warnings, fmt.Errorf("failed to remove anchor from pf.conf: %w", err))
	}

	if err := removeAnchorDirectory(progress); err != nil {
		warnings = append(warnings, err)
	}

	return warnings
}

// IsHelperInstalled checks if the network helper is installed and loaded.
func IsHelperInstalled() bool {
	return IsLaunchDaemonLoaded() && FileExists(PfAnchorDir)
}

// IsHelperNeedsUpdate checks if plist or pf.conf anchor needs update.
func IsHelperNeedsUpdate() bool {
	// Check plist content
	existingPlist, err := os.ReadFile(LaunchDaemonPath)
	if err == nil && string(existingPlist) != launchDaemonPlist {
		return true
	}

	// Check pf.conf anchor (old wildcard vs new single)
	pfConf, err := os.ReadFile(PfConfPath)
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


// createAnchorDirectory creates the pf anchor directory with proper permissions.
func createAnchorDirectory(progress ProgressFunc) error {
	progress("Creating %s...\n", PfAnchorDir)
	if err := sudo.EnsurePath(PfAnchorDir); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}
	if err := sudo.EnsureChmod(PfAnchorDir, 0755, false); err != nil {
		return fmt.Errorf("failed to set anchor directory permissions: %w", err)
	}
	return nil
}

// installPlistFile installs or updates the LaunchDaemon plist.
func installPlistFile(progress ProgressFunc) error {
	changed, err := sudo.EnsureFileContent(LaunchDaemonPath, launchDaemonPlist)
	if err != nil {
		return fmt.Errorf("failed to install plist: %w", err)
	}
	if !changed {
		progress("Plist %s already up to date\n", LaunchDaemonPath)
		return nil
	}

	progress("Installing %s...\n", LaunchDaemonPath)

	// Set correct ownership and permissions
	if err := sudo.Run("chown", "root:wheel", LaunchDaemonPath); err != nil {
		return fmt.Errorf("failed to set plist ownership: %w", err)
	}
	if err := sudo.EnsureChmod(LaunchDaemonPath, 0644, false); err != nil {
		return fmt.Errorf("failed to set plist permissions: %w", err)
	}

	return nil
}

// loadLaunchDaemon loads the LaunchDaemon using launchctl.
func loadLaunchDaemon(progress ProgressFunc) error {
	progress("Loading LaunchDaemon...\n")
	if IsLaunchDaemonLoaded() {
		// Bootout first to allow re-bootstrap
		_ = sudo.Run("launchctl", "bootout", "system/"+LaunchDaemonLabel)
	}
	if err := sudo.Run("launchctl", "bootstrap", "system", LaunchDaemonPath); err != nil {
		// Fallback to legacy load for older systems
		if err := sudo.Run("launchctl", "load", LaunchDaemonPath); err != nil {
			return fmt.Errorf("failed to load LaunchDaemon: %w", err)
		}
	}
	return nil
}

// loadInitialPfRules loads initial pf rules after daemon installation.
func loadInitialPfRules(progress ProgressFunc) error {
	progress("Loading initial pf rules...\n")
	output, err := sudo.RunQuiet("sh", "-c", "cat /etc/pf.anchors/alcatraz/* 2>/dev/null | pfctl -a alcatraz -f -")
	if err != nil {
		// Not fatal - rules may not exist yet, will be loaded when created
		if output != "" {
			progress("Note: %s\n", output)
		} else {
			progress("Note: No initial rules to load (this is normal on first install)\n")
		}
	}
	return nil
}

// IsLaunchDaemonLoaded checks if the LaunchDaemon is loaded in the system domain.
// Uses 'launchctl print' to check system domain (not user domain).
func IsLaunchDaemonLoaded() bool {
	cmd := exec.Command("launchctl", "print", "system/"+LaunchDaemonLabel)
	err := cmd.Run()
	return err == nil
}

// unloadLaunchDaemon unloads the LaunchDaemon.
func unloadLaunchDaemon(progress ProgressFunc) error {
	if !IsLaunchDaemonLoaded() {
		return nil
	}
	progress("Unloading LaunchDaemon...\n")
	if err := sudo.Run("launchctl", "unload", LaunchDaemonPath); err != nil {
		return fmt.Errorf("failed to unload LaunchDaemon: %w", err)
	}
	return nil
}

// removePlistFile removes the LaunchDaemon plist file.
func removePlistFile(progress ProgressFunc) error {
	if !FileExists(LaunchDaemonPath) {
		return nil
	}
	progress("Removing %s...\n", LaunchDaemonPath)
	if err := sudo.Run("rm", "-f", LaunchDaemonPath); err != nil {
		return fmt.Errorf("failed to remove plist: %w", err)
	}
	return nil
}

// FlushPfRules flushes all pf rules in the alcatraz anchor.
func FlushPfRules(progress ProgressFunc) error {
	if progress != nil {
		progress("Flushing pf rules in anchor %s...\n", PfAnchorName)
	}
	if err := sudo.Run("pfctl", "-a", PfAnchorName, "-F", "all"); err != nil {
		return fmt.Errorf("failed to flush pf rules: %w", err)
	}
	return nil
}

// removeAnchorDirectory removes the pf anchor directory.
func removeAnchorDirectory(progress ProgressFunc) error {
	if !FileExists(PfAnchorDir) {
		return nil
	}
	progress("Removing %s...\n", PfAnchorDir)
	if err := sudo.Run("rm", "-rf", PfAnchorDir); err != nil {
		return fmt.Errorf("failed to remove anchor directory: %w", err)
	}
	return nil
}
