package nft

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// rulesetData holds all data needed to render the nftables ruleset template.
type rulesetData struct {
	TableName   string
	ProxyTable  string
	ContainerIP string
	Priority    string
	ProjectDir  string
	ProjectID   string
	AllowRules  string // Pre-rendered allow rules (complex per-rule logic)
	BlockRules  string // Pre-rendered block rules (IPv4 vs IPv6 ranges)
	SkipBlock   bool   // True when AllLAN — skip block rules to honor user intent
	Proxy       *shared.ProxyConfig
	ProxyAddr   string // "host:port" for DNAT target
}

var rulesetTmpl = template.Must(template.New("ruleset").Parse(`#!/usr/sbin/nft -f
# Alcatraz container rules for table: {{.TableName}}

# Delete table if exists (idempotent)
table inet {{.TableName}}
delete table inet {{.TableName}}

{{- if .Proxy}}
# Delete proxy table if exists (idempotent)
table ip {{.ProxyTable}}
delete table ip {{.ProxyTable}}
{{- end}}

# project-dir: {{.ProjectDir}}
# project-id: {{.ProjectID}}

# Create fresh table with rules
table inet {{.TableName}} {
	chain forward {
		type filter hook forward priority {{.Priority}}; policy accept;

		# Allow established/related connections (return traffic)
		ct state established,related accept

{{.AllowRules}}{{- if .Proxy}}		# Allow traffic to proxy address (auto-injected, AGD-037)
		ip saddr {{.ContainerIP}} ip daddr {{.Proxy.Host}} tcp dport {{.Proxy.Port}} accept
		ip saddr {{.ContainerIP}} ip daddr {{.Proxy.Host}} udp dport {{.Proxy.Port}} accept

{{end}}{{- if not .SkipBlock}}		# Block RFC1918 and other private ranges from container
{{.BlockRules}}{{- end}}
	}
}
{{- if .Proxy}}

# Transparent TCP proxy DNAT rules (AGD-037).
#
# TCP only: DNAT to the proxy; the proxy recovers the original destination via
# SO_ORIGINAL_DST (conntrack lookup). UDP is intentionally NOT DNAT'd — see
# AGD-037's "Why only TCP" for why transparent UDP proxying of container
# traffic has no working path on Linux today (bridged packets + TPROXY, and
# IP_RECVORIGDSTADDR returning the post-DNAT address, both block it).
#
# NOTE: this table uses the "ip" family (IPv4 only). IPv6 container IPs are not
# supported for transparent proxy.
table ip {{.ProxyTable}} {
	chain prerouting {
		# Priority dstnat - 1 (-101) to run BEFORE Docker's iptables PREROUTING (-100).
		# Docker defaults to iptables for networking on most distros. NAT rules are
		# only evaluated on the first packet of each flow — once a NAT binding is set
		# (even "no NAT" via nf_nat_alloc_null_binding), subsequent chains are skipped.
		# If Docker's chain runs first without DNAT, our rules are never evaluated.
		#
		# References:
		#   NAT first-packet semantics: https://wiki.nftables.org/wiki-nftables/index.php/Performing_Network_Address_Translation_(NAT)
		#   null_binding source: https://github.com/torvalds/linux/blob/master/net/netfilter/nf_nat_core.c
		type nat hook prerouting priority dstnat - 1; policy accept;

		# Loop prevention MUST come before the DNAT wildcard rule — traffic to the
		# proxy's own TCP port otherwise matches the wildcard and redirects to itself.
		ip saddr {{.ContainerIP}} ip daddr {{.Proxy.Host}} tcp dport {{.Proxy.Port}} accept

		# DNAT all outbound TCP to the proxy.
		ip saddr {{.ContainerIP}} tcp dport 1-65535 dnat to {{.ProxyAddr}}
	}
}
{{- end}}
`))

// renderAllowRules pre-renders the allow rules section.
func renderAllowRules(containerIP string, containerIsV6 bool, rules []shared.LANAccessRule) string {
	if len(rules) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\t\t# Allow rules from lan-access configuration\n")
	for _, rule := range rules {
		if rule.AllLAN {
			continue
		}
		writeNftAllowRule(&sb, containerIP, containerIsV6, rule)
	}
	sb.WriteString("\n")
	return sb.String()
}

// renderBlockRules pre-renders the RFC1918/private range block rules.
func renderBlockRules(containerIP string, containerIsV6 bool) string {
	var sb strings.Builder
	if containerIsV6 {
		for _, cidr := range shared.PrivateIPv6Ranges {
			fmt.Fprintf(&sb, "\t\tip6 saddr %s ip6 daddr %s drop\n", containerIP, cidr)
		}
	} else {
		for _, cidr := range shared.PrivateIPv4Ranges {
			fmt.Fprintf(&sb, "\t\tip saddr %s ip daddr %s drop\n", containerIP, cidr)
		}
	}
	return sb.String()
}

// generateRuleset generates the nftables ruleset using the template.
// Includes isolation rules (inet filter table) and optional proxy DNAT rules (ip nat table).
// Uses idempotent flush+recreate pattern per AGD-028.
// allLAN=true skips RFC1918 block rules (user explicitly allows all LAN access).
func generateRuleset(tableName string, containerIP string, rules []shared.LANAccessRule, proxy *shared.ProxyConfig, allLAN bool, priority string, projectDir string, projectID string) string {
	containerIsV6 := shared.IsIPv6(containerIP)

	data := rulesetData{
		TableName:   tableName,
		ProxyTable:  proxyTableFromIsolationTable(tableName),
		ContainerIP: containerIP,
		Priority:    priority,
		ProjectDir:  projectDir,
		ProjectID:   projectID,
		AllowRules:  renderAllowRules(containerIP, containerIsV6, rules),
		BlockRules:  renderBlockRules(containerIP, containerIsV6),
		SkipBlock:   allLAN,
		Proxy:       proxy,
	}
	if proxy != nil {
		data.ProxyAddr = fmt.Sprintf("%s:%d", proxy.Host, proxy.Port)
	}

	var buf bytes.Buffer
	if err := rulesetTmpl.Execute(&buf, data); err != nil {
		// Template is compile-time validated, this should never happen
		panic(fmt.Sprintf("ruleset template execution failed: %v", err))
	}
	return buf.String()
}
