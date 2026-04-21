package nft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

func TestProxyTableName(t *testing.T) {
	assert.Equal(t, "alca-proxy-abcdef012345", proxyTableName("abcdef0123456789"))
	assert.Equal(t, "alca-proxy-short", proxyTableName("short"))
}

func TestProxyTableFromIsolationTable(t *testing.T) {
	assert.Equal(t, "alca-proxy-abc123", proxyTableFromIsolationTable("alca-abc123"))
	assert.Equal(t, "alca-proxy-short", proxyTableFromIsolationTable("alca-short"))
	assert.Equal(t, "", proxyTableFromIsolationTable("other-prefix"))
	assert.Equal(t, "", proxyTableFromIsolationTable(""))

	// Edge case: bare prefix "alca-" with no ID suffix.
	// This can occur when tableName() is called with an empty container ID.
	// Returns "alca-proxy-" — the empty suffix is preserved.
	assert.Equal(t, "alca-proxy-", proxyTableFromIsolationTable("alca-"))
}

func TestGenerateRuleset_WithProxy(t *testing.T) {
	proxy := &shared.ProxyConfig{Host: "172.17.0.1", Port: 1080}
	ruleset := generateRuleset(
		"alca-abc123",
		"172.17.0.2",
		nil,
		proxy, false,
		"filter - 1",
		"/home/user/project",
		"test-project-id",
	)

	// Verify isolation table (inet) is present
	assert.Contains(t, ruleset, "table inet alca-abc123")

	// Verify proxy table uses "ip" family (not "inet") — required for DNAT
	assert.Contains(t, ruleset, "table ip alca-proxy-abc123")
	assert.Contains(t, ruleset, "delete table ip alca-proxy-abc123")

	// Filter-table allow rules stay in place for both TCP and UDP so the
	// container can still reach the proxy address directly (e.g., a UDP DNS
	// server living on the same host), even though only TCP is DNAT'd.
	assert.Contains(t, ruleset, "ip saddr 172.17.0.2 ip daddr 172.17.0.1 tcp dport 1080 accept")
	assert.Contains(t, ruleset, "ip saddr 172.17.0.2 ip daddr 172.17.0.1 udp dport 1080 accept")

	// TCP DNAT rule + its loop-prevention rule are present.
	assert.Contains(t, ruleset, "ip saddr 172.17.0.2 tcp dport 1-65535 dnat to 172.17.0.1:1080")

	// UDP must NOT be DNAT'd — AGD-037 scopes the transparent proxy to TCP
	// because UDP has no working transparent-proxy path under container
	// networking today, so UDP egresses the container's normal way.
	assert.NotContains(t, ruleset, "udp dport 1-65535 dnat to")
	assert.NotContains(t, ruleset, "ct timeout proxy-udp-timeout")
	assert.NotContains(t, ruleset, `ct timeout set "proxy-udp-timeout"`)

	// Verify proxy chain uses nat hook with dstnat priority
	assert.Contains(t, ruleset, "type nat hook prerouting priority dstnat - 1")
}

func TestGenerateRuleset_WithoutProxy(t *testing.T) {
	ruleset := generateRuleset(
		"alca-abc123",
		"172.17.0.2",
		nil,
		nil, false,
		"filter - 1",
		"/test",
		"id",
	)

	// Isolation table should be present
	assert.Contains(t, ruleset, "table inet alca-abc123")

	// Proxy table deletion should NOT be present when proxy is nil
	assert.NotContains(t, ruleset, "delete table ip alca-proxy-abc123")

	// No proxy DNAT rules when no proxy is configured
	assert.NotContains(t, ruleset, "dnat to")
	assert.NotContains(t, ruleset, "proxy-udp-timeout")
	assert.NotContains(t, ruleset, "table ip alca-proxy-abc123 {")
}

