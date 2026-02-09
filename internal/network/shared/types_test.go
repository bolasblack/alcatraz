package shared

import (
	"testing"
)

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
