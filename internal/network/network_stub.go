//go:build !darwin && !linux

package network

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// newFirewallForType returns nil on unsupported platforms.
func newFirewallForType(_ Type, _ *NetworkEnv) Firewall {
	return nil
}

// newNetworkHelperForPlatform returns nil on unsupported platforms.
func newNetworkHelperForPlatform(_ config.Network, _ string) shared.NetworkHelper {
	return nil
}
