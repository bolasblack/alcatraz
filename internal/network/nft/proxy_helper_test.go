package nft

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// Test: proxy alone (no lan-access) activates network helper
func TestNewHelperForProject_ReturnsNonNilWhenProxyConfigured(t *testing.T) {
	cfg := config.Network{Proxy: "10.0.0.1:1080"}

	helper := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.NotNil(t, helper, "NewHelperForProject should return non-nil when Proxy is configured")

	helperLinux := NewHelperForProject(cfg, runtime.PlatformLinux)
	assert.NotNil(t, helperLinux, "NewHelperForProject should return non-nil when Proxy is configured (linux)")
}

// Test: neither lan-access nor proxy returns nil
func TestNewHelperForProject_ReturnsNilWhenNoLANAccessAndNoProxy(t *testing.T) {
	cfg := config.Network{}

	helper := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.Nil(t, helper, "NewHelperForProject should return nil when no LANAccess and no Proxy")
}

// Test: wildcard lan-access with proxy still activates helper (proxy needs it)
func TestNewHelperForProject_ReturnsNonNilWhenWildcardLANAccessWithProxy(t *testing.T) {
	cfg := config.Network{
		LANAccess: []string{"*"},
		Proxy:     "10.0.0.1:1080",
	}

	helper := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.NotNil(t, helper, "NewHelperForProject should return non-nil when Proxy is configured even with wildcard LANAccess")
}
