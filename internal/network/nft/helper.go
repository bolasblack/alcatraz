package nft

import (
	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// NewHelperForProject creates a platform-specific NetworkHelper based on the runtime platform.
func NewHelperForProject(cfg config.Network, platform runtime.RuntimePlatform) shared.NetworkHelper {
	if !hasLANAccess(cfg.LANAccess) {
		return nil
	}
	return NewHelperForSystem(platform)
}

// NewHelperForSystem creates a NetworkHelper for system-level operations
// (install/uninstall/status). Unlike NewHelperForProject, this does not check LANAccess
// config — system operations are platform-level, not project-level.
func NewHelperForSystem(platform runtime.RuntimePlatform) shared.NetworkHelper {
	if runtime.IsDarwin(platform) {
		return NewDarwinHelper(platform)
	}
	if platform == runtime.PlatformLinux {
		return NewLinuxHelper()
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
