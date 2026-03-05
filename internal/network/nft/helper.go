package nft

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// NewHelper creates a platform-specific NetworkHelper based on the runtime platform.
func NewHelper(cfg config.Network, platform runtime.RuntimePlatform) shared.NetworkHelper {
	if runtime.IsDarwin(platform) {
		return NewDarwinHelper(cfg, platform)
	}
	if platform == runtime.PlatformLinux {
		return NewLinuxHelper(cfg, platform)
	}
	return nil
}

// hasLANAccess checks if LAN access rules require a network helper.
// Returns false for empty/nil and for wildcard-only (["*"]) — wildcard means
// allow-all, which needs no helper or firewall rules.
func hasLANAccess(lanAccess []string) bool {
	if len(lanAccess) == 0 {
		return false
	}
	if len(lanAccess) == 1 && lanAccess[0] == shared.LanAccessWildcard {
		return false
	}
	return true
}
