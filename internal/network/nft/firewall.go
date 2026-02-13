package nft

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
)

// Compile-time interface assertion.
var _ shared.Firewall = (*NFTables)(nil)

// NFTables implements shared.Firewall using nftables.
// Each container gets its own table for isolation and clean teardown (AGD-030).
type NFTables struct {
	env         *shared.NetworkEnv
	vmHelperEnv *vmhelper.VMHelperEnv // pre-constructed for Darwin; nil on Linux
}

// isDarwin reports whether this instance targets macOS (Darwin).
// Uses the Runtime field which is set by CLI via runtime.DetectPlatform().
// See runtime.IsDarwin() for the platform detection logic.
func (n *NFTables) isDarwin() bool {
	return runtime.IsDarwin(n.env.Runtime)
}

// tableName returns the nftables table name for a container.
// Uses short container ID prefix to keep names manageable.
func tableName(containerID string) string {
	return "alca-" + shared.ShortContainerID(containerID)
}

// nftFileName returns the nft rule filename for a project.
// Uses the project directory path encoded as a safe filename.
func nftFileName(projectDir string) string {
	return shared.EncodePathForFilename(projectDir) + ".nft"
}

// chainPriority returns the nftables chain priority string for the given runtime.
// OrbStack: filter - 2 (must beat flowtable offload)
// Docker Desktop: filter - 1
func chainPriority(rt runtime.RuntimePlatform) string {
	if rt == runtime.PlatformMacOrbStack {
		return "filter - 2"
	}
	return "filter - 1"
}

// writeIdempotentHeader writes the nft shebang and idempotent create/delete table pattern.
func writeIdempotentHeader(sb *strings.Builder, comment string, table string) {
	sb.WriteString("#!/usr/sbin/nft -f\n")
	fmt.Fprintf(sb, "# %s\n\n", comment)
	sb.WriteString("# Delete table if exists (idempotent)\n")
	fmt.Fprintf(sb, "table inet %s\n", table)
	fmt.Fprintf(sb, "delete table inet %s\n\n", table)
}

// writeAllowRules writes per-container allow rules, skipping AllLAN entries.
func writeAllowRules(sb *strings.Builder, containerIP string, containerIsV6 bool, rules []shared.LANAccessRule, comment string) {
	if len(rules) == 0 {
		return
	}
	fmt.Fprintf(sb, "\t\t# %s\n", comment)
	for _, rule := range rules {
		if rule.AllLAN {
			continue
		}
		writeNftAllowRule(sb, containerIP, containerIsV6, rule)
	}
	sb.WriteString("\n")
}

// generateRuleset generates the nftables ruleset for a container with allow rules.
// If rules is empty, blocks all RFC1918 traffic.
// Uses idempotent flush+recreate pattern per AGD-028.
// This is a pure function for testability.
func generateRuleset(tableName string, containerIP string, rules []shared.LANAccessRule, priority string, projectDir string, projectID string) string {
	var sb strings.Builder

	containerIsV6 := shared.IsIPv6(containerIP)

	writeIdempotentHeader(&sb, fmt.Sprintf("Alcatraz container rules for table: %s", tableName), tableName)
	fmt.Fprintf(&sb, "# project-dir: %s\n", projectDir)
	fmt.Fprintf(&sb, "# project-id: %s\n\n", projectID)

	// Create fresh table with rules
	sb.WriteString("# Create fresh table with rules\n")
	fmt.Fprintf(&sb, "table inet %s {\n", tableName)
	sb.WriteString("\tchain forward {\n")
	fmt.Fprintf(&sb, "\t\ttype filter hook forward priority %s; policy accept;\n\n", priority)
	sb.WriteString("\t\t# Allow established/related connections (return traffic)\n")
	sb.WriteString("\t\tct state established,related accept\n\n")

	writeAllowRules(&sb, containerIP, containerIsV6, rules, "Allow rules from lan-access configuration")

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

	for _, suffix := range formatProtocolSuffixes(rule.Protocol, rule.Port) {
		sb.WriteString(base + suffix + " accept\n")
	}
}

// formatProtocolSuffixes returns the nft rule suffixes for a protocol/port combination.
// Each suffix is appended to the base "saddr X daddr Y" to form a complete rule.
func formatProtocolSuffixes(proto shared.Protocol, port int) []string {
	switch {
	case port == 0 && proto == shared.ProtoAll:
		return []string{""}
	case port == 0 && proto == shared.ProtoTCP:
		return []string{" tcp dport 1-65535"}
	case port == 0 && proto == shared.ProtoUDP:
		return []string{" udp dport 1-65535"}
	case port > 0 && proto == shared.ProtoTCP:
		return []string{fmt.Sprintf(" tcp dport %d", port)}
	case port > 0 && proto == shared.ProtoUDP:
		return []string{fmt.Sprintf(" udp dport %d", port)}
	case port > 0 && proto == shared.ProtoAll:
		return []string{
			fmt.Sprintf(" tcp dport %d", port),
			fmt.Sprintf(" udp dport %d", port),
		}
	default:
		return nil
	}
}

