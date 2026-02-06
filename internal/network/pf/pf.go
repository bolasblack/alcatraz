//go:build darwin

// Package pf implements network isolation for macOS using pf (packet filter).
// See AGD-027 for the decision to use pf for macOS.
// See AGD-028 for the lan-access rule syntax specification.
package pf

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// New creates a new PF firewall instance.
func New(env *shared.NetworkEnv) shared.Firewall {
	return &PF{env: env}
}

// NewHelper creates a NetworkHelper for macOS.
// Returns nil for Docker Desktop (handles LAN access natively).
func NewHelper(cfg config.Network, runtimeName string) shared.NetworkHelper {
	if !hasLANAccess(cfg.LANAccess) {
		return nil
	}
	if runtimeName == "docker" {
		return nil // Docker Desktop handles LAN access natively
	}
	return &pfHelper{}
}

// hasLANAccess checks if LAN access is configured.
// Returns true if any lan-access rules are specified (not just wildcard).
func hasLANAccess(lanAccess []string) bool {
	return len(lanAccess) > 0
}
