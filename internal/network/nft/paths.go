package nft

import (
	"os"
	"path/filepath"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// Linux-specific nftables paths. These are only used within the nft package.
// Cross-package paths live in shared/paths.go (e.g., NftDirRel for macOS).
const (
	nftablesConfPathOnLinux = "/etc/nftables.conf"
	alcatrazNftDirOnLinux   = "/etc/nftables.d/alcatraz"
)

// nftDirOnLinux returns the Linux nftables rule directory path.
func nftDirOnLinux() string {
	return alcatrazNftDirOnLinux
}

// nftDirOnDarwin returns the macOS nft rule directory path.
// Returns ~/.alcatraz/files/alcatraz_nft/
func nftDirOnDarwin() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, shared.NftDirRel), nil
}
