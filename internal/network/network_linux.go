//go:build linux

package network

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/nft"
	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// newFirewallForType creates a Firewall for the given type.
func newFirewallForType(t Type, env *NetworkEnv) Firewall {
	switch t {
	case TypeNFTables:
		return nft.New(env)
	default:
		return nil
	}
}

// newNetworkHelperForPlatform creates a platform-specific NetworkHelper.
func newNetworkHelperForPlatform(cfg config.Network, runtimeName string) shared.NetworkHelper {
	return nft.NewHelper(cfg, runtimeName)
}
