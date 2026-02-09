package nft

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
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

	ruleset := generateRuleset(table, containerIP, nil, "filter - 1")

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

	ruleset := generateRuleset(table, containerIP, rules, "filter - 1")

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

	ruleset := generateRuleset(table, containerIP, nil, "filter - 1")

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
			ruleset := generateRuleset(table, containerIP, []shared.LANAccessRule{tt.rule}, "filter - 1")

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

	ruleset := generateRuleset(table, containerIP, rules, "filter - 1")

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

	ruleset := generateRuleset(table, containerIP, rules, "filter - 1")

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

	ruleset := generateRuleset(table, containerIP, rules, "filter - 1")

	// IPv4 container to IPv4 destination
	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip daddr 192.168.1.100 tcp dport 8080 accept") {
		t.Errorf("ruleset should contain IPv4->IPv4 allow rule\nGot:\n%s", ruleset)
	}

	// IPv4 container to IPv6 destination (cross-family)
	if !strings.Contains(ruleset, "ip saddr 172.17.0.2 ip6 daddr fe80::1 tcp dport 443 accept") {
		t.Errorf("ruleset should contain IPv4->IPv6 allow rule\nGot:\n%s", ruleset)
	}
}

func TestIsDarwin_Linux(t *testing.T) {
	env := shared.NewNetworkEnv(
		afero.NewMemMapFs(),
		util.NewMockCommandRunner(),
		"/test",
		runtime.PlatformLinux,
	)
	n := New(env).(*NFTables)
	if n.isDarwin() {
		t.Error("isDarwin() should return false for PlatformLinux")
	}
}

func TestIsDarwin_MacOrbStack(t *testing.T) {
	env := shared.NewNetworkEnv(
		afero.NewMemMapFs(),
		util.NewMockCommandRunner(),
		"/test",
		runtime.PlatformMacOrbStack,
	)
	n := New(env).(*NFTables)
	if !n.isDarwin() {
		t.Error("isDarwin() should return true for PlatformMacOrbStack")
	}
}

func TestIsDarwin_MacDockerDesktop(t *testing.T) {
	env := shared.NewNetworkEnv(
		afero.NewMemMapFs(),
		util.NewMockCommandRunner(),
		"/test",
		runtime.PlatformMacDockerDesktop,
	)
	n := New(env).(*NFTables)
	if !n.isDarwin() {
		t.Error("isDarwin() should return true for PlatformMacDockerDesktop")
	}
}

func TestNftDir(t *testing.T) {
	t.Run("Linux", func(t *testing.T) {
		got := nftDirOnLinux()
		want := "/etc/nftables.d/alcatraz"
		if got != want {
			t.Errorf("nftDirOnLinux() = %q, want %q", got, want)
		}
	})
}

// =============================================================================
// writeRuleFile tests
// =============================================================================

func TestWriteRuleFile_CreatesFileWithContent(t *testing.T) {
	fs := afero.NewMemMapFs()
	dir := "/etc/nftables.d/alcatraz"
	content := "#!/usr/sbin/nft -f\ntable inet alca-test {}\n"

	path, err := writeRuleFile(fs, dir, "test.nft", content)
	if err != nil {
		t.Fatalf("writeRuleFile() error = %v", err)
	}

	wantPath := "/etc/nftables.d/alcatraz/test.nft"
	if path != wantPath {
		t.Errorf("writeRuleFile() path = %q, want %q", path, wantPath)
	}

	got, err := afero.ReadFile(fs, path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
}

func TestWriteRuleFile_CreatesDirectoryIfMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	dir := "/some/deep/nested/dir"

	exists, _ := afero.DirExists(fs, dir)
	if exists {
		t.Fatal("directory should not exist before writeRuleFile")
	}

	_, err := writeRuleFile(fs, dir, "rule.nft", "content")
	if err != nil {
		t.Fatalf("writeRuleFile() error = %v", err)
	}

	exists, _ = afero.DirExists(fs, dir)
	if !exists {
		t.Error("writeRuleFile should create the directory if missing")
	}
}

func TestWriteRuleFile_OverwritesExistingFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	dir := "/etc/nftables.d/alcatraz"
	_ = fs.MkdirAll(dir, 0755)
	_ = afero.WriteFile(fs, dir+"/test.nft", []byte("old content"), 0644)

	newContent := "new content"
	_, err := writeRuleFile(fs, dir, "test.nft", newContent)
	if err != nil {
		t.Fatalf("writeRuleFile() error = %v", err)
	}

	got, _ := afero.ReadFile(fs, dir+"/test.nft")
	if string(got) != newContent {
		t.Errorf("file content = %q, want %q", string(got), newContent)
	}
}

