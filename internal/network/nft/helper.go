//go:build linux

package nft

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// nftHelper implements shared.NetworkHelper for Linux using nftables.
type nftHelper struct{}

// Compile-time interface assertion.
var _ shared.NetworkHelper = (*nftHelper)(nil)

func (h *nftHelper) Setup(env *shared.NetworkEnv, projectDir string, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	// nftables rules are managed per-container via ApplyRules, not per-project
	// This is a no-op for Linux - the Firewall interface handles container rules
	return &shared.PostCommitAction{}, nil
}

func (h *nftHelper) Teardown(env *shared.NetworkEnv, projectDir string) error {
	// nftables cleanup is handled per-container via Cleanup
	return nil
}

func (h *nftHelper) HelperStatus(env *shared.NetworkEnv) shared.HelperStatus {
	fs := env.Fs

	// Check if directory exists
	dirExists, _ := afero.DirExists(fs, alcatrazNftDir)
	if !dirExists {
		return shared.HelperStatus{Installed: false, NeedsUpdate: false}
	}

	// Check if include line exists in nftables.conf
	hasInclude := h.hasIncludeLine(fs)

	return shared.HelperStatus{
		Installed:   hasInclude,
		NeedsUpdate: false,
	}
}

func (h *nftHelper) DetailedStatus(env *shared.NetworkEnv) shared.DetailedStatusInfo {
	fs := env.Fs

	info := shared.DetailedStatusInfo{
		DaemonLoaded: h.hasIncludeLine(fs), // Check if include line exists
	}

	// List rule files in alcatraz directory
	files, err := afero.ReadDir(fs, alcatrazNftDir)
	if err != nil {
		return info
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".nft") {
			continue
		}
		path := filepath.Join(alcatrazNftDir, f.Name())
		content, err := afero.ReadFile(fs, path)
		if err != nil {
			continue
		}
		info.RuleFiles = append(info.RuleFiles, shared.RuleFileInfo{
			Name:    f.Name(),
			Content: string(content),
		})
	}

	return info
}

func (h *nftHelper) InstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	fs := env.Fs

	// 1. Create directory
	progress("Creating nftables directory %s...\n", alcatrazNftDir)
	if err := fs.MkdirAll(alcatrazNftDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", alcatrazNftDir, err)
	}

	// 2. Add include line to nftables.conf
	if !h.hasIncludeLine(fs) {
		progress("Adding include line to %s...\n", nftablesConfPath)
		if err := h.addIncludeLine(fs); err != nil {
			return nil, fmt.Errorf("failed to add include line: %w", err)
		}
	}

	// 3. Return post-commit action to reload nftables
	return &shared.PostCommitAction{
		Run: func(progress shared.ProgressFunc) error {
			progress = shared.SafeProgress(progress)
			cmd := env.Cmd

			// Enable nftables service
			progress("Enabling nftables.service...\n")
			_, _ = cmd.RunQuiet("systemctl", "enable", "nftables.service")

			// Reload nftables configuration
			progress("Reloading nftables configuration...\n")
			output, err := cmd.RunQuiet("nft", "-f", nftablesConfPath)
			if err != nil {
				return fmt.Errorf("failed to reload nftables: %w: %s", err, strings.TrimSpace(string(output)))
			}
			return nil
		},
	}, nil
}

func (h *nftHelper) UninstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	fs := env.Fs

	// 1. Remove all files in alcatraz directory
	progress("Removing rule files from %s...\n", alcatrazNftDir)
	files, _ := afero.ReadDir(fs, alcatrazNftDir)
	for _, f := range files {
		if !f.IsDir() {
			_ = fs.Remove(filepath.Join(alcatrazNftDir, f.Name()))
		}
	}

	// 2. Remove include line from nftables.conf
	progress("Removing include line from %s...\n", nftablesConfPath)
	if err := h.removeIncludeLine(fs); err != nil {
		return nil, fmt.Errorf("failed to remove include line: %w", err)
	}

	// 3. Return post-commit action to delete tables and remove directory
	return &shared.PostCommitAction{
		Run: func(progress shared.ProgressFunc) error {
			progress = shared.SafeProgress(progress)

			cmd := env.Cmd

			// Delete all alca-* tables
			progress("Deleting alcatraz nftables tables...\n")
			output, err := cmd.RunQuiet("nft", "list", "tables")
			if err == nil {
				for _, line := range strings.Split(string(output), "\n") {
					// Line format: "table inet alca-abc123"
					parts := strings.Fields(line)
					if len(parts) >= 3 && strings.HasPrefix(parts[2], "alca-") {
						_, _ = cmd.RunQuiet("nft", "delete", "table", parts[1], parts[2])
					}
				}
			}

			// Remove directory
			progress("Removing directory %s...\n", alcatrazNftDir)
			_ = fs.RemoveAll(alcatrazNftDir)

			return nil
		},
	}, nil
}
