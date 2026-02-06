//go:build linux

package nft

import (
	"strings"
	"testing"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

func TestTableName(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		want        string
	}{
		{
			name:        "short container ID",
			containerID: "abc123",
			want:        "alca-abc123",
		},
		{
			name:        "exactly 12 chars",
			containerID: "abc123def456",
			want:        "alca-abc123def456",
		},
		{
			name:        "long container ID (truncated to 12)",
			containerID: "abc123def456789xyz",
			want:        "alca-abc123def456",
		},
		{
			name:        "full SHA256 container ID",
			containerID: "abc123def456789xyz0123456789abcdef0123456789abcdef0123456789abcd",
			want:        "alca-abc123def456",
		},
		{
			name:        "empty container ID",
			containerID: "",
			want:        "alca-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableName(tt.containerID)
			if got != tt.want {
				t.Errorf("tableName(%q) = %q, want %q", tt.containerID, got, tt.want)
			}
		})
	}
}

func TestGenerateRulesetNoRules(t *testing.T) {
	table := "alca-abc123def456"
	containerIP := "172.17.0.2"

	ruleset := generateRuleset(table, containerIP, nil)

	// Verify idempotent header (shebang and delete pattern)
	if !strings.Contains(ruleset, "#!/usr/sbin/nft -f") {
		t.Error("ruleset should start with nft shebang")
	}
	if !strings.Contains(ruleset, "table inet alca-abc123def456\ndelete table inet alca-abc123def456") {
		t.Error("ruleset should contain idempotent delete pattern")
	}

	// Verify table declaration
	if !strings.Contains(ruleset, "table inet alca-abc123def456 {") {
		t.Error("ruleset should contain table declaration with correct name")
	}

	// Verify chain declaration with correct priority
	if !strings.Contains(ruleset, "type filter hook forward priority filter - 1") {
		t.Error("ruleset should contain forward chain with priority filter - 1")
	}

	// Verify established/related connections are allowed
	if !strings.Contains(ruleset, "ct state established,related accept") {
		t.Error("ruleset should allow established/related connections")
	}

	// Verify all RFC1918 ranges are blocked
	expectedRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"127.0.0.0/8",
	}

	for _, cidr := range expectedRanges {
		expected := "ip saddr " + containerIP + " ip daddr " + cidr + " drop"
		if !strings.Contains(ruleset, expected) {
			t.Errorf("ruleset should block %s, expected: %s", cidr, expected)
		}
	}

	// Verify container IP is used as source address
	if strings.Count(ruleset, "ip saddr "+containerIP) != 5 {
		t.Error("ruleset should use container IP as source address for all 5 drop rules")
	}
}

func TestGenerateRulesetWithAllowRules(t *testing.T) {
	table := "alca-test"
	containerIP := "172.17.0.2"

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP, IsIPv6: false},
		{IP: "192.168.1.50", Port: 53, Protocol: shared.ProtoUDP, IsIPv6: false},
		{IP: "10.0.0.0/8", Port: 0, Protocol: shared.ProtoAll, IsIPv6: false},
	}

	ruleset := generateRuleset(table, containerIP, rules)

	// Verify allow rules are present
	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip daddr 192.168.1.100 tcp dport 8080 accept") {
		t.Error("ruleset should contain TCP allow rule for 192.168.1.100:8080")
	}

	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip daddr 192.168.1.50 udp dport 53 accept") {
		t.Error("ruleset should contain UDP allow rule for 192.168.1.50:53")
	}

	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip daddr 10.0.0.0/8 accept") {
		t.Error("ruleset should contain allow rule for entire 10.0.0.0/8 subnet")
	}

	// Verify block rules still present after allow rules
	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip daddr 10.0.0.0/8 drop") {
		t.Error("ruleset should still block 10.0.0.0/8 after allow rules")
	}

	// Verify allow rules come before block rules
	allowPos := strings.Index(ruleset, "192.168.1.100 tcp dport 8080 accept")
	blockPos := strings.Index(ruleset, "10.0.0.0/8 drop")
	if allowPos > blockPos {
		t.Error("allow rules should come before block rules")
	}
}

func TestGenerateRulesetIPv6Container(t *testing.T) {
	table := "alca-test"
	containerIP := "2001:db8::2"

	ruleset := generateRuleset(table, containerIP, nil)

	// Verify IPv6 private ranges are blocked
	if !strings.Contains(ruleset, "ip6 saddr 2001:db8::2 ip6 daddr fe80::/10 drop") {
		t.Error("ruleset should block IPv6 link-local range")
	}
	if !strings.Contains(ruleset, "ip6 saddr 2001:db8::2 ip6 daddr fc00::/7 drop") {
		t.Error("ruleset should block IPv6 ULA range")
	}
	if !strings.Contains(ruleset, "ip6 saddr 2001:db8::2 ip6 daddr ::1/128 drop") {
		t.Error("ruleset should block IPv6 loopback")
	}

	// Verify IPv4 ranges are NOT blocked for IPv6 container
	if strings.Contains(ruleset, "ip saddr") {
		t.Error("ruleset should not contain IPv4 rules for IPv6 container")
	}
}

