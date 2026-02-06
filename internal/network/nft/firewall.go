//go:build linux

package nft

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// Constants for nftables persistence paths.
const (
	nftablesConfPath    = "/etc/nftables.conf"
	alcatrazNftDir      = "/etc/nftables.d/alcatraz"
	alcatrazIncludeLine = `include "/etc/nftables.d/alcatraz/*.nft"`
)

// Compile-time interface assertion.
var _ shared.Firewall = (*NFTables)(nil)

// NFTables implements shared.Firewall using Linux nftables.
// Each container gets its own table for isolation and clean teardown.
type NFTables struct {
	env *shared.NetworkEnv
}

// tableName returns the nftables table name for a container.
// Uses short container ID prefix to keep names manageable.
func tableName(containerID string) string {
	return "alca-" + shared.ShortContainerID(containerID)
}

// generateRuleset generates the nftables ruleset for a container with allow rules.
// If rules is empty, blocks all RFC1918 traffic.
// Uses idempotent flush+recreate pattern per AGD-028.
// This is a pure function for testability.
func generateRuleset(tableName string, containerIP string, rules []shared.LANAccessRule) string {
	var sb strings.Builder

	// Determine if container IP is IPv6
	containerIsV6 := shared.IsIPv6(containerIP)

	// Idempotent header: create empty table then delete it (ensures table exists for delete)
	sb.WriteString("#!/usr/sbin/nft -f\n")
	sb.WriteString(fmt.Sprintf("# Alcatraz container rules for table: %s\n\n", tableName))
	sb.WriteString("# Delete table if exists (idempotent)\n")
	sb.WriteString(fmt.Sprintf("table inet %s\n", tableName))
	sb.WriteString(fmt.Sprintf("delete table inet %s\n\n", tableName))

	// Create fresh table with rules
	sb.WriteString("# Create fresh table with rules\n")
	sb.WriteString(fmt.Sprintf("table inet %s {\n", tableName))
	sb.WriteString("\tchain forward {\n")
	sb.WriteString("\t\ttype filter hook forward priority filter - 1; policy accept;\n\n")
	sb.WriteString("\t\t# Allow established/related connections (return traffic)\n")
	sb.WriteString("\t\tct state established,related accept\n\n")

	// Generate allow rules for each lan-access entry
	if len(rules) > 0 {
		sb.WriteString("\t\t# Allow rules from lan-access configuration\n")
		for _, rule := range rules {
			if rule.AllLAN {
				continue // AllLAN means no blocking needed
			}
			writeNftAllowRule(&sb, containerIP, containerIsV6, rule)
		}
		sb.WriteString("\n")
	}

	// Block RFC1918 and private ranges
	sb.WriteString("\t\t# Block RFC1918 and other private ranges from container\n")
	if containerIsV6 {
		for _, cidr := range shared.PrivateIPv6Ranges {
			fmt.Fprintf(&sb, "\t\tip6 saddr %s ip6 daddr %s drop\n", containerIP, cidr)
		}
	} else {
		for _, cidr := range shared.PrivateIPv4Ranges {
			fmt.Fprintf(&sb, "\t\tip saddr %s ip daddr %s drop\n", containerIP, cidr)
		}
	}

	sb.WriteString("\t}\n}\n")
	return sb.String()
}