func TestGenerateRuleset_WithProxyIPv6ContainerNoProxyDNAT(t *testing.T) {
	// IPv6 container IPs are a known limitation for transparent proxy.
	// The proxy DNAT table uses "ip" family (IPv4 only), so DNAT rules
	// won't match IPv6 traffic. The template still renders the proxy table
	// but the "ip saddr" rules won't match an IPv6 source address.
	proxy := &shared.ProxyConfig{Host: "172.17.0.1", Port: 1080}
	ruleset := generateRuleset(
		"alca-v6test",
		"2001:db8::2",
		nil,
		proxy, false,
		"filter - 1",
		"/home/user/project",
		"test-project-id",
	)

	// Proxy table is still rendered (ip family)
	assert.Contains(t, ruleset, "table ip alca-proxy-v6test")

	// DNAT rules reference the IPv6 address in an "ip saddr" context,
	// which will never match — documenting this as expected (known limitation)
	assert.Contains(t, ruleset, "ip saddr 2001:db8::2")
	assert.Contains(t, ruleset, "dnat to 172.17.0.1:1080")
}

// Test: proxy address is auto-allowed in the inet filter table (not just the ip nat table)
func TestGenerateRuleset_WithProxy_AutoAllowsProxyInFilterTable(t *testing.T) {
	proxy := &shared.ProxyConfig{Host: "192.168.1.100", Port: 1080}
	ruleset := generateRuleset(
		"alca-test",
		"172.17.0.2",
		nil,
		proxy, false,
		"filter - 1",
		"/test",
		"id",
	)

	// Split the ruleset at the proxy table boundary.
	// The inet filter table must contain an allow rule for the proxy address
	// BEFORE the RFC1918 block rules, so the container can reach a LAN proxy.
	parts := strings.SplitN(ruleset, "# Transparent TCP proxy DNAT rules", 2)
	require.Len(t, parts, 2, "ruleset should contain both filter and proxy sections")

	filterSection := parts[0] // everything before the proxy nat table
	assert.Contains(t, filterSection, "ip saddr 172.17.0.2 ip daddr 192.168.1.100 tcp dport 1080 accept")
	assert.Contains(t, filterSection, "ip saddr 172.17.0.2 ip daddr 192.168.1.100 udp dport 1080 accept")
}

func TestGenerateRuleset_WithProxyAndAllLAN(t *testing.T) {
	proxy := &shared.ProxyConfig{Host: "172.17.0.1", Port: 1080}
	rules := []shared.LANAccessRule{
		{AllLAN: true},
	}
	ruleset := generateRuleset(
		"alca-abc123",
		"172.17.0.2",
		rules,
		proxy, true,
		"filter - 1",
		"/home/user/project",
		"test-project-id",
	)

	// (a) Block rules should NOT be present (SkipBlock=true when allLAN=true)
	assert.NotContains(t, ruleset, "10.0.0.0/8 drop")
	assert.NotContains(t, ruleset, "172.16.0.0/12 drop")
	assert.NotContains(t, ruleset, "192.168.0.0/16 drop")

	// (b) Proxy TCP DNAT rule IS present; UDP is not DNAT'd (AGD-037: TCP-only).
	assert.Contains(t, ruleset, "table ip alca-proxy-abc123")
	assert.Contains(t, ruleset, "ip saddr 172.17.0.2 tcp dport 1-65535 dnat to 172.17.0.1:1080")
	assert.NotContains(t, ruleset, "udp dport 1-65535 dnat to")

	// (c) Proxy allow rule in filter table IS present
	assert.Contains(t, ruleset, "ip saddr 172.17.0.2 ip daddr 172.17.0.1 tcp dport 1080 accept")
	assert.Contains(t, ruleset, "ip saddr 172.17.0.2 ip daddr 172.17.0.1 udp dport 1080 accept")
}

func TestGenerateRuleset_WithProxyAndRules(t *testing.T) {
	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP},
	}
	proxy := &shared.ProxyConfig{Host: "10.0.0.1", Port: 3128}

	ruleset := generateRuleset(
		"alca-test",
		"172.17.0.2",
		rules,
		proxy, false,
		"filter - 1",
		"/test",
		"id",
	)

	// Both isolation and proxy should be present
	assert.Contains(t, ruleset, "table inet alca-test")
	assert.Contains(t, ruleset, "table ip alca-proxy-test")
	assert.Contains(t, ruleset, "192.168.1.100")
	assert.Contains(t, ruleset, "dnat to 10.0.0.1:3128")
}
