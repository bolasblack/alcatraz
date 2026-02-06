package shared

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

func TestNewNetworkEnv(t *testing.T) {
	fs := afero.NewOsFs()
	cmd := util.NewCommandRunner()
	env := NewNetworkEnv(fs, cmd, "", false)

	if env == nil {
		t.Fatal("NewNetworkEnv() should return non-nil NetworkEnv")
	}
	if env.Fs != fs {
		t.Error("NewNetworkEnv().Fs should be the provided filesystem")
	}
	if env.Cmd != cmd {
		t.Error("NewNetworkEnv().Cmd should be the provided CommandRunner")
	}
}

func TestNewTestNetworkEnv(t *testing.T) {
	env := NewTestNetworkEnv()

	if env == nil {
		t.Fatal("NewTestNetworkEnv() should return non-nil NetworkEnv")
	}
	if env.Fs == nil {
		t.Error("NewTestNetworkEnv().Fs should be non-nil")
	}
	if env.Cmd == nil {
		t.Error("NewTestNetworkEnv().Cmd should be non-nil")
	}
}

func TestTypeString(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{TypeNone, "none"},
		{TypeNFTables, "nftables"},
		{TypePF, "pf"},
		{Type(99), "none"}, // Unknown type defaults to "none"
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("Type(%d).String() = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}
