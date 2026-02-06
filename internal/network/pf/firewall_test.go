//go:build darwin

package pf

import (
	"strings"
	"testing"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

func TestGeneratePfRuleset_BlockOnly(t *testing.T) {
	tests := []struct {
		name        string
		containerIP string
		rules       []shared.LANAccessRule
		wantBlocks  []string // substrings that should appear
		wantMissing []string // substrings that should NOT appear
	}{
		{
			name:        "IPv4 container blocks RFC1918",
			containerIP: "172.20.0.5",
			rules:       nil,
			wantBlocks: []string{
				"block drop quick from 172.20.0.5 to 10.0.0.0/8",
				"block drop quick from 172.20.0.5 to 172.16.0.0/12",
				"block drop quick from 172.20.0.5 to 192.168.0.0/16",
				"block drop quick from 172.20.0.5 to 169.254.0.0/16",
				"block drop quick from 172.20.0.5 to 127.0.0.0/8",
			},
			wantMissing: []string{
				"fe80::/10", // IPv6 ranges should not appear for IPv4
				"fc00::/7",
			},
		},
		{
			name:        "IPv6 container blocks private IPv6 ranges",
			containerIP: "2001:db8::5",
			rules:       nil,
			wantBlocks: []string{
				"block drop quick from 2001:db8::5 to fe80::/10",
				"block drop quick from 2001:db8::5 to fc00::/7",
				"block drop quick from 2001:db8::5 to ::1/128",
			},
			wantMissing: []string{
				"10.0.0.0/8",     // IPv4 ranges should not appear for IPv6
				"192.168.0.0/16", // IPv4 ranges should not appear for IPv6
			},
		},
		{
			name:        "empty rules still blocks",
			containerIP: "172.20.0.5",
			rules:       []shared.LANAccessRule{},
			wantBlocks: []string{
				"block drop quick from 172.20.0.5 to 10.0.0.0/8",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generatePfRuleset(tt.containerIP, tt.rules)

			for _, want := range tt.wantBlocks {
				if !strings.Contains(result, want) {
					t.Errorf("generatePfRuleset() missing expected block rule:\n  want: %q\n  got:\n%s", want, result)
				}
			}

			for _, notWant := range tt.wantMissing {
				if strings.Contains(result, notWant) {
					t.Errorf("generatePfRuleset() should not contain %q for this IP type:\n%s", notWant, result)
				}
			}
		})
	}
}

func TestGeneratePfRuleset_WithAllowRules(t *testing.T) {
	tests := []struct {
		name        string
		containerIP string
		rules       []shared.LANAccessRule
		wantPass    []string // pass rules that should appear
		wantBlocks  []string // block rules that should still appear
	}{
		{
			name:        "allow specific IP all ports",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoAll},
			},
			wantPass: []string{
				"pass quick from 172.20.0.5 to 192.168.1.100",
			},
			wantBlocks: []string{
				"block drop quick from 172.20.0.5 to 10.0.0.0/8",
			},
		},
		{
			name:        "allow TCP specific port",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP},
			},
			wantPass: []string{
				"pass quick proto tcp from 172.20.0.5 to 192.168.1.100 port 8080",
			},
		},
		{
			name:        "allow UDP specific port",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "10.0.0.53", Port: 53, Protocol: shared.ProtoUDP},
			},
			wantPass: []string{
				"pass quick proto udp from 172.20.0.5 to 10.0.0.53 port 53",
			},
		},
		{
			name:        "allow all protocols specific port generates both TCP and UDP",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "192.168.1.100", Port: 443, Protocol: shared.ProtoAll},
			},
			wantPass: []string{
				"pass quick proto tcp from 172.20.0.5 to 192.168.1.100 port 443",
				"pass quick proto udp from 172.20.0.5 to 192.168.1.100 port 443",
			},
		},
		{
			name:        "allow TCP all ports",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoTCP},
			},
			wantPass: []string{
				"pass quick proto tcp from 172.20.0.5 to 192.168.1.100",
			},
		},
		{
			name:        "allow UDP all ports",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoUDP},
			},
			wantPass: []string{
				"pass quick proto udp from 172.20.0.5 to 192.168.1.100",
			},
		},
		{
			name:        "multiple allow rules",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
				{IP: "10.0.0.53", Port: 53, Protocol: shared.ProtoUDP},
			},
			wantPass: []string{
				"pass quick proto tcp from 172.20.0.5 to 192.168.1.100 port 80",
				"pass quick proto udp from 172.20.0.5 to 10.0.0.53 port 53",
			},
		},
		{
			name:        "allow CIDR range",
			containerIP: "172.20.0.5",
			rules: []shared.LANAccessRule{
				{IP: "10.0.0.0/8", Port: 0, Protocol: shared.ProtoAll},
			},
			wantPass: []string{
				"pass quick from 172.20.0.5 to 10.0.0.0/8",
			},
		},
		{
			name:        "IPv6 allow rule",
			containerIP: "2001:db8::5",
			rules: []shared.LANAccessRule{
				{IP: "fe80::1", Port: 8080, Protocol: shared.ProtoTCP, IsIPv6: true},
			},
			wantPass: []string{
				"pass quick proto tcp from 2001:db8::5 to fe80::1 port 8080",
			},
			wantBlocks: []string{
				"block drop quick from 2001:db8::5 to fe80::/10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generatePfRuleset(tt.containerIP, tt.rules)

			for _, want := range tt.wantPass {
				if !strings.Contains(result, want) {
					t.Errorf("generatePfRuleset() missing expected pass rule:\n  want: %q\n  got:\n%s", want, result)
				}
			}

			for _, want := range tt.wantBlocks {
				if !strings.Contains(result, want) {
					t.Errorf("generatePfRuleset() missing expected block rule:\n  want: %q\n  got:\n%s", want, result)
				}
			}
		})
	}
}

