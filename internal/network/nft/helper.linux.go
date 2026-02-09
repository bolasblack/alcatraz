package nft

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// NewLinuxHelper creates a NetworkHelper for Linux.
func NewLinuxHelper(cfg config.Network, _ runtime.RuntimePlatform) shared.NetworkHelper {
	if !hasLANAccess(cfg.LANAccess) {
		return nil
	}
	return &nftLinuxHelper{}
}

// nftLinuxHelper implements shared.NetworkHelper for Linux using nftables.
type nftLinuxHelper struct{}

// Compile-time interface assertion.
var _ shared.NetworkHelper = (*nftLinuxHelper)(nil)

func (h *nftLinuxHelper) Setup(env *shared.NetworkEnv, projectDir string, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	// nftables rules are managed per-container via ApplyRules, not per-project
	// This is a no-op for Linux - the Firewall interface handles container rules
	return &shared.PostCommitAction{}, nil
}

func (h *nftLinuxHelper) Teardown(env *shared.NetworkEnv, projectDir string) error {
	// nftables cleanup is handled per-container via Cleanup
	return nil
}

func (h *nftLinuxHelper) HelperStatus(env *shared.NetworkEnv) shared.HelperStatus {
	fs := env.Fs

	// Check if directory exists
	dirExists, _ := afero.DirExists(fs, alcatrazNftDirOnLinux)
	if !dirExists {
		return shared.HelperStatus{Installed: false, NeedsUpdate: false}
	}

	// Check if include line exists in nftables.conf
	hasInclude := h.hasIncludeLineOnLinux(fs)

	return shared.HelperStatus{
		Installed:   hasInclude,
		NeedsUpdate: false,
	}
}

func (h *nftLinuxHelper) DetailedStatus(env *shared.NetworkEnv) shared.DetailedStatusInfo {
	fs := env.Fs

	info := shared.DetailedStatusInfo{}

	// List rule files in alcatraz directory.
	// Errors are intentionally ignored: the directory may not exist if the helper
	// has not been installed yet, and this function is for display purposes only.
	files, err := afero.ReadDir(fs, alcatrazNftDirOnLinux)
	if err != nil {
		return info
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".nft") {
			continue
		}
		path := filepath.Join(alcatrazNftDirOnLinux, f.Name())
		// Skip unreadable files rather than failing the entire status display.
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

func (h *nftLinuxHelper) InstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	fs := env.Fs

	// 1. Create directory
	progress("Creating nftables directory %s...\n", alcatrazNftDirOnLinux)
	if err := fs.MkdirAll(alcatrazNftDirOnLinux, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", alcatrazNftDirOnLinux, err)
	}

	// 2. Add include line to nftables.conf
	if !h.hasIncludeLineOnLinux(fs) {
		progress("Adding include line to %s...\n", nftablesConfPathOnLinux)
		if err := h.addIncludeLineOnLinux(fs); err != nil {
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
			output, err := cmd.RunQuiet("nft", "-f", nftablesConfPathOnLinux)
			if err != nil {
				return fmt.Errorf("failed to reload nftables: %w: %s", err, strings.TrimSpace(string(output)))
			}
			return nil
		},
	}, nil
}

func (h *nftLinuxHelper) UninstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	fs := env.Fs

	// 1. Remove all files in alcatraz directory
	progress("Removing rule files from %s...\n", alcatrazNftDirOnLinux)
	files, _ := afero.ReadDir(fs, alcatrazNftDirOnLinux)
	for _, f := range files {
		if !f.IsDir() {
			_ = fs.Remove(filepath.Join(alcatrazNftDirOnLinux, f.Name()))
		}
	}

	// 2. Remove include line from nftables.conf
	progress("Removing include line from %s...\n", nftablesConfPathOnLinux)
	if err := h.removeIncludeLineOnLinux(fs); err != nil {
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
			progress("Removing directory %s...\n", alcatrazNftDirOnLinux)
			_ = fs.RemoveAll(alcatrazNftDirOnLinux)

			return nil
		},
	}, nil
}
