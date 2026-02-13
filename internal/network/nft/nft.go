// Package nft implements network isolation using nftables.
// On Linux: per-container tables via direct nft execution (AGD-027).
// On macOS: per-project rules via VM nftables (AGD-030).
// See AGD-028 for the lan-access rule syntax specification.
package nft

import (
	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// New creates a new NFTables firewall instance.
func New(env *shared.NetworkEnv) shared.Firewall {
	var vmHelperEnv *vmhelper.VMHelperEnv
	if runtime.IsDarwin(env.Runtime) {
		vmHelperEnv = vmhelper.NewVMHelperEnv(env.Fs, env.Cmd)
	}
	return &NFTables{env: env, vmHelperEnv: vmHelperEnv}
}
