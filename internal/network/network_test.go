package network

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	alcaruntime "github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// Detect tests — cross-platform via injected RuntimePlatform
// =============================================================================

func TestDetect_LinuxWithNft(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess("which nft", []byte("/usr/sbin/nft"))
	cmd.ExpectSuccess("nft list tables", []byte(""))
	defer cmd.AssertAllExpectationsMet(t)

	fwType := Detect(context.Background(), cmd, alcaruntime.PlatformLinux)
	if fwType != TypeNFTables {
		t.Errorf("Detect(PlatformLinux, nft available) should return TypeNFTables, got %v", fwType)
	}
}

func TestDetect_LinuxWithoutNft(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectFailure("which nft", fmt.Errorf("not found"))

	fwType := Detect(context.Background(), cmd, alcaruntime.PlatformLinux)
	if fwType != TypeNone {
		t.Errorf("Detect(PlatformLinux, nft unavailable) should return TypeNone, got %v", fwType)
	}
}

func TestDetect_LinuxNftNotWorking(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess("which nft", []byte("/usr/sbin/nft"))
	cmd.ExpectFailure("nft list tables", fmt.Errorf("kernel support missing"))

	fwType := Detect(context.Background(), cmd, alcaruntime.PlatformLinux)
	if fwType != TypeNone {
		t.Errorf("Detect(PlatformLinux, nft not working) should return TypeNone, got %v", fwType)
	}
}

func TestDetect_Darwin(t *testing.T) {
	// Darwin short-circuits — no commands called
	cmd := util.NewMockCommandRunner()

	for _, platform := range []alcaruntime.RuntimePlatform{
		alcaruntime.PlatformMacDockerDesktop,
		alcaruntime.PlatformMacOrbStack,
	} {
		t.Run(string(platform), func(t *testing.T) {
			fwType := Detect(context.Background(), cmd, platform)
			if fwType != TypeNFTables {
				t.Errorf("Detect(%s) should return TypeNFTables, got %v", platform, fwType)
			}
		})
	}
}

func TestDetect_UnknownPlatform(t *testing.T) {
	cmd := util.NewMockCommandRunner()

	fwType := Detect(context.Background(), cmd, alcaruntime.RuntimePlatform("freebsd"))
	if fwType != TypeNone {
		t.Errorf("Detect(unknown) should return TypeNone, got %v", fwType)
	}
}

// =============================================================================
// New tests — cross-platform via injected RuntimePlatform
// =============================================================================

func TestNew_LinuxWithNft(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess("which nft", []byte("/usr/sbin/nft"))
	cmd.ExpectSuccess("nft list tables", []byte(""))
	defer cmd.AssertAllExpectationsMet(t)

	env := NewNetworkEnv(afero.NewMemMapFs(), cmd, "", "", alcaruntime.PlatformLinux)
	fw, fwType := New(context.Background(), env)

	if fw == nil {
		t.Error("New(PlatformLinux, nft available) should return non-nil firewall")
	}
	if fwType != TypeNFTables {
		t.Errorf("New(PlatformLinux, nft available) should return TypeNFTables, got %v", fwType)
	}
}

func TestNew_LinuxWithoutNft(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectFailure("which nft", fmt.Errorf("not found"))

	env := NewNetworkEnv(afero.NewMemMapFs(), cmd, "", "", alcaruntime.PlatformLinux)
	fw, fwType := New(context.Background(), env)

	if fw != nil {
		t.Error("New(PlatformLinux, no nft) should return nil firewall")
	}
	if fwType != TypeNone {
		t.Errorf("New(PlatformLinux, no nft) should return TypeNone, got %v", fwType)
	}
}

func TestNew_Darwin(t *testing.T) {
	// Darwin short-circuits — no nft commands called
	cmd := util.NewMockCommandRunner()

	env := NewNetworkEnv(afero.NewMemMapFs(), cmd, "", "", alcaruntime.PlatformMacDockerDesktop)
	fw, fwType := New(context.Background(), env)

	if fw == nil {
		t.Error("New(darwin) should return non-nil firewall")
	}
	if fwType != TypeNFTables {
		t.Errorf("New(darwin) should return TypeNFTables, got %v", fwType)
	}
}

// =============================================================================
// newFirewallForType tests
// =============================================================================

func TestNewFirewallForType_NFTables(t *testing.T) {
	env := NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "", "")
	fw := newFirewallForType(TypeNFTables, env)
	if fw == nil {
		t.Error("newFirewallForType(TypeNFTables) should return non-nil firewall")
	}
}

func TestNewFirewallForType_None(t *testing.T) {
	env := NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "", "")
	fw := newFirewallForType(TypeNone, env)
	if fw != nil {
		t.Errorf("newFirewallForType(TypeNone) should return nil, got %v", fw)
	}
}

func TestNewFirewallForType_UnknownType(t *testing.T) {
	env := NewNetworkEnv(afero.NewMemMapFs(), util.NewMockCommandRunner(), "", "", "")
	fw := newFirewallForType(Type(99), env)
	if fw != nil {
		t.Errorf("newFirewallForType(unknown) should return nil, got %v", fw)
	}
}

// =============================================================================
// newNetworkHelperForPlatform tests
// =============================================================================

func TestNewNetworkHelperForPlatform_DarwinWithLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
	helper := newNetworkHelperForPlatform(cfg, alcaruntime.PlatformMacOrbStack)
	if helper == nil {
		t.Error("newNetworkHelperForPlatform(darwin, LANAccess) should return non-nil helper")
	}
}

func TestNewNetworkHelperForPlatform_LinuxWithLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
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
	cfg := config.Network{LANAccess: []string{"192.168.1.0/24"}}
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
