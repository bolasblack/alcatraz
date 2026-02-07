//go:build darwin

package network

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/pf"
	"github.com/bolasblack/alcatraz/internal/network/shared"
)

func ensureFirewallSystemConfig(env *NetworkEnv, fwType Type) error {
	if fwType == TypePF {
		return pf.EnsureAnchorConfig(env)
	}
	return nil
}

// newFirewallForType creates a Firewall for the given type.
func newFirewallForType(t Type, env *NetworkEnv) Firewall {
	switch t {
	case TypePF:
		return pf.New(env)
	default:
		return nil
	}
}

// newNetworkHelperForPlatform creates a platform-specific NetworkHelper.
func newNetworkHelperForPlatform(cfg config.Network, runtimeName string) shared.NetworkHelper {
	return pf.NewHelper(cfg, runtimeName)
}
