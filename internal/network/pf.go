// Package network provides network configuration helpers for Alcatraz.
// This file contains pf (packet filter) configuration functions for macOS.
// See AGD-023 for LAN access design decisions.
package network

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// pf.conf constants.
const (
	// pfAnchorName is the name of the pf anchor.
	pfAnchorName = "alcatraz"
	// pfConfPath is the path to the pf configuration file.
	pfConfPath = "/etc/pf.conf"
)

// ensurePfAnchor adds nat-anchor to /etc/pf.conf if not present.
// Also handles migration from old wildcard format.
// IMPORTANT: nat-anchor lines must come BEFORE anchor lines in pf.conf.
func (p *pfHelper) ensurePfAnchor(env *util.Env) error {
	anchorLine := fmt.Sprintf(`nat-anchor "%s"`, pfAnchorName)
	oldAnchorLine := fmt.Sprintf(`nat-anchor "%s/*"`, pfAnchorName)

	content, err := afero.ReadFile(env.Fs, pfConfPath)
	if err != nil {
		return fmt.Errorf("failed to read pf.conf: %w", err)
	}

	contentStr := string(content)

	// Check if new anchor already exists (and no old one to migrate)
	hasNewAnchor := strings.Contains(contentStr, anchorLine)
	hasOldAnchor := strings.Contains(contentStr, oldAnchorLine)
	if hasNewAnchor && !hasOldAnchor {
		return nil
	}

	newLines := parsePfConfAndRemoveOldAnchor(contentStr, oldAnchorLine)
	newLines = insertAnchorLine(newLines, anchorLine)

	return writePfConf(env, newLines)
}

// removePfAnchor removes nat-anchor from /etc/pf.conf.
func (p *pfHelper) removePfAnchor(env *util.Env) error {
	anchorLine := fmt.Sprintf(`nat-anchor "%s"`, pfAnchorName)

	content, err := afero.ReadFile(env.Fs, pfConfPath)
	if err != nil {
		return fmt.Errorf("failed to read pf.conf: %w", err)
	}

	if !strings.Contains(string(content), anchorLine) {
		return nil // Already removed
	}

	// Remove anchor line
	lines := strings.Split(string(content), "\n")
	var newLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != anchorLine {
			newLines = append(newLines, line)
		}
	}

	return writePfConf(env, newLines)
}

// writeSharedRule writes the shared NAT rule file to the staging filesystem.
// The actual file write with sudo will happen during commit.
func (p *pfHelper) writeSharedRule(env *util.Env, rules string) error {
	sharedPath := filepath.Join(pfAnchorDir, sharedRuleFile)

	// Ensure parent directory exists in staging
	if err := env.Fs.MkdirAll(pfAnchorDir, 0755); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}

	// Stage the write
	if err := afero.WriteFile(env.Fs, sharedPath, []byte(rules), 0644); err != nil {
		return fmt.Errorf("failed to stage shared rule: %w", err)
	}
	return nil
}

// writeProjectFile writes the project-specific rule file to the staging filesystem.
// The actual file write with sudo will happen during commit.
func (p *pfHelper) writeProjectFile(env *util.Env, projectDir, content string) error {
	filename := p.projectFileName(projectDir)
	projectFilePath := filepath.Join(pfAnchorDir, filename)

	// Ensure parent directory exists in staging
	if err := env.Fs.MkdirAll(pfAnchorDir, 0755); err != nil {
		return fmt.Errorf("failed to create anchor directory: %w", err)
	}

	// Stage the write
	if err := afero.WriteFile(env.Fs, projectFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to stage project file: %w", err)
	}
	return nil
}

// parsePfConfAndRemoveOldAnchor parses pf.conf lines and removes old wildcard anchor.
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
func writePfConf(env *util.Env, lines []string) error {
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
