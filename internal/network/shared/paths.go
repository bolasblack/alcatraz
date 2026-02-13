package shared

import "github.com/bolasblack/alcatraz/internal/util"

// Path constants shared across multiple packages (nft, network helper).
// Platform-specific paths that belong to a single package live in that package
// (e.g., nft/paths.go for Linux-only nftables paths).

// NftDirRel is the nft rule directory path relative to user home.
// Used by both nft/paths.go (nftDirOnDarwin) and network helper (directory creation).
const NftDirRel = util.FilesDir + "/alcatraz_nft"