// parseProjectDir extracts the project directory path from an nft ruleset file content.
// Returns empty string if the comment is not found.
func parseProjectDir(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# project-dir: ") {
			return strings.TrimPrefix(line, "# project-dir: ")
		}
	}
	return ""
}

// parseProjectID extracts the project ID from an nft ruleset file content.
// Returns empty string if the comment is not found.
func parseProjectID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# project-id: ") {
			return strings.TrimPrefix(line, "# project-id: ")
		}
	}
	return ""
}

// isStaleProject checks if a project is stale based on its nft file metadata.
// A project is stale if any of: dir doesn't exist, state.json doesn't exist,
// or project ID doesn't match (aligned with AGD-014 orphan detection).
func isStaleProject(fs afero.Fs, projectDir string, projectID string) bool {
	// Condition a: project directory does not exist
	exists, err := afero.DirExists(fs, projectDir)
	if err != nil || !exists {
		return true
	}

	// Condition b: .alca/state.json does not exist
	stateFilePath := filepath.Join(projectDir, ".alca", "state.json")
	data, err := afero.ReadFile(fs, stateFilePath)
	if err != nil {
		return true
	}

	// Condition c: project ID mismatch
	if projectID == "" {
		// Old-format file without project-id, can't verify — not stale
		return false
	}
	var st struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return true
	}
	return st.ProjectID != projectID
}

// parseTableName extracts the table name from an nft ruleset file content.
// Returns empty string if the comment is not found.
func parseTableName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# Alcatraz container rules for table: ") {
			return strings.TrimPrefix(line, "# Alcatraz container rules for table: ")
		}
	}
	return ""
}

// ApplyRules creates nftables rules to apply network isolation with allow rules.
// On Linux: persisted to /etc/nftables.d/alcatraz/<container-id>.nft, loaded via `nft -f`.
// On macOS: persisted to ~/.alcatraz/files/alcatraz_nft/<container-table>.nft, reload via docker exec.
// Returns PostCommitAction that MUST be called after TransactFs.Commit().
func (n *NFTables) ApplyRules(containerID string, containerIP string, rules []shared.LANAccessRule) (*shared.PostCommitAction, error) {
	// If any rule allows all LAN, skip firewall entirely
	if shared.HasAllLAN(rules) {
		return &shared.PostCommitAction{}, nil
	}

	if n.isDarwin() {
		return n.applyRulesOnDarwin(containerID, containerIP, rules)
	}
	return n.applyRulesOnLinux(containerID, containerIP, rules)
}

// writeRuleFile creates the directory and writes the ruleset file atomically.
func writeRuleFile(fs afero.Fs, dir string, fileName string, ruleset string) (string, error) {
	if err := fs.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create nft directory %s: %w", dir, err)
	}
	rulePath := filepath.Join(dir, fileName)
	if err := afero.WriteFile(fs, rulePath, []byte(ruleset), 0644); err != nil {
		return "", fmt.Errorf("failed to write ruleset to %s: %w", rulePath, err)
	}
	return rulePath, nil
}

// applyRulesOnLinux applies per-container rules on Linux.
// Writes the rule file via Fs, returns PostCommitAction to load rules via nft.
func (n *NFTables) applyRulesOnLinux(containerID string, containerIP string, rules []shared.LANAccessRule) (*shared.PostCommitAction, error) {
	table := tableName(containerID)
	ruleset := generateRuleset(table, containerIP, rules, "filter - 1", n.env.ProjectDir, n.env.ProjectID)

	rulePath, err := writeRuleFile(n.env.Fs, nftDirOnLinux(), nftFileName(n.env.ProjectDir), ruleset)
	if err != nil {
		return nil, err
	}

	// Post-commit: load ruleset atomically (idempotent format handles existing table)
	return &shared.PostCommitAction{
		Run: func(ctx context.Context, _ shared.ProgressFunc) error {
			output, err := n.env.Cmd.SudoRunQuiet(ctx, "nft", "-f", rulePath)
			if err != nil {
				return fmt.Errorf("failed to load nftables rules from %s for table %s: %w: %s", rulePath, table, err, strings.TrimSpace(string(output)))
			}
			return nil
		},
	}, nil
}

