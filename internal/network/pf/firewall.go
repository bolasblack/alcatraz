//go:build darwin

package pf

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// File prefixes ensure correct pf rule ordering when files are concatenated via cat *.
// pf requires: translation (nat) rules before filtering (pass/block) rules.
const (
	// natRulePrefix is the file prefix for NAT/translation rules.
	natRulePrefix = "0nat"
	// filterRulePrefix is the file prefix for filter (pass/block) rules.
	filterRulePrefix = "1filter"
)

// PF implements shared.Firewall using macOS pf (packet filter).
// Each container gets its own anchor under alcatraz/ for isolation and clean teardown.
type PF struct {
	env *shared.NetworkEnv
}

// Compile-time interface assertion.
var _ shared.Firewall = (*PF)(nil)

// generatePfRuleset generates pf filter rules for a container with allow rules.
// Allow rules come before block rules (first-match with "quick" keyword).
// This is a pure function for testability.
func generatePfRuleset(containerIP string, rules []shared.LANAccessRule) string {
	var sb strings.Builder

	containerIsV6 := shared.IsIPv6(containerIP)

	// Generate allow rules for each lan-access entry (BEFORE blocks)
	if len(rules) > 0 {
		hasAllowRules := false
		for _, rule := range rules {
			if rule.AllLAN {
				continue
			}
			if !hasAllowRules {
				sb.WriteString("# Allow specific lan-access entries\n")
				hasAllowRules = true
			}
			writePfAllowRule(&sb, containerIP, rule)
		}
		if hasAllowRules {
			sb.WriteString("\n")
		}
	}

	// Block RFC1918 and private ranges
	sb.WriteString("# Block RFC1918 and other private ranges\n")
	if containerIsV6 {
		for _, cidr := range shared.PrivateIPv6Ranges {
			fmt.Fprintf(&sb, "block drop quick from %s to %s\n", containerIP, cidr)
		}
	} else {
		for _, cidr := range shared.PrivateIPv4Ranges {
			fmt.Fprintf(&sb, "block drop quick from %s to %s\n", containerIP, cidr)
		}
	}

	return sb.String()
}

// writePfAllowRule writes a pf pass rule for a LANAccessRule.
// pf syntax: pass quick proto <proto> from <src> to <dst> port <port>
// Note: No 'in'/'out' direction - matches both directions like block rules.
// The proto keyword must come BEFORE from/to, not after port.
func writePfAllowRule(sb *strings.Builder, containerIP string, rule shared.LANAccessRule) {
	switch {
	case rule.Port == 0 && rule.Protocol == shared.ProtoAll:
		// No proto, no port: pass quick from X to Y
		fmt.Fprintf(sb, "pass quick from %s to %s\n", containerIP, rule.IP)
	case rule.Port == 0 && rule.Protocol == shared.ProtoTCP:
		// TCP all ports: pass quick proto tcp from X to Y
		fmt.Fprintf(sb, "pass quick proto tcp from %s to %s\n", containerIP, rule.IP)
	case rule.Port == 0 && rule.Protocol == shared.ProtoUDP:
		// UDP all ports: pass quick proto udp from X to Y
		fmt.Fprintf(sb, "pass quick proto udp from %s to %s\n", containerIP, rule.IP)
	case rule.Port > 0 && rule.Protocol == shared.ProtoTCP:
		// TCP specific port: pass quick proto tcp from X to Y port P
		fmt.Fprintf(sb, "pass quick proto tcp from %s to %s port %d\n", containerIP, rule.IP, rule.Port)
	case rule.Port > 0 && rule.Protocol == shared.ProtoUDP:
		// UDP specific port: pass quick proto udp from X to Y port P
		fmt.Fprintf(sb, "pass quick proto udp from %s to %s port %d\n", containerIP, rule.IP, rule.Port)
	case rule.Port > 0 && rule.Protocol == shared.ProtoAll:
		// Both protocols with specific port
		fmt.Fprintf(sb, "pass quick proto tcp from %s to %s port %d\n", containerIP, rule.IP, rule.Port)
		fmt.Fprintf(sb, "pass quick proto udp from %s to %s port %d\n", containerIP, rule.IP, rule.Port)
	}
}

