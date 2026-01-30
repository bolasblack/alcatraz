// Package network provides LaunchDaemon management for macOS network helper.
// See AGD-023 for design decisions.
package network

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// LaunchDaemon constants.
const (
	launchDaemonLabel = "com.alcatraz.pf-watcher"
	launchDaemonPath  = "/Library/LaunchDaemons/com.alcatraz.pf-watcher.plist"
)

// buildLaunchDaemonPlist generates the plist content from constants.
func buildLaunchDaemonPlist() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>WatchPaths</key>
    <array>
        <string>%s</string>
    </array>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>cat %s/* 2>/dev/null | pfctl -a %s -f -</string>
    </array>
</dict>
</plist>
`, launchDaemonLabel, pfAnchorDir, pfAnchorDir, pfAnchorName)
}

// launchDaemonPlist is the plist content for the LaunchDaemon.
var launchDaemonPlist = buildLaunchDaemonPlist()

// installHelper stages network helper files.
// IMPORTANT: This only stages files. Caller must:
// 1. Call this function to stage files
// 2. Commit the TransactFs with sudo
// 3. Call activateLaunchDaemon() after commit to load the daemon
// See AGD-023 for lifecycle details.
func (p *pfHelper) installHelper(env *util.Env, progress ProgressFunc) error {
	progress = safeProgress(progress)

	if err := p.createAnchorDirectory(env, progress); err != nil {
		return err
	}

	if err := p.installPlistFile(env, progress); err != nil {
		return err
	}

	progress("Configuring pf anchor...\n")
	if err := p.ensurePfAnchor(env); err != nil {
		return fmt.Errorf("failed to configure pf anchor: %w", err)
	}

	// NOTE: loadLaunchDaemon and loadInitialPfRules must be called AFTER
	// TransactFs commit, since they expect files to exist on disk.
	// Caller should call activateLaunchDaemon() after committing.
	return nil
}

// activateLaunchDaemon loads the LaunchDaemon after files have been committed.
// Call this AFTER committing TransactFs changes to disk.
func (p *pfHelper) activateLaunchDaemon(env *util.Env, progress ProgressFunc) error {
	progress = safeProgress(progress)
	if err := p.loadLaunchDaemon(env, progress); err != nil {
		return err
	}
	return p.loadInitialPfRules(env, progress)
}

// uninstallHelper unloads daemon and stages file deletions.
// IMPORTANT: This unloads the daemon first, then stages file deletions. Caller must:
// 1. Call this function (unloads daemon, stages file deletions)
// 2. Commit the TransactFs with sudo
// 3. Call flushPfRulesAfterUninstall() after commit if needed
// Returns errors as warnings - caller decides whether to fail.
func (p *pfHelper) uninstallHelper(env *util.Env, progress ProgressFunc) (warnings []error) {
	progress = safeProgress(progress)

	// Unload daemon FIRST (before staging file deletions)
	if err := p.unloadLaunchDaemon(env, progress); err != nil {
		warnings = append(warnings, err)
	}

	// Stage file deletions
	if err := p.removePlistFile(env, progress); err != nil {
		warnings = append(warnings, err)
	}

	progress("Removing anchor from pf.conf...\n")
	if err := p.removePfAnchor(env); err != nil {
		warnings = append(warnings, fmt.Errorf("failed to remove anchor from pf.conf: %w", err))
	}

	if err := p.removeAnchorDirectory(env, progress); err != nil {
		warnings = append(warnings, err)
	}

	// NOTE: FlushPfRules should be called AFTER commit.
	// Caller should call flushPfRulesAfterUninstall() after committing.
	return warnings
}

// flushPfRulesAfterUninstall flushes pf rules after uninstall files have been committed.
// Call this AFTER committing TransactFs changes to disk.
func (p *pfHelper) flushPfRulesAfterUninstall(env *util.Env, progress ProgressFunc) error {
	return p.flushPfRules(env, progress)
}

// isHelperInstalled checks if the network helper is installed.
func (p *pfHelper) isHelperInstalled(env *util.Env) bool {
	return p.isLaunchDaemonLoaded(env) && fileExists(env, pfAnchorDir)
}

// isHelperNeedsUpdate checks if plist or pf.conf anchor needs update.
func (p *pfHelper) isHelperNeedsUpdate(env *util.Env) bool {
	// Check plist content
	existingPlist, err := afero.ReadFile(env.Fs, launchDaemonPath)
	if err == nil && string(existingPlist) != launchDaemonPlist {
		return true
	}

	// Check pf.conf anchor (old wildcard vs new single)
	pfConf, err := afero.ReadFile(env.Fs, pfConfPath)
	if err == nil {
		content := string(pfConf)
		hasNew := strings.Contains(content, pfAnchorLine)
		hasOld := strings.Contains(content, pfOldAnchorLine)
		if hasOld || !hasNew {
			return true
		}
	}

	return false
}

// createAnchorDirectory creates the pf anchor directory with proper permissions.
func (p *pfHelper) createAnchorDirectory(env *util.Env, progress ProgressFunc) error {
	progress("Creating %s...\n", pfAnchorDir)
	if err := env.Fs.MkdirAll(pfAnchorDir, 0755); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}
	return nil
}

// installPlistFile installs or updates the LaunchDaemon plist.
func (p *pfHelper) installPlistFile(env *util.Env, progress ProgressFunc) error {
	// Check if content already matches
	existingContent, err := afero.ReadFile(env.Fs, launchDaemonPath)
	if err == nil && string(existingContent) == launchDaemonPlist {
		progress("Plist %s already up to date\n", launchDaemonPath)
		return nil
	}

	progress("Installing %s...\n", launchDaemonPath)

	// Write plist file
	if err := afero.WriteFile(env.Fs, launchDaemonPath, []byte(launchDaemonPlist), 0644); err != nil {
		return fmt.Errorf("failed to install plist: %w", err)
	}

	return nil
}

// loadLaunchDaemon loads the LaunchDaemon using launchctl.
func (p *pfHelper) loadLaunchDaemon(env *util.Env, progress ProgressFunc) error {
	progress("Loading LaunchDaemon...\n")
	if p.isLaunchDaemonLoaded(env) {
		// Bootout first to allow re-bootstrap
		_ = env.Cmd.SudoRun("launchctl", "bootout", "system/"+launchDaemonLabel)
	}
	if err := env.Cmd.SudoRun("launchctl", "bootstrap", "system", launchDaemonPath); err != nil {
		// Fallback to legacy load for older systems
		if err := env.Cmd.SudoRun("launchctl", "load", launchDaemonPath); err != nil {
			return fmt.Errorf("failed to load LaunchDaemon: %w", err)
		}
	}
	return nil
}

// loadInitialPfRules loads initial pf rules after daemon installation.
func (p *pfHelper) loadInitialPfRules(env *util.Env, progress ProgressFunc) error {
	progress("Loading initial pf rules...\n")
	cmd := fmt.Sprintf("cat %s/* 2>/dev/null | pfctl -a %s -f -", pfAnchorDir, pfAnchorName)
	output, err := env.Cmd.SudoRunQuiet("sh", "-c", cmd)
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

// isLaunchDaemonLoaded checks if the LaunchDaemon is loaded in the system domain.
// Uses 'launchctl print' to check system domain (not user domain).
func (p *pfHelper) isLaunchDaemonLoaded(env *util.Env) bool {
	_, err := env.Cmd.RunQuiet("launchctl", "print", "system/"+launchDaemonLabel)
	return err == nil
}

// unloadLaunchDaemon unloads the LaunchDaemon.
func (p *pfHelper) unloadLaunchDaemon(env *util.Env, progress ProgressFunc) error {
	if !p.isLaunchDaemonLoaded(env) {
		return nil
	}
	progress("Unloading LaunchDaemon...\n")
	if err := env.Cmd.SudoRun("launchctl", "unload", launchDaemonPath); err != nil {
		return fmt.Errorf("failed to unload LaunchDaemon: %w", err)
	}
	return nil
}

// removePlistFile removes the LaunchDaemon plist file.
func (p *pfHelper) removePlistFile(env *util.Env, progress ProgressFunc) error {
	if !fileExists(env, launchDaemonPath) {
		return nil
	}
	progress("Removing %s...\n", launchDaemonPath)
	if err := env.Fs.Remove(launchDaemonPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}
	return nil
}

// flushPfRules flushes all pf rules in the anchor.
func (p *pfHelper) flushPfRules(env *util.Env, progress ProgressFunc) error {
	progress = safeProgress(progress)
	progress("Flushing pf rules in anchor %s...\n", pfAnchorName)
	if err := env.Cmd.SudoRun("pfctl", "-a", pfAnchorName, "-F", "all"); err != nil {
		return fmt.Errorf("failed to flush pf rules: %w", err)
	}
	return nil
}

// removeAnchorDirectory removes the pf anchor directory.
func (p *pfHelper) removeAnchorDirectory(env *util.Env, progress ProgressFunc) error {
	if !fileExists(env, pfAnchorDir) {
		return nil
	}
	progress("Removing %s...\n", pfAnchorDir)
	if err := env.Fs.RemoveAll(pfAnchorDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove anchor directory: %w", err)
	}
	return nil
}