func TestWriteRuleFile_ErrorOnReadOnlyFs(t *testing.T) {
	baseFs := afero.NewMemMapFs()
	readOnlyFs := afero.NewReadOnlyFs(baseFs)

	_, err := writeRuleFile(readOnlyFs, "/dir", "file.nft", "content")
	if err == nil {
		t.Error("writeRuleFile() should return error on read-only filesystem")
	}
}

// =============================================================================
// formatProtocolSuffixes tests
// =============================================================================

func TestFormatProtocolSuffixes(t *testing.T) {
	tests := []struct {
		name  string
		proto shared.Protocol
		port  int
		want  []string
	}{
		{
			name:  "ProtoAll with no port — wildcard allow",
			proto: shared.ProtoAll,
			port:  0,
			want:  []string{""},
		},
		{
			name:  "TCP with specific port",
			proto: shared.ProtoTCP,
			port:  8080,
			want:  []string{" tcp dport 8080"},
		},
		{
			name:  "UDP with specific port",
			proto: shared.ProtoUDP,
			port:  53,
			want:  []string{" udp dport 53"},
		},
		{
			name:  "TCP with no port — all TCP ports",
			proto: shared.ProtoTCP,
			port:  0,
			want:  []string{" tcp dport 1-65535"},
		},
		{
			name:  "UDP with no port — all UDP ports",
			proto: shared.ProtoUDP,
			port:  0,
			want:  []string{" udp dport 1-65535"},
		},
		{
			name:  "ProtoAll with specific port — expands to TCP and UDP",
			proto: shared.ProtoAll,
			port:  443,
			want: []string{
				" tcp dport 443",
				" udp dport 443",
			},
		},
		{
			name:  "port 1 edge case",
			proto: shared.ProtoTCP,
			port:  1,
			want:  []string{" tcp dport 1"},
		},
		{
			name:  "port 65535 edge case",
			proto: shared.ProtoUDP,
			port:  65535,
			want:  []string{fmt.Sprintf(" udp dport %d", 65535)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatProtocolSuffixes(tt.proto, tt.port)
			if len(got) != len(tt.want) {
				t.Fatalf("formatProtocolSuffixes(%v, %d) returned %d suffixes, want %d: %v",
					tt.proto, tt.port, len(got), len(tt.want), got)
			}
			for i, s := range got {
				if s != tt.want[i] {
					t.Errorf("suffix[%d] = %q, want %q", i, s, tt.want[i])
				}
			}
		})
	}
}

func TestFormatProtocolSuffixes_UnknownProtocol(t *testing.T) {
	// An unknown protocol value (not ProtoAll/TCP/UDP) with port=0 should return nil
	got := formatProtocolSuffixes(shared.Protocol(99), 0)
	if got != nil {
		t.Errorf("formatProtocolSuffixes with unknown protocol should return nil, got %v", got)
	}
}

// =============================================================================
// New() VMHelperEnv pre-construction tests
// =============================================================================

func TestNew_VMHelperEnvPreConstructedOnDarwin(t *testing.T) {
	platforms := []runtime.RuntimePlatform{
		runtime.PlatformMacOrbStack,
		runtime.PlatformMacDockerDesktop,
	}

	for _, platform := range platforms {
		t.Run(string(platform), func(t *testing.T) {
			env := shared.NewNetworkEnv(
				afero.NewMemMapFs(),
				util.NewMockCommandRunner(),
				"/test",
				platform,
			)
			n := New(env).(*NFTables)

			if n.vmEnv == nil {
				t.Errorf("New() with %s should pre-construct vmEnv, got nil", platform)
			}
		})
	}
}

func TestNew_VMHelperEnvNilOnLinux(t *testing.T) {
	env := shared.NewNetworkEnv(
		afero.NewMemMapFs(),
		util.NewMockCommandRunner(),
		"/test",
		runtime.PlatformLinux,
	)
	n := New(env).(*NFTables)

	if n.vmEnv != nil {
		t.Errorf("New() with PlatformLinux should not pre-construct vmEnv, got %v", n.vmEnv)
	}
}

func TestNew_VMHelperEnvNilForEmptyPlatform(t *testing.T) {
	env := shared.NewNetworkEnv(
		afero.NewMemMapFs(),
		util.NewMockCommandRunner(),
		"/test",
		"",
	)
	n := New(env).(*NFTables)

	if n.vmEnv != nil {
		t.Errorf("New() with empty platform should not pre-construct vmEnv, got %v", n.vmEnv)
	}
}