// writeNftAllowRule writes an nftables accept rule for a LANAccessRule.
func writeNftAllowRule(sb *strings.Builder, containerIP string, containerIsV6 bool, rule shared.LANAccessRule) {
	// Determine IP command based on source (container) and destination (rule)
	srcIPCmd := "ip"
	if containerIsV6 {
		srcIPCmd = "ip6"
	}
	dstIPCmd := "ip"
	if rule.IsIPv6 {
		dstIPCmd = "ip6"
	}

	base := fmt.Sprintf("\t\t%s saddr %s %s daddr %s", srcIPCmd, containerIP, dstIPCmd, rule.IP)

	switch {
	case rule.Port == 0 && rule.Protocol == shared.ProtoAll:
		// All ports, all protocols
		sb.WriteString(base + " accept\n")
	case rule.Port == 0 && rule.Protocol == shared.ProtoTCP:
		// All TCP ports
		sb.WriteString(base + " tcp dport 1-65535 accept\n")
	case rule.Port == 0 && rule.Protocol == shared.ProtoUDP:
		// All UDP ports
		sb.WriteString(base + " udp dport 1-65535 accept\n")
	case rule.Port > 0 && rule.Protocol == shared.ProtoTCP:
		sb.WriteString(base + fmt.Sprintf(" tcp dport %d accept\n", rule.Port))
	case rule.Port > 0 && rule.Protocol == shared.ProtoUDP:
		sb.WriteString(base + fmt.Sprintf(" udp dport %d accept\n", rule.Port))
	case rule.Port > 0 && rule.Protocol == shared.ProtoAll:
		// Both TCP and UDP for specific port
		sb.WriteString(base + fmt.Sprintf(" tcp dport %d accept\n", rule.Port))
		sb.WriteString(base + fmt.Sprintf(" udp dport %d accept\n", rule.Port))
	}
}

// ruleFilePath returns the persistent rule file path for a container.
func ruleFilePath(containerID string) string {
	return filepath.Join(alcatrazNftDir, containerID+".nft")
}

// ApplyRules creates nftables rules to apply network isolation with allow rules.
// Rules are persisted to /etc/nftables.d/alcatraz/<container-id>.nft for reload support.
// Rules are loaded atomically via `nft -f` to satisfy AGD-008 crash safety requirements.
func (n *NFTables) ApplyRules(containerID string, containerIP string, rules []shared.LANAccessRule) error {
	// If any rule allows all LAN, skip firewall entirely
	if shared.HasAllLAN(rules) {
		return nil
	}

	table := tableName(containerID)
	ruleset := generateRuleset(table, containerIP, rules)
	fs := n.env.Fs

	// Ensure alcatraz nft directory exists
	if err := fs.MkdirAll(alcatrazNftDir, 0755); err != nil {
		return fmt.Errorf("failed to create nftables directory %s: %w", alcatrazNftDir, err)
	}

	// Write to persistent file location
	rulePath := ruleFilePath(containerID)
	if err := afero.WriteFile(fs, rulePath, []byte(ruleset), 0644); err != nil {
		return fmt.Errorf("failed to write ruleset to %s: %w", rulePath, err)
	}

	// Load ruleset atomically (idempotent format handles existing table)
	// Requires sudo for nftables access
	output, err := n.env.Cmd.SudoRunQuiet("nft", "-f", rulePath)
	if err != nil {
		return fmt.Errorf("failed to load nftables rules for table %s: %w: %s", table, err, strings.TrimSpace(string(output)))
	}

	return nil
}

// Cleanup removes all firewall rules for a container.
// Deletes both the persistent rule file and the nftables table.
func (n *NFTables) Cleanup(containerID string) error {
	// Delete rule file (best-effort, ignore if not exists)
	rulePath := ruleFilePath(containerID)
	_ = n.env.Fs.Remove(rulePath)

	// Delete nftables table
	table := tableName(containerID)
	return n.deleteTable(table)
}

// deleteTable removes an nftables table. Returns nil if table doesn't exist.
func (n *NFTables) deleteTable(table string) error {
	// Requires sudo for nftables access
	output, err := n.env.Cmd.SudoRunQuiet("nft", "delete", "table", "inet", table)
	if err != nil {
		// Table doesn't exist — not an error during cleanup.
		// Check both command output and error message for the kernel error string.
		combined := string(output) + " " + err.Error()
		if strings.Contains(combined, "No such file or directory") {
			return nil
		}
		return fmt.Errorf("failed to delete table %s: %w: %s", table, err, strings.TrimSpace(string(output)))
	}
	return nil
}
