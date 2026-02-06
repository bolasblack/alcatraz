package shared

import (
	"testing"
)

func TestShortContainerID(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		want        string
	}{
		{
			name:        "empty string",
			containerID: "",
			want:        "",
		},
		{
			name:        "shorter than 12 chars",
			containerID: "abc123",
			want:        "abc123",
		},
		{
			name:        "exactly 12 chars",
			containerID: "abc123def456",
			want:        "abc123def456",
		},
		{
			name:        "longer than 12 chars",
			containerID: "abc123def456789",
			want:        "abc123def456",
		},
		{
			name:        "full SHA256 container ID (64 chars)",
			containerID: "abc123def456789xyz0123456789abcdef0123456789abcdef0123456789abcd",
			want:        "abc123def456",
		},
		{
			name:        "single char",
			containerID: "a",
			want:        "a",
		},
		{
			name:        "12 chars boundary",
			containerID: "123456789012",
			want:        "123456789012",
		},
		{
			name:        "13 chars boundary",
			containerID: "1234567890123",
			want:        "123456789012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortContainerID(tt.containerID)
			if got != tt.want {
				t.Errorf("ShortContainerID(%q) = %q, want %q", tt.containerID, got, tt.want)
			}
		})
	}
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// IPv4 addresses
		{
			name: "IPv4 simple",
			ip:   "192.168.1.1",
			want: false,
		},
		{
			name: "IPv4 loopback",
			ip:   "127.0.0.1",
			want: false,
		},
		{
			name: "IPv4 CIDR",
			ip:   "10.0.0.0/8",
			want: false,
		},
		{
			name: "empty string",
			ip:   "",
			want: false,
		},

		// IPv6 addresses
		{
			name: "IPv6 full",
			ip:   "2001:db8::1",
			want: true,
		},
		{
			name: "IPv6 loopback",
			ip:   "::1",
			want: true,
		},
		{
			name: "IPv6 link-local",
			ip:   "fe80::1",
			want: true,
		},
		{
			name: "IPv6 CIDR",
			ip:   "2001:db8::/32",
			want: true,
		},
		{
			name: "IPv6 full notation",
			ip:   "2001:0db8:0000:0000:0000:0000:0000:0001",
			want: true,
		},
		{
			name: "IPv6 with zone ID",
			ip:   "fe80::1%eth0",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsIPv6(tt.ip)
			if got != tt.want {
				t.Errorf("IsIPv6(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestSafeProgress(t *testing.T) {
	t.Run("nil progress returns no-op function", func(t *testing.T) {
		fn := SafeProgress(nil)
		if fn == nil {
			t.Fatal("SafeProgress(nil) should return non-nil function")
		}
		// Should not panic when called
		fn("test %s", "arg")
	})

	t.Run("non-nil progress returns same function behavior", func(t *testing.T) {
		var called bool
		var gotFormat string
		var gotArgs []any

		original := func(format string, args ...any) {
			called = true
			gotFormat = format
			gotArgs = args
		}

		fn := SafeProgress(original)
		fn("test %s %d", "hello", 42)

		if !called {
			t.Error("original function should have been called")
		}
		if gotFormat != "test %s %d" {
			t.Errorf("format = %q, want %q", gotFormat, "test %s %d")
		}
		if len(gotArgs) != 2 {
			t.Errorf("len(args) = %d, want 2", len(gotArgs))
		}
	})
}
