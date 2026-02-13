package nft

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// nftDarwinHelper implements shared.NetworkHelper for macOS using vmhelper + nft.
type nftDarwinHelper struct {
	platform         runtime.RuntimePlatform
	vmHelperEnvCache *vmhelper.VMHelperEnv
}

// Compile-time interface assertion.
var _ shared.NetworkHelper = (*nftDarwinHelper)(nil)

// NewDarwinHelper creates a NetworkHelper for macOS.
// Returns nil if no LAN access is configured.
func NewDarwinHelper(cfg config.Network, platform runtime.RuntimePlatform) shared.NetworkHelper {
	if !hasLANAccess(cfg.LANAccess) {
		return nil
	}
	return &nftDarwinHelper{platform: platform}
}

func (h *nftDarwinHelper) vmHelperEnv(env *shared.NetworkEnv) *vmhelper.VMHelperEnv {
	if h.vmHelperEnvCache == nil {
		h.vmHelperEnvCache = vmhelper.NewVMHelperEnv(env.Fs, env.Cmd)
	}
	return h.vmHelperEnvCache
}

func (h *nftDarwinHelper) Setup(env *shared.NetworkEnv, projectDir string, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	// Per-container rules are applied via Firewall.ApplyRules.
	// NetworkHelper.Setup is a no-op on macOS — the helper container handles rule loading.
	return &shared.PostCommitAction{}, nil
}

func (h *nftDarwinHelper) Teardown(env *shared.NetworkEnv, projectDir string) error {
	// No-op: per-container .nft files are cleaned up via Firewall.Cleanup()
	// which is called per-container by `alca down`.
	return nil
}

func (h *nftDarwinHelper) HelperStatus(ctx context.Context, env *shared.NetworkEnv) shared.HelperStatus {
	vmHelperEnv := h.vmHelperEnv(env)

	installed, err := vmhelper.IsInstalled(ctx, vmHelperEnv)
	if err != nil {
		return shared.HelperStatus{Installed: false}
	}

	needsUpdate := false
	if installed {
		needsUpdate, _ = vmhelper.NeedsUpdate(vmHelperEnv)
	}

	return shared.HelperStatus{
		Installed:   installed,
		NeedsUpdate: needsUpdate,
	}
}

func (h *nftDarwinHelper) DetailedStatus(env *shared.NetworkEnv) shared.DetailedStatusInfo {
	info := shared.DetailedStatusInfo{}

	// List rule files in macOS nft directory.
	// Errors are intentionally ignored: the directory may not exist if the helper
	// has not been installed yet, and this function is for display purposes only.
	nftDirPath, err := nftDirOnDarwin()
	if err != nil {
		return info
	}

	files, err := afero.ReadDir(env.Fs, nftDirPath)
	if err != nil {
		return info
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".nft") {
			continue
		}
		path := filepath.Join(nftDirPath, f.Name())
		// Skip unreadable files rather than failing the entire status display.
		content, err := afero.ReadFile(env.Fs, path)
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

func (h *nftDarwinHelper) InstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	vmHelperEnv := h.vmHelperEnv(env)
	platform := h.platform

	// Write entry.sh and create directories via TransactFs (pre-commit).
	// These changes are staged and will be flushed to disk when the caller commits.
	progress("Writing entry script and creating directories...\n")
	if err := vmhelper.WriteEntryScript(vmHelperEnv); err != nil {
		return nil, fmt.Errorf("failed to write entry script: %w", err)
	}

	return &shared.PostCommitAction{
		Run: func(ctx context.Context, progress shared.ProgressFunc) error {
			return vmhelper.InstallHelper(ctx, vmHelperEnv, platform, progress)
		},
	}, nil
}

func (h *nftDarwinHelper) UninstallHelper(env *shared.NetworkEnv, _ shared.ProgressFunc) (*shared.PostCommitAction, error) {
	// No pre-commit progress reporting needed; PostCommitAction.Run receives its own progress.
	vmHelperEnv := h.vmHelperEnv(env)

	return &shared.PostCommitAction{
		Run: func(ctx context.Context, progress shared.ProgressFunc) error {
			return vmhelper.UninstallHelper(ctx, vmHelperEnv, progress)
		},
	}, nil
}
