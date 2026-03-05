package nft

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// NewHelperForProject Factory Tests
// =============================================================================

func TestNewHelperForProject_ReturnsFunctionalHelperForOrbStack(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
	helper := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)

	assert.NotNil(t, helper, "NewHelperForProject should return non-nil for darwin OrbStack platform")

	// Verify the helper works through the NetworkHelper interface
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "", runtime.PlatformMacOrbStack)
	action, err := helper.Setup(env, "/project", nil)
	assert.NoError(t, err, "Setup should succeed for OrbStack helper")
	assert.NotNil(t, action, "Setup should return a PostCommitAction")
}

func TestNewHelperForProject_ReturnsFunctionalHelperForDockerDesktop(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
	helper := NewHelperForProject(cfg, runtime.PlatformMacDockerDesktop)

	assert.NotNil(t, helper, "NewHelperForProject should return non-nil for darwin Docker Desktop platform")

	// Verify the helper works through the NetworkHelper interface
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "", runtime.PlatformMacDockerDesktop)
	action, err := helper.Setup(env, "/project", nil)
	assert.NoError(t, err, "Setup should succeed for Docker Desktop helper")
	assert.NotNil(t, action, "Setup should return a PostCommitAction")
}

func TestNewHelperForProject_ReturnsFunctionalHelperForLinux(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
	helper := NewHelperForProject(cfg, runtime.PlatformLinux)

	assert.NotNil(t, helper, "NewHelperForProject should return non-nil for Linux platform")

	// Verify the helper works through the NetworkHelper interface
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "", runtime.PlatformLinux)
	err := helper.Teardown(env, "/project")
	assert.NoError(t, err, "Teardown should succeed for Linux helper")
}

func TestNewHelperForProject_ReturnsNilForUnsupportedPlatform(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
	helper := NewHelperForProject(cfg, runtime.RuntimePlatform("freebsd"))

	assert.Nil(t, helper, "NewHelperForProject should return nil for unsupported platform")
}

func TestNewHelperForProject_ReturnsNilWhenLANAccessIsWildcard(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"*"}}

	helperDarwin := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.Nil(t, helperDarwin, "NewHelperForProject should return nil when LANAccess is wildcard (darwin)")

	helperLinux := NewHelperForProject(cfg, runtime.PlatformLinux)
	assert.Nil(t, helperLinux, "NewHelperForProject should return nil when LANAccess is wildcard (linux)")
}

func TestNewHelperForProject_ReturnsNilWhenLANAccessEmpty(t *testing.T) {
	cfg := config.Network{LANAccess: []string{}}

	helperDarwin := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.Nil(t, helperDarwin, "NewHelperForProject should return nil when LANAccess is empty (darwin)")

	helperLinux := NewHelperForProject(cfg, runtime.PlatformLinux)
	assert.Nil(t, helperLinux, "NewHelperForProject should return nil when LANAccess is empty (linux)")
}

func TestNewHelperForProject_ReturnsNilWhenLANAccessNil(t *testing.T) {
	cfg := config.Network{LANAccess: nil}

	helper := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.Nil(t, helper, "NewHelperForProject should return nil when LANAccess is nil")
}

func TestNewHelperForProject_ReturnsNonNilWhenLANAccessHasEntries(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}

	helper := NewHelperForProject(cfg, runtime.PlatformMacOrbStack)
	assert.NotNil(t, helper, "NewHelperForProject should return non-nil when LANAccess has entries")
}

// =============================================================================
// hasLANAccess Tests
// =============================================================================

func TestHasLANAccess_ReturnsFalseForNil(t *testing.T) {
	assert.False(t, hasLANAccess(nil))
}

func TestHasLANAccess_ReturnsFalseForEmpty(t *testing.T) {
	assert.False(t, hasLANAccess([]string{}))
}

func TestHasLANAccess_ReturnsFalseForWildcard(t *testing.T) {
	assert.False(t, hasLANAccess([]string{"*"}))
}

func TestHasLANAccess_ReturnsTrueForWildcardWithOtherRules(t *testing.T) {
	assert.True(t, hasLANAccess([]string{"*", "192.168.1.0/24"}))
}

func TestHasLANAccess_ReturnsTrueForCIDR(t *testing.T) {
	assert.True(t, hasLANAccess([]string{"192.168.1.0/24"}))
}
