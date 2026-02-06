package network

import (
	"runtime"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

func TestDetect(t *testing.T) {
	// Test detection on current platform
	fwType := Detect(util.NewCommandRunner())

	switch runtime.GOOS {
	case "darwin":
		if fwType != TypePF {
			t.Errorf("Detect() on darwin should return TypePF, got %v", fwType)
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

func TestTypeString(t *testing.T) {
	tests := []struct {
		fwType Type
		want   string
	}{
		{TypeNone, "none"},
		{TypeNFTables, "nftables"},
		{TypePF, "pf"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.fwType.String(); got != tt.want {
				t.Errorf("Type(%d).String() = %q, want %q", tt.fwType, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	env := NewNetworkEnv(afero.NewOsFs(), util.NewCommandRunner(), "", false)
	fw, fwType := New(env)

	switch runtime.GOOS {
	case "darwin":
		if fw == nil {
			t.Error("New() on darwin should return non-nil PF firewall")
		}
		if fwType != TypePF {
			t.Errorf("New() on darwin should return TypePF, got %v", fwType)
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

func TestNewNetworkEnv(t *testing.T) {
	fs := afero.NewOsFs()
	cmd := util.NewCommandRunner()
	env := NewNetworkEnv(fs, cmd, "", false)

	if env == nil {
		t.Fatal("NewNetworkEnv() should not return nil")
	}
	if env.Fs != fs {
		t.Error("NewNetworkEnv().Fs should be the provided filesystem")
	}
	if env.Cmd != cmd {
		t.Error("NewNetworkEnv().Cmd should be the provided CommandRunner")
	}
}