// writeSelectiveNATRule writes a NAT rule for a specific whitelist entry.
// Used for OrbStack where we NAT only whitelisted destinations.
func writeSelectiveNATRule(sb *strings.Builder, iface, containerIP string, rule shared.LANAccessRule) {
	switch {
	case rule.Port == 0 && rule.Protocol == shared.ProtoAll:
		fmt.Fprintf(sb, "nat on %s from %s to %s -> (%s)\n", iface, containerIP, rule.IP, iface)
	case rule.Port == 0 && rule.Protocol == shared.ProtoTCP:
		fmt.Fprintf(sb, "nat on %s proto tcp from %s to %s -> (%s)\n", iface, containerIP, rule.IP, iface)
	case rule.Port == 0 && rule.Protocol == shared.ProtoUDP:
		fmt.Fprintf(sb, "nat on %s proto udp from %s to %s -> (%s)\n", iface, containerIP, rule.IP, iface)
	case rule.Port > 0 && rule.Protocol == shared.ProtoTCP:
		fmt.Fprintf(sb, "nat on %s proto tcp from %s to %s port %d -> (%s)\n", iface, containerIP, rule.IP, rule.Port, iface)
	case rule.Port > 0 && rule.Protocol == shared.ProtoUDP:
		fmt.Fprintf(sb, "nat on %s proto udp from %s to %s port %d -> (%s)\n", iface, containerIP, rule.IP, rule.Port, iface)
	case rule.Port > 0 && rule.Protocol == shared.ProtoAll:
		fmt.Fprintf(sb, "nat on %s proto tcp from %s to %s port %d -> (%s)\n", iface, containerIP, rule.IP, rule.Port, iface)
		fmt.Fprintf(sb, "nat on %s proto udp from %s to %s port %d -> (%s)\n", iface, containerIP, rule.IP, rule.Port, iface)
	}
}

// projectFileName converts a project path to a safe filename.
// Replaces "/" with "-" (e.g., "/Users/alice/project" becomes "-Users-alice-project").
// This matches the naming pattern used by NAT rules in nat.go.
func projectFileName(projectPath string) string {
	return strings.ReplaceAll(projectPath, "/", "-")
}

// natRuleFile returns the path for a project's NAT/translation rule file.
// Uses "0nat" prefix to sort before filter rules (e.g., "0nat-Users-alice-project").
func natRuleFile(projectDir string) string {
	return filepath.Join(pfAnchorDir, natRulePrefix+projectFileName(projectDir))
}

// filterRuleFile returns the path for a project's filter (pass/block) rule file.
// Uses "1filter" prefix to sort after NAT rules (e.g., "1filter-Users-alice-project").
func filterRuleFile(projectDir string) string {
	return filepath.Join(pfAnchorDir, filterRulePrefix+projectFileName(projectDir))
}

// legacyRuleFile returns the old single-file path for migration cleanup.
// Old format stored both NAT and filter rules in one file (e.g., "-Users-alice-project").
func legacyRuleFile(projectDir string) string {
	return filepath.Join(pfAnchorDir, projectFileName(projectDir))
}

// writeBroadNATRules writes NAT rules for all interfaces (used for lan-access: ['*']).
func writeBroadNATRules(sb *strings.Builder, containerIP string, interfaces []string) {
	sb.WriteString("# Broad NAT for all LAN access\n")
	for _, iface := range interfaces {
		fmt.Fprintf(sb, "nat on %s from %s to any -> (%s)\n", iface, containerIP, iface)
	}
	sb.WriteString("\n")
}

