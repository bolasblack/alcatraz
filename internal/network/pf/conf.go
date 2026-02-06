//go:build darwin

package pf

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// pf.conf constants.
const (
	// pfAnchorName is the name of the pf anchor.
	pfAnchorName = "alcatraz"
	// pfConfPath is the path to the pf configuration file.
	pfConfPath = "/etc/pf.conf"
)

// newPfHelper creates a new pfHelper instance.
func newPfHelper() *pfHelper {
	return &pfHelper{}
}

// parsePfConfAndRemoveOldAnchor parses pf.conf content and removes old anchor line.
func parsePfConfAndRemoveOldAnchor(content, oldAnchorLine string) []string {
	lines := strings.Split(content, "\n")
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip old wildcard anchor (migration)
		if trimmed == oldAnchorLine {
			continue
		}
		newLines = append(newLines, line)
	}

	return newLines
}

// ensurePfAnchor adds nat-anchor and filter anchor to /etc/pf.conf if not present.
// IMPORTANT: nat-anchor lines must come BEFORE anchor lines in pf.conf.
func (p *pfHelper) ensurePfAnchor(env *shared.NetworkEnv) error {
	content, err := afero.ReadFile(env.Fs, pfConfPath)
	if err != nil {
		return fmt.Errorf("failed to read pf.conf: %w", err)
	}

	contentStr := string(content)

	// Check if both anchors already exist using exact line matching.
	// IMPORTANT: "nat-anchor X" contains "anchor X" as a substring, so we must
	// use hasExactLine() rather than strings.Contains() to avoid false positives.
	hasNatAnchor := hasExactLine(contentStr, pfAnchorLine)
	hasFilterAnchor := hasExactLine(contentStr, pfFilterAnchorLine)
	if hasNatAnchor && hasFilterAnchor {
		return nil
	}

	lines := strings.Split(contentStr, "\n")

	// Insert nat-anchor if not present
	if !hasNatAnchor {
		lines = insertAnchorLine(lines, pfAnchorLine)
	}

	// Insert filter anchor if not present (goes after nat-anchor lines, in filter section)
	if !hasFilterAnchor {
		lines = insertFilterAnchorLine(lines, pfFilterAnchorLine)
	}

	return writePfConf(env, lines)
}

// removePfAnchor removes both nat-anchor and filter anchor from /etc/pf.conf.
func (p *pfHelper) removePfAnchor(env *shared.NetworkEnv) error {
	content, err := afero.ReadFile(env.Fs, pfConfPath)
	if err != nil {
		return fmt.Errorf("failed to read pf.conf: %w", err)
	}

	contentStr := string(content)
	hasNatAnchor := strings.Contains(contentStr, pfAnchorLine)
	hasFilterAnchor := strings.Contains(contentStr, pfFilterAnchorLine)
	if !hasNatAnchor && !hasFilterAnchor {
		return nil // Already removed
	}

	// Remove both anchor lines
	lines := strings.Split(contentStr, "\n")
	var newLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == pfAnchorLine || trimmed == pfFilterAnchorLine {
			continue
		}
		newLines = append(newLines, line)
	}

	return writePfConf(env, newLines)
}

// writeAnchorFile writes a file to the pf anchor directory in the staging filesystem.
// Ensures the anchor directory exists before writing.
func (p *pfHelper) writeAnchorFile(env *shared.NetworkEnv, filename, content string) error {
	if err := env.Fs.MkdirAll(pfAnchorDir, 0755); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}

	filePath := filepath.Join(pfAnchorDir, filename)
	if err := afero.WriteFile(env.Fs, filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to stage %s: %w", filename, err)
	}
	return nil
}

// writeSharedRule writes the shared NAT rule file to the staging filesystem.
func (p *pfHelper) writeSharedRule(env *shared.NetworkEnv, rules string) error {
	return p.writeAnchorFile(env, sharedRuleFile, rules)
}

// writeProjectFile writes the project-specific rule file to the staging filesystem.
func (p *pfHelper) writeProjectFile(env *shared.NetworkEnv, projectDir, content string) error {
	return p.writeAnchorFile(env, p.projectFileName(projectDir), content)
}

// insertFilterAnchorLine inserts the filter anchor line at the appropriate position.
// It should be placed after the last existing "anchor" line in the filter section,
// or after the last nat-anchor line if no anchor lines exist.
func insertFilterAnchorLine(lines []string, filterAnchorLine string) []string {
	lastAnchorIdx := findLastAnchorIndex(lines)
	if lastAnchorIdx >= 0 {
		return insertAfterIndex(lines, lastAnchorIdx, filterAnchorLine)
	}

	// No anchor lines found - insert after last nat-anchor
	lastNatIdx := findLastNatAnchorIndex(lines)
	if lastNatIdx >= 0 {
		return insertAfterIndex(lines, lastNatIdx, filterAnchorLine)
	}

	// No anchor lines at all - append before trailing empty line
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		result := make([]string, 0, len(lines)+1)
		result = append(result, lines[:len(lines)-1]...)
		result = append(result, filterAnchorLine, "")
		return result
	}

	return append(lines, filterAnchorLine)
}

// findLastAnchorIndex finds the index of the last "anchor" (non-nat-anchor) line.
func findLastAnchorIndex(lines []string) int {
	lastIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "anchor ") {
			lastIdx = i
		}
	}
	return lastIdx
}

// insertAnchorLine inserts the anchor line at the appropriate position.
// It should be placed after the last nat-anchor line, or before the first anchor line.
func insertAnchorLine(lines []string, anchorLine string) []string {
	lastNatAnchorIdx := findLastNatAnchorIndex(lines)

	// Insert after last nat-anchor if found
	if lastNatAnchorIdx >= 0 {
		return insertAfterIndex(lines, lastNatAnchorIdx, anchorLine)
	}

	// If no nat-anchor found, insert before first "anchor" line
	return insertBeforeFirstAnchor(lines, anchorLine)
}

// findLastNatAnchorIndex finds the index of the last nat-anchor line.
func findLastNatAnchorIndex(lines []string) int {
	lastIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "nat-anchor ") {
			lastIdx = i
		}
	}
	return lastIdx
}

// insertAfterIndex inserts a line after the given index.
func insertAfterIndex(lines []string, idx int, line string) []string {
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:idx+1]...)
	result = append(result, line)
	result = append(result, lines[idx+1:]...)
	return result
}

// insertBeforeFirstAnchor inserts a line before the first "anchor" line.
// If no anchor line found, appends before any trailing empty line.
func insertBeforeFirstAnchor(lines []string, anchorLine string) []string {
	// Find first "anchor" line position
	anchorIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "anchor ") {
			anchorIdx = i
			break
		}
	}

	// Insert before anchor if found
	if anchorIdx >= 0 {
		result := make([]string, 0, len(lines)+1)
		result = append(result, lines[:anchorIdx]...)
		result = append(result, anchorLine)
		result = append(result, lines[anchorIdx:]...)
		return result
	}

	// No anchor found - append at end, but before trailing empty line
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		result := make([]string, 0, len(lines)+1)
		result = append(result, lines[:len(lines)-1]...)
		result = append(result, anchorLine, "")
		return result
	}

	return append(lines, anchorLine)
}

// writePfConf writes the new pf.conf content to the staging filesystem.
// The actual file write with sudo will happen during commit.
func writePfConf(env *shared.NetworkEnv, lines []string) error {
	newContent := strings.Join(lines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	// Stage the write - actual sudo write happens during commit
	if err := afero.WriteFile(env.Fs, pfConfPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to stage pf.conf: %w", err)
	}

	return nil
}
