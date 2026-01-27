// Package network provides network configuration helpers for Alcatraz.
// This file contains pf (packet filter) configuration functions for macOS.
// See AGD-023 for LAN access design decisions.
package network

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bolasblack/alcatraz/internal/sudo"
)

// pf.conf constants.
const (
	// PfAnchorName is the name of the pf anchor.
	PfAnchorName = "alcatraz"
	// PfConfPath is the path to the pf configuration file.
	PfConfPath = "/etc/pf.conf"
)

// EnsurePfAnchor adds nat-anchor "alcatraz" to /etc/pf.conf if not present.
// Also handles migration from old 'alcatraz/*' wildcard format.
// IMPORTANT: nat-anchor lines must come BEFORE anchor lines in pf.conf.
func EnsurePfAnchor() error {
	anchorLine := `nat-anchor "alcatraz"`
	oldAnchorLine := `nat-anchor "alcatraz/*"`

	content, err := os.ReadFile(PfConfPath)
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

	return writePfConf(newLines)
}

// RemovePfAnchor removes nat-anchor "alcatraz" from /etc/pf.conf.
func RemovePfAnchor() error {
	anchorLine := `nat-anchor "alcatraz"`

	content, err := os.ReadFile(PfConfPath)
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
	newContent := strings.Join(newLines, "\n")

	tmpFile, err := os.CreateTemp("", "pf.conf-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	if err := sudo.Run("cp", tmpFile.Name(), PfConfPath); err != nil {
		return fmt.Errorf("failed to update pf.conf: %w", err)
	}

	// Note: pfctl reload errors are intentionally ignored as they are non-fatal.
	// The rules will be loaded on next system boot or manual reload.
	_ = sudo.Run("pfctl", "-f", PfConfPath)

	return nil
}

// WriteSharedRule writes the shared NAT rule file.
func WriteSharedRule(rules string) error {
	sharedPath := filepath.Join(PfAnchorDir, SharedRuleFile)

	changed, err := sudo.EnsureFileContent(sharedPath, rules)
	if err != nil {
		return fmt.Errorf("failed to write shared rule: %w", err)
	}
	if changed {
		if err := sudo.EnsureChmod(sharedPath, 0644, false); err != nil {
			return fmt.Errorf("failed to set shared rule permissions: %w", err)
		}
	}
	return nil
}

// WriteProjectFile writes the project-specific rule file.
func WriteProjectFile(projectDir, content string) error {
	filename := ProjectFileName(projectDir)
	projectFilePath := filepath.Join(PfAnchorDir, filename)

	changed, err := sudo.EnsureFileContent(projectFilePath, content)
	if err != nil {
		return fmt.Errorf("failed to write project file: %w", err)
	}
	if changed {
		if err := sudo.EnsureChmod(projectFilePath, 0644, false); err != nil {
			return fmt.Errorf("failed to set project file permissions: %w", err)
		}
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
func insertBeforeFirstAnchor(lines []string, anchorLine string) []string {
	result := make([]string, 0, len(lines)+1)
	inserted := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inserted && strings.HasPrefix(trimmed, "anchor ") {
			result = append(result, anchorLine)
			inserted = true
		}
		result = append(result, line)
		// If we reach end without finding anchor line, append
		if i == len(lines)-1 && !inserted {
			if trimmed != "" {
				result = append(result, anchorLine)
			} else {
				// Insert before trailing empty line
				result = append(result[:len(result)-1], anchorLine, "")
			}
		}
	}

	return result
}

// writePfConf writes the new pf.conf content with backup.
func writePfConf(lines []string) error {
	newContent := strings.Join(lines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	// Write to temp file and sudo cp
	tmpFile, err := os.CreateTemp("", "pf.conf-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Backup original
	if err := sudo.Run("cp", PfConfPath, PfConfPath+".bak"); err != nil {
		return fmt.Errorf("failed to backup pf.conf: %w", err)
	}

	// Copy new content
	if err := sudo.Run("cp", tmpFile.Name(), PfConfPath); err != nil {
		return fmt.Errorf("failed to update pf.conf: %w", err)
	}

	// Reload pf - errors are non-fatal, rules will load on next boot
	_ = sudo.Run("pfctl", "-f", PfConfPath)

	return nil
}