// ApplyRules creates pf rules to apply network isolation with allow rules.
// NAT and filter rules are written to separate files with ordering prefixes
// (0nat-*, 1filter-*) so that cat * produces correct pf rule ordering:
// all translation rules before all filtering rules.
//
// For OrbStack: NAT rules are generated per-container:
//   - lan-access: ['*'] → broad NAT to all interfaces
//   - lan-access: [whitelist] → selective NAT + no-nat catch-all
//   - lan-access: [] or none → no-nat catch-all only
//
// For Docker Desktop: uses filter rules only (block non-whitelisted traffic).
func (p *PF) ApplyRules(containerID string, containerIP string, rules []shared.LANAccessRule) error {
	header := fmt.Sprintf("# Container: %s (%s)\n", containerID, containerIP)
	hasAllLAN := shared.HasAllLAN(rules)

	// Build NAT content (OrbStack only)
	var natContent string
	if p.env.IsOrbStack {
		var sb strings.Builder
		sb.WriteString(header)
		pfh := newPfHelper()
		interfaces, _ := pfh.getPhysicalInterfaces(p.env)
		if len(interfaces) > 0 {
			if hasAllLAN {
				writeBroadNATRules(&sb, containerIP, interfaces)
			} else if len(rules) > 0 {
				sb.WriteString("# Selective NAT for whitelisted destinations\n")
				for _, rule := range rules {
					for _, iface := range interfaces {
						writeSelectiveNATRule(&sb, iface, containerIP, rule)
					}
				}
				sb.WriteString("\n")
				sb.WriteString("# Catch-all: prevent other traffic from being NAT'd\n")
				sb.WriteString(fmt.Sprintf("no nat from %s to any\n", containerIP))
			} else {
				sb.WriteString("# No LAN access: block all NAT for this container\n")
				sb.WriteString(fmt.Sprintf("no nat from %s to any\n", containerIP))
			}
		}
		natContent = sb.String()
	}

	// Build filter content (Docker Desktop + defense-in-depth for OrbStack)
	var filterContent string
	if !hasAllLAN {
		var sb strings.Builder
		sb.WriteString(header)
		sb.WriteString(generatePfRuleset(containerIP, rules))
		filterContent = sb.String()
	}

	// Ensure directory exists with sudo
	if _, err := p.env.Cmd.SudoRunQuiet("mkdir", "-p", pfAnchorDir); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}

	// Write NAT file (or remove if empty)
	if err := writeOrRemoveRuleFile(p.env, natRuleFile(p.env.ProjectDir), natContent); err != nil {
		return err
	}

	// Write filter file (or remove if empty)
	if err := writeOrRemoveRuleFile(p.env, filterRuleFile(p.env.ProjectDir), filterContent); err != nil {
		return err
	}

	// Remove legacy single-file format (migration from pre-split layout)
	_, _ = p.env.Cmd.SudoRunQuiet("rm", "-f", legacyRuleFile(p.env.ProjectDir))

	// Load ALL anchor files together. File prefixes ensure correct ordering:
	// 0nat-* (translation) before 1filter-* (filtering).
	cmd := fmt.Sprintf("cat %s/* 2>/dev/null | pfctl -a %s -f -", pfAnchorDir, pfAnchorName)
	output, err := p.env.Cmd.SudoRunQuiet("sh", "-c", cmd)
	if err != nil {
		return fmt.Errorf("failed to load pf rules: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// writeOrRemoveRuleFile writes content to a rule file, or removes the file if content is empty.
// Uses temp file + sudo mv pattern since /etc/pf.anchors requires root access.
func writeOrRemoveRuleFile(env *shared.NetworkEnv, path, content string) error {
	if content == "" {
		_, _ = env.Cmd.SudoRunQuiet("rm", "-f", path)
		return nil
	}

	tmpFile, err := afero.TempFile(env.Fs, "", "alcatraz-rule-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = env.Fs.Remove(tmpPath) }()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if _, err := env.Cmd.SudoRunQuiet("mv", tmpPath, path); err != nil {
		return fmt.Errorf("failed to move rule file to %s: %w", path, err)
	}
	return nil
}

// Cleanup removes pf rules for a container by removing both NAT and filter files and reloading.
func (p *PF) Cleanup(containerID string) error {
	_, _ = p.env.Cmd.SudoRunQuiet("rm", "-f", natRuleFile(p.env.ProjectDir))
	_, _ = p.env.Cmd.SudoRunQuiet("rm", "-f", filterRuleFile(p.env.ProjectDir))
	_, _ = p.env.Cmd.SudoRunQuiet("rm", "-f", legacyRuleFile(p.env.ProjectDir))

	// Reload anchor with remaining files (or empty if none left)
	cmd := fmt.Sprintf("cat %s/* 2>/dev/null | pfctl -a %s -f - || pfctl -a %s -F all", pfAnchorDir, pfAnchorName, pfAnchorName)
	_, _ = p.env.Cmd.SudoRunQuiet("sh", "-c", cmd)
	return nil
}
