// Package network provides LaunchDaemon management for macOS network helper.
// See AGD-023 for design decisions.
package network

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bolasblack/alcatraz/internal/sudo"
	"github.com/bolasblack/alcatraz/internal/util"
	"github.com/spf13/afero"
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

// InstallHelper stages network helper files using TransactFs from context.
// IMPORTANT: This only stages files. Caller must:
// 1. Call this function to stage files
// 2. Commit the TransactFs with sudo
// 3. Call LoadLaunchDaemon() after commit to load the daemon
// See AGD-023 for lifecycle details.
func InstallHelper(ctx context.Context, progress ProgressFunc) error {
	if progress == nil {
		progress = func(format string, args ...any) {} // no-op
	}

	if err := createAnchorDirectory(ctx, progress); err != nil {
		return err
	}

	if err := installPlistFile(ctx, progress); err != nil {
		return err
	}

	progress("Configuring pf anchor...\n")
	if err := EnsurePfAnchor(ctx); err != nil {
		return fmt.Errorf("failed to configure pf anchor: %w", err)
	}

	// NOTE: loadLaunchDaemon and loadInitialPfRules must be called AFTER
	// TransactFs commit, since they expect files to exist on disk.
	// Caller should call LoadLaunchDaemon() after committing.
	return nil
}

// LoadLaunchDaemon loads the LaunchDaemon after files have been committed.
// Call this AFTER committing TransactFs changes to disk.
func LoadLaunchDaemon(progress ProgressFunc) error {
	if progress == nil {
		progress = func(format string, args ...any) {}
	}
	if err := loadLaunchDaemon(progress); err != nil {
		return err
	}
	return loadInitialPfRules(progress)
}

// UninstallHelper unloads daemon and stages file deletions using TransactFs from context.
// IMPORTANT: This unloads the daemon first, then stages file deletions. Caller must:
// 1. Call this function (unloads daemon, stages file deletions)
// 2. Commit the TransactFs with sudo
// 3. Call FlushPfRulesAfterUninstall() after commit if needed
// Returns errors as warnings - caller decides whether to fail.
func UninstallHelper(ctx context.Context, progress ProgressFunc) (warnings []error) {
	if progress == nil {
		progress = func(format string, args ...any) {}
	}

	// Unload daemon FIRST (before staging file deletions)
	if err := unloadLaunchDaemon(progress); err != nil {
		warnings = append(warnings, err)
	}

	// Stage file deletions
	if err := removePlistFile(ctx, progress); err != nil {
		warnings = append(warnings, err)
	}

	progress("Removing anchor from pf.conf...\n")
	if err := RemovePfAnchor(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("failed to remove anchor from pf.conf: %w", err))
	}

	if err := removeAnchorDirectory(ctx, progress); err != nil {
		warnings = append(warnings, err)
	}

	// NOTE: FlushPfRules should be called AFTER commit.
	// Caller should call FlushPfRulesAfterUninstall() after committing.
	return warnings
}

// FlushPfRulesAfterUninstall flushes pf rules after uninstall files have been committed.
// Call this AFTER committing TransactFs changes to disk.
func FlushPfRulesAfterUninstall(progress ProgressFunc) error {
	return FlushPfRules(progress)
}

// IsHelperInstalled checks if the network helper is installed using TransactFs from context.
func IsHelperInstalled(ctx context.Context) bool {
	return IsLaunchDaemonLoaded() && FileExists(ctx, PfAnchorDir)
}

// IsHelperNeedsUpdate checks if plist or pf.conf anchor needs update using TransactFs from context.
func IsHelperNeedsUpdate(ctx context.Context) bool {
	fs := util.MustGetFs(ctx)

	// Check plist content
	existingPlist, err := afero.ReadFile(fs, LaunchDaemonPath)
	if err == nil && string(existingPlist) != launchDaemonPlist {
		return true
	}

	// Check pf.conf anchor (old wildcard vs new single)
	pfConf, err := afero.ReadFile(fs, PfConfPath)
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
func createAnchorDirectory(ctx context.Context, progress ProgressFunc) error {
	fs := util.MustGetFs(ctx)
	progress("Creating %s...\n", PfAnchorDir)
	if err := fs.MkdirAll(PfAnchorDir, 0755); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}
	return nil
}

// installPlistFile installs or updates the LaunchDaemon plist.
func installPlistFile(ctx context.Context, progress ProgressFunc) error {
	fs := util.MustGetFs(ctx)

	// Check if content already matches
	existingContent, err := afero.ReadFile(fs, LaunchDaemonPath)
	if err == nil && string(existingContent) == launchDaemonPlist {
		progress("Plist %s already up to date\n", LaunchDaemonPath)
		return nil
	}

	progress("Installing %s...\n", LaunchDaemonPath)

	// Write plist file
	if err := afero.WriteFile(fs, LaunchDaemonPath, []byte(launchDaemonPlist), 0644); err != nil {
		return fmt.Errorf("failed to install plist: %w", err)
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
func removePlistFile(ctx context.Context, progress ProgressFunc) error {
	fs := util.MustGetFs(ctx)
	if !FileExists(ctx, LaunchDaemonPath) {
		return nil
	}
	progress("Removing %s...\n", LaunchDaemonPath)
	if err := fs.Remove(LaunchDaemonPath); err != nil && !os.IsNotExist(err) {
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

// removeAnchorDirectory removes the pf anchor directory using TransactFs from context.
func removeAnchorDirectory(ctx context.Context, progress ProgressFunc) error {
	fs := util.MustGetFs(ctx)
	if !FileExists(ctx, PfAnchorDir) {
		return nil
	}
	progress("Removing %s...\n", PfAnchorDir)
	if err := fs.RemoveAll(PfAnchorDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove anchor directory: %w", err)
	}
	return nil
}
