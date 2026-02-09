package network

import (
	"runtime"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	alcaruntime "github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

func TestDetect(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: depends on host platform and installed tools")
	}
	fwType := Detect(util.NewCommandRunner())

	switch runtime.GOOS {
	case "darwin":
		if fwType != TypeNFTables {
			t.Errorf("Detect() on darwin should return TypeNFTables, got %v", fwType)
		}
	case "linux":
		// On Linux, result depends on whether nft is available
		// Just verify it returns a valid type
		if fwType != TypeNFTables && fwType != TypeNone {
			t.Errorf("Detect() on linux should return TypeNFTables or TypeNone, got %v", fwType)
		}
	default:
		if fwType != TypeNone {
			t.Errorf("Detect() on %s should return TypeNone, got %v", runtime.GOOS, fwType)
		}
	}
}

func TestNew(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: depends on host platform and installed tools")
	}
	env := NewNetworkEnv(afero.NewOsFs(), util.NewCommandRunner(), "", "")
	fw, fwType := New(env)

	switch runtime.GOOS {
	case "darwin":
		if fw == nil {
			t.Error("New() on darwin should return non-nil NFTables firewall")
		}
		if fwType != TypeNFTables {
			t.Errorf("New() on darwin should return TypeNFTables, got %v", fwType)
		}
	case "linux":
		// On Linux, depends on nftables availability
		if fwType == TypeNFTables {
			if fw == nil {
				t.Error("New() should return non-nil firewall when TypeNFTables")
			}
		} else {
			if fw != nil {
				t.Error("New() should return nil firewall when TypeNone")
			}
		}
	default:
		if fw != nil {
			t.Errorf("New() on %s should return nil firewall", runtime.GOOS)
		}
		if fwType != TypeNone {
			t.Errorf("New() on %s should return TypeNone, got %v", runtime.GOOS, fwType)
		}
	}
}

// =============================================================================
// newFirewallForType tests
// =============================================================================

func TestNewFirewallForType_NFTables(t *testing.T) {
	env := NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "")
	fw := newFirewallForType(TypeNFTables, env)
	if fw == nil {
		t.Error("newFirewallForType(TypeNFTables) should return non-nil firewall")
	}
}

func TestNewFirewallForType_None(t *testing.T) {
	env := NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "")
	fw := newFirewallForType(TypeNone, env)
	if fw != nil {
		t.Errorf("newFirewallForType(TypeNone) should return nil, got %v", fw)
	}
}

func TestNewFirewallForType_UnknownType(t *testing.T) {
	env := NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "")
	fw := newFirewallForType(Type(99), env)
	if fw != nil {
		t.Errorf("newFirewallForType(unknown) should return nil, got %v", fw)
	}
}

// =============================================================================
// newNetworkHelperForPlatform tests
// =============================================================================

func TestNewNetworkHelperForPlatform_DarwinWithLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"*"}}
	helper := newNetworkHelperForPlatform(cfg, alcaruntime.PlatformMacOrbStack)
	if helper == nil {
		t.Error("newNetworkHelperForPlatform(darwin, LANAccess) should return non-nil helper")
	}
}

func TestNewNetworkHelperForPlatform_LinuxWithLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"*"}}
	helper := newNetworkHelperForPlatform(cfg, alcaruntime.PlatformLinux)
	if helper == nil {
		t.Error("newNetworkHelperForPlatform(linux, LANAccess) should return non-nil helper")
	}
}

func TestNewNetworkHelperForPlatform_EmptyLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: nil}

	platforms := []alcaruntime.RuntimePlatform{
		alcaruntime.PlatformLinux,
		alcaruntime.PlatformMacOrbStack,
		alcaruntime.PlatformMacDockerDesktop,
	}

	for _, platform := range platforms {
		t.Run(string(platform), func(t *testing.T) {
			helper := newNetworkHelperForPlatform(cfg, platform)
			if helper != nil {
				t.Errorf("newNetworkHelperForPlatform(%s, no LANAccess) should return nil, got %v", platform, helper)
			}
		})
	}
}

func TestNewNetworkHelperForPlatform_UnsupportedPlatform(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"*"}}
	helper := newNetworkHelperForPlatform(cfg, alcaruntime.RuntimePlatform("unknown-os"))
	if helper != nil {
		t.Errorf("newNetworkHelperForPlatform(unsupported) should return nil, got %v", helper)
	}
}

func TestNewNetworkHelperForPlatform_DockerDesktopWithLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
	helper := newNetworkHelperForPlatform(cfg, alcaruntime.PlatformMacDockerDesktop)
	if helper == nil {
		t.Error("newNetworkHelperForPlatform(docker-desktop, LANAccess) should return non-nil helper")
	}
}
