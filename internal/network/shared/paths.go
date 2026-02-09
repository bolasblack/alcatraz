package shared

import "github.com/bolasblack/alcatraz/internal/util"

// Path constants shared across multiple packages (nft, vmhelper).
// Platform-specific paths that belong to a single package live in that package
// (e.g., nft/paths.go for Linux-only nftables paths).

// NftDirRel is the nft rule directory path relative to user home.
// Used by both nft/paths.go (nftDirOnDarwin) and vmhelper (directory creation).
const NftDirRel = util.FilesDir + "/alcatraz_nft"
