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

// hasLANAccess checks if LAN access is configured.
// Returns true if any lan-access rules are specified (not just wildcard).
// TODO: lan-access=["*"] returns true here, causing setupNetwork to run unnecessarily
// since ["*"] means allow-all (no helper/rules needed). Low priority — harmless but wasteful.
func hasLANAccess(lanAccess []string) bool {
	return len(lanAccess) > 0
}
