//go:build linux

// Package nft implements network isolation for Linux using nftables.
// See AGD-027 for the decision to use nftables as the primary Linux network.
// See AGD-028 for the lan-access rule syntax specification.
package nft

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// New creates a new NFTables firewall instance.
func New(env *shared.NetworkEnv) shared.Firewall {
	return &NFTables{env: env}
}

// NewHelper creates a NetworkHelper for Linux.
func NewHelper(cfg config.Network, _ string) shared.NetworkHelper {
	if !hasLANAccess(cfg.LANAccess) {
		return nil
	}
	return &nftHelper{}
}

// hasLANAccess checks if LAN access is configured.
// Returns true if any lan-access rules are specified (not just wildcard).
func hasLANAccess(lanAccess []string) bool {
	return len(lanAccess) > 0
}