func TestGenerateRulesetProtocolVariants(t *testing.T) {
	table := "alca-test"
	containerIP := "172.17.0.2"

	tests := []struct {
		name     string
		rule     shared.LANAccessRule
		expected []string
	}{
		{
			name: "TCP all ports",
			rule: shared.LANAccessRule{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoTCP, IsIPv6: false},
			expected: []string{
				"ip saddr 172.17.0.2 ip daddr 192.168.1.100 tcp dport 1-65535 accept",
			},
		},
		{
			name: "UDP all ports",
			rule: shared.LANAccessRule{IP: "192.168.1.100", Port: 0, Protocol: shared.ProtoUDP, IsIPv6: false},
			expected: []string{
				"ip saddr 172.17.0.2 ip daddr 192.168.1.100 udp dport 1-65535 accept",
			},
		},
		{
			name: "Both protocols with specific port",
			rule: shared.LANAccessRule{IP: "192.168.1.100", Port: 443, Protocol: shared.ProtoAll, IsIPv6: false},
			expected: []string{
				"ip saddr 172.17.0.2 ip daddr 192.168.1.100 tcp dport 443 accept",
				"ip saddr 172.17.0.2 ip daddr 192.168.1.100 udp dport 443 accept",
			},
		},
		{
			name: "All protocols all ports (no port, no proto restriction)",
			rule: shared.LANAccessRule{IP: "10.0.0.0/8", Port: 0, Protocol: shared.ProtoAll, IsIPv6: false},
			expected: []string{
				"ip saddr 172.17.0.2 ip daddr 10.0.0.0/8 accept",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleset := generateRuleset(table, containerIP, []shared.LANAccessRule{tt.rule})

			for _, exp := range tt.expected {
				if !strings.Contains(ruleset, exp) {
					t.Errorf("ruleset should contain %q\nGot:\n%s", exp, ruleset)
				}
			}
		})
	}
}

func TestGenerateRulesetSkipsAllLANRule(t *testing.T) {
	table := "alca-test"
	containerIP := "172.17.0.2"

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP, IsIPv6: false},
		{AllLAN: true}, // This should be skipped in rule generation
		{IP: "10.0.0.1", Port: 443, Protocol: shared.ProtoTCP, IsIPv6: false},
	}

	ruleset := generateRuleset(table, containerIP, rules)

	// Verify normal rules are present
	if !strings.Contains(ruleset, "192.168.1.100 tcp dport 8080 accept") {
		t.Error("ruleset should contain first allow rule")
	}
	if !strings.Contains(ruleset, "10.0.0.1 tcp dport 443 accept") {
		t.Error("ruleset should contain third allow rule")
	}

	// AllLAN rule should not generate any specific allow line
	// (it's handled at ApplyRules level by returning early)
}

func TestGenerateRulesetIPv6AllowRule(t *testing.T) {
	table := "alca-test"
	containerIP := "2001:db8::2"

	rules := []shared.LANAccessRule{
		{IP: "fe80::1", Port: 8080, Protocol: shared.ProtoTCP, IsIPv6: true},
	}

	ruleset := generateRuleset(table, containerIP, rules)

	// IPv6 container to IPv6 destination
	if !strings.Contains(ruleset, "ip6 saddr 2001:db8::2 ip6 daddr fe80::1 tcp dport 8080 accept") {
		t.Errorf("ruleset should contain IPv6 allow rule\nGot:\n%s", ruleset)
	}
}

func TestGenerateRulesetMixedIPVersionAllowRules(t *testing.T) {
	table := "alca-test"
	containerIP := "172.17.0.2" // IPv4 container

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP, IsIPv6: false},
		{IP: "fe80::1", Port: 443, Protocol: shared.ProtoTCP, IsIPv6: true},
	}

	ruleset := generateRuleset(table, containerIP, rules)

	// IPv4 container to IPv4 destination
	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip daddr 192.168.1.100 tcp dport 8080 accept") {
		t.Errorf("ruleset should contain IPv4->IPv4 allow rule\nGot:\n%s", ruleset)
	}

	// IPv4 container to IPv6 destination (cross-family)
	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip6 daddr fe80::1 tcp dport 443 accept") {
		t.Errorf("ruleset should contain IPv4->IPv6 allow rule\nGot:\n%s", ruleset)
	}
}

func TestRuleFilePath(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		want        string
	}{
		{
			name:        "short container ID",
			containerID: "abc123",
			want:        "/etc/nftables.d/alcatraz/abc123.nft",
		},
		{
			name:        "full container ID",
			containerID: "abc123def456789xyz",
			want:        "/etc/nftables.d/alcatraz/abc123def456789xyz.nft",
		},
		{
			name:        "empty container ID",
			containerID: "",
			want:        "/etc/nftables.d/alcatraz/.nft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ruleFilePath(tt.containerID)
			if got != tt.want {
				t.Errorf("ruleFilePath(%q) = %q, want %q", tt.containerID, got, tt.want)
			}
		})
	}
}
