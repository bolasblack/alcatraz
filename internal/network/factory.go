package network

import (
	goruntime "runtime"

	"github.com/bolasblack/alcatraz/internal/config"
)

// New creates a NetworkHelper for the given platform and runtime.
// Returns nil if network isolation is not needed.
func New(cfg config.Network, runtimeName string) NetworkHelper {
	if !hasLANAccess(cfg.LANAccess) {
		return nil
	}

	switch goruntime.GOOS {
	case "darwin":
		if runtimeName == "docker" {
			return nil // Docker Desktop handles LAN access natively
		}
		return newPfHelper()
	default:
		return nil
	}
}

// hasLANAccess checks if LAN access is configured.
func hasLANAccess(lanAccess []string) bool {
	for _, access := range lanAccess {
		if access == lanAccessWildcard {
			return true
		}
	}
	return false
}