func TestGeneratePfRuleset_AllLANSkipped(t *testing.T) {
	// When AllLAN is true, the rule should be skipped (no pass rule generated)
	// The block rules should still be present
	rules := []shared.LANAccessRule{
		{AllLAN: true},
		{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
	}

	result := generatePfRuleset("172.20.0.5", rules)

	// Should have pass rule for the non-AllLAN rule
	if !strings.Contains(result, "pass quick proto tcp from 172.20.0.5 to 192.168.1.100 port 80") {
		t.Errorf("expected pass rule for specific IP, got:\n%s", result)
	}

	// Should still have block rules
	if !strings.Contains(result, "block drop quick") {
		t.Errorf("expected block rules, got:\n%s", result)
	}
}

func TestGeneratePfRuleset_PassBeforeBlock(t *testing.T) {
	// Verify pass rules come BEFORE block rules (critical for pf first-match behavior)
	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
	}

	result := generatePfRuleset("172.20.0.5", rules)

	passIdx := strings.Index(result, "pass quick")
	blockIdx := strings.Index(result, "block drop quick")

	if passIdx == -1 {
		t.Fatal("no pass rule found")
	}
	if blockIdx == -1 {
		t.Fatal("no block rule found")
	}
	if passIdx > blockIdx {
		t.Errorf("pass rules should come BEFORE block rules for pf first-match semantics.\npass at %d, block at %d\nruleset:\n%s", passIdx, blockIdx, result)
	}
}

func TestWritePfAllowRule(t *testing.T) {
	tests := []struct {
		name        string
		containerIP string
		rule        shared.LANAccessRule
		expected    string
	}{
		{
			name:        "all ports all protocols",
			containerIP: "172.20.0.5",
			rule:        shared.LANAccessRule{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoAll},
			expected:    "pass quick from 172.20.0.5 to 192.168.1.100\n",
		},
		{
			name:        "TCP all ports",
			containerIP: "172.20.0.5",
			rule:        shared.LANAccessRule{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoTCP},
			expected:    "pass quick proto tcp from 172.20.0.5 to 192.168.1.100\n",
		},
		{
			name:        "UDP all ports",
			containerIP: "172.20.0.5",
			rule:        shared.LANAccessRule{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoUDP},
			expected:    "pass quick proto udp from 172.20.0.5 to 192.168.1.100\n",
		},
		{
			name:        "TCP specific port",
			containerIP: "172.20.0.5",
			rule:        shared.LANAccessRule{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP},
			expected:    "pass quick proto tcp from 172.20.0.5 to 192.168.1.100 port 8080\n",
		},
		{
			name:        "UDP specific port",
			containerIP: "172.20.0.5",
			rule:        shared.LANAccessRule{IP: "192.168.1.100", Port: 53, Protocol: shared.ProtoUDP},
			expected:    "pass quick proto udp from 172.20.0.5 to 192.168.1.100 port 53\n",
		},
		{
			name:        "all protocols specific port generates two rules",
			containerIP: "172.20.0.5",
			rule:        shared.LANAccessRule{IP: "192.168.1.100", Port: 443, Protocol: shared.ProtoAll},
			expected:    "pass quick proto tcp from 172.20.0.5 to 192.168.1.100 port 443\npass quick proto udp from 172.20.0.5 to 192.168.1.100 port 443\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			writePfAllowRule(&sb, tt.containerIP, tt.rule)
			result := sb.String()
			if result != tt.expected {
				t.Errorf("writePfAllowRule() =\n%q\nwant:\n%q", result, tt.expected)
			}
		})
	}
}