// applyRulesOnDarwin applies per-container rules on macOS per AGD-030.
// Writes the rule file via Fs, returns PostCommitAction to reload network helper.
func (n *NFTables) applyRulesOnDarwin(containerID string, containerIP string, rules []shared.LANAccessRule) (*shared.PostCommitAction, error) {
	table := tableName(containerID)
	ruleset := generateRuleset(table, containerIP, rules, chainPriority(n.env.Runtime), n.env.ProjectDir, n.env.ProjectID)

	dir, err := nftDirOnDarwin()
	if err != nil {
		return nil, fmt.Errorf("failed to determine nft directory: %w", err)
	}

	rulePath, err := writeRuleFile(n.env.Fs, dir, nftFileName(n.env.ProjectDir), ruleset)
	if err != nil {
		return nil, err
	}

	// Post-commit: trigger reload via network helper container
	return &shared.PostCommitAction{
		Run: func(ctx context.Context, _ shared.ProgressFunc) error {
			if err := n.reloadNetworkHelper(ctx); err != nil {
				return fmt.Errorf("failed to trigger nft reload on darwin for %s: %w", rulePath, err)
			}
			return nil
		},
	}, nil
}

// Cleanup removes all firewall rules for a container.
// On Linux: deletes the persistent rule file and the nftables table.
// On macOS: removes the per-container .nft file and triggers reload.
// Returns PostCommitAction that MUST be called after TransactFs.Commit().
func (n *NFTables) Cleanup(containerID string) (*shared.PostCommitAction, error) {
	if n.isDarwin() {
		return n.cleanupOnDarwin(containerID)
	}
	return n.cleanupOnLinux(containerID)
}

// cleanupOnLinux removes per-container rules on Linux.
// Removes the rule file via Fs, returns PostCommitAction to delete the nftables table.
func (n *NFTables) cleanupOnLinux(containerID string) (*shared.PostCommitAction, error) {
	// Delete rule file (best-effort, ignore if not exists)
	rulePath := filepath.Join(nftDirOnLinux(), nftFileName(n.env.ProjectDir))
	_ = n.env.Fs.Remove(rulePath)

	// Post-commit: delete nftables table
	table := tableName(containerID)
	return &shared.PostCommitAction{
		Run: func(ctx context.Context, _ shared.ProgressFunc) error {
			return n.deleteTable(ctx, table)
		},
	}, nil
}

// cleanupOnDarwin removes per-container rules on macOS.
// Removes the rule file via Fs, returns PostCommitAction to reload network helper.
func (n *NFTables) cleanupOnDarwin(containerID string) (*shared.PostCommitAction, error) {
	dir, err := nftDirOnDarwin()
	if err != nil {
		return nil, fmt.Errorf("failed to determine nft directory: %w", err)
	}

	rulePath := filepath.Join(dir, nftFileName(n.env.ProjectDir))
	_ = n.env.Fs.Remove(rulePath)

	// Post-commit: trigger reload via network helper container
	return &shared.PostCommitAction{
		Run: func(ctx context.Context, _ shared.ProgressFunc) error {
			if err := n.reloadNetworkHelper(ctx); err != nil {
				return fmt.Errorf("failed to trigger nft reload on darwin after cleanup of %s: %w", rulePath, err)
			}
			return nil
		},
	}, nil
}

// deleteTable removes an nftables table. Returns nil if table doesn't exist.
func (n *NFTables) deleteTable(ctx context.Context, table string) error {
	// Requires sudo for nftables access
	output, err := n.env.Cmd.SudoRunQuiet(ctx, "nft", "delete", "table", "inet", table)
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

// CleanupStaleFiles scans the nft rule directory and removes files whose
// project directory no longer exists on disk. Returns the count of cleaned-up
// files. This handles orphaned files from projects that were moved/deleted
// without running "alca down".
func (n *NFTables) CleanupStaleFiles() (int, error) {
	var dir string
	if n.isDarwin() {
		d, err := nftDirOnDarwin()
		if err != nil {
			return 0, fmt.Errorf("failed to determine nft directory: %w", err)
		}
		dir = d
	} else {
		dir = nftDirOnLinux()
	}

	entries, err := afero.ReadDir(n.env.Fs, dir)
	if err != nil {
		// Directory doesn't exist yet — nothing to clean up
		return 0, nil
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".nft") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		content, err := afero.ReadFile(n.env.Fs, filePath)
		if err != nil {
			continue
		}

		projectDir := parseProjectDir(string(content))
		if projectDir == "" {
			// Old format file without project-dir comment — treat as stale
			if err := n.env.Fs.Remove(filePath); err != nil {
				continue
			}
			cleaned++
			continue
		}

		projectID := parseProjectID(string(content))
		if isStaleProject(n.env.Fs, projectDir, projectID) {
			if err := n.env.Fs.Remove(filePath); err != nil {
				continue
			}
			cleaned++
		}
	}

	return cleaned, nil
}
