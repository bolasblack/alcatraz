// Package network provides network configuration helpers for Alcatraz.
// See AGD-023 for LAN access design decisions.
//
// Naming conventions:
//   - Is*: checks existence or state (e.g., isHelperInstalled)
//   - Has*: checks possession or configuration (e.g., hasLANAccess)
package network

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// pf anchor constants for macOS NAT configuration.
// See AGD-023 for design decisions.
const (
	// pfAnchorDir is the directory for pf anchor files.
	pfAnchorDir = "/etc/pf.anchors/alcatraz"
	// sharedRuleFile is the filename for the shared NAT rule.
	sharedRuleFile = "_shared"
	// lanAccessWildcard is the wildcard value for full LAN access.
	lanAccessWildcard = "*"
)

// parseLineValues extracts values from "key: value" lines in command output.
// Returns a slice of trimmed values (may be empty).
func parseLineValues(output, prefix string) []string {
	var values []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				if value := strings.TrimSpace(parts[1]); value != "" {
					values = append(values, value)
				}
			}
		}
	}
	return values
}

// parseLineValue extracts the first value from a "key: value" line in command output.
// Returns the trimmed value and true if found, empty string and false otherwise.
func parseLineValue(output, prefix string) (string, bool) {
	values := parseLineValues(output, prefix)
	if len(values) > 0 {
		return values[0], true
	}
	return "", false
}

// getOrbStackSubnet gets the OrbStack network subnet from orbctl config.
// Returns the subnet (e.g., "192.168.138.0/23") or error.
func (p *pfHelper) getOrbStackSubnet(env *util.Env) (string, error) {
	output, err := env.Cmd.Run("orbctl", "config", "show")
	if err != nil {
		return "", fmt.Errorf("failed to run orbctl config show: %w", err)
	}

	if subnet, found := parseLineValue(string(output), "network.subnet4"); found {
		return subnet, nil
	}

	return "", fmt.Errorf("network.subnet4 not found in orbctl output")
}

// getDefaultInterface returns the default network interface on macOS.
// Uses `route -n get default` to detect the active interface.
func (p *pfHelper) getDefaultInterface(env *util.Env) (string, error) {
	output, err := env.Cmd.Run("route", "-n", "get", "default")
	if err != nil {
		return "", fmt.Errorf("failed to get default route: %w", err)
	}

	if iface, found := parseLineValue(string(output), "interface:"); found {
		return iface, nil
	}

	return "", fmt.Errorf("interface not found in route output")
}

// getPhysicalInterfaces returns all physical network interfaces on macOS.
// Uses `networksetup -listallhardwareports` to enumerate.
func (p *pfHelper) getPhysicalInterfaces(env *util.Env) ([]string, error) {
	output, err := env.Cmd.Run("networksetup", "-listallhardwareports")
	if err != nil {
		return nil, fmt.Errorf("failed to list hardware ports: %w", err)
	}

	interfaces := parseLineValues(string(output), "Device:")
	if len(interfaces) == 0 {
		return nil, fmt.Errorf("interfaces not found in networksetup output")
	}

	return interfaces, nil
}

// projectFileName converts a project path to a safe filename.
// Replaces "/" with "-" (e.g., "/Users/alice/project" becomes "-Users-alice-project").
func (p *pfHelper) projectFileName(projectPath string) string {
	return strings.ReplaceAll(projectPath, "/", "-")
}

// generateNATRules generates NAT rule content for the given subnet and interfaces.
func (p *pfHelper) generateNATRules(subnet string, interfaces []string) string {
	var rules strings.Builder
	for _, iface := range interfaces {
		rules.WriteString(fmt.Sprintf("nat on %s from %s to any -> (%s)\n", iface, subnet, iface))
	}
	return rules.String()
}

// readExistingRuleInterfaces reads the existing rule file.
// Returns (interfaces, exists, error) where exists indicates if the file was found.
func (p *pfHelper) readExistingRuleInterfaces(env *util.Env) (interfaces []string, exists bool, err error) {
	sharedPath := filepath.Join(pfAnchorDir, sharedRuleFile)

	data, err := afero.ReadFile(env.Fs, sharedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to read rule file: %w", err)
	}

	return p.parseRuleInterfaces(string(data)), true, nil
}

// parseRuleInterfaces extracts interface names from NAT rule content.
func (p *pfHelper) parseRuleInterfaces(content string) []string {
	var interfaces []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nat on ") {
			// Parse: "nat on en0 from ..."
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				interfaces = append(interfaces, parts[2])
			}
		}
	}
	return interfaces
}

// needsRuleUpdate checks if the rule file needs to be updated.
// Returns (needsUpdate, newInterfaces, error).
func (p *pfHelper) needsRuleUpdate(env *util.Env) (bool, []string, error) {
	currentInterfaces, err := p.getPhysicalInterfaces(env)
	if err != nil {
		return false, nil, err
	}

	existingInterfaces, exists, err := p.readExistingRuleInterfaces(env)
	if err != nil {
		return false, nil, err
	}

	if !exists {
		return true, currentInterfaces, nil
	}

	newInterfaces := findNewInterfaces(currentInterfaces, existingInterfaces)
	return len(newInterfaces) > 0, newInterfaces, nil
}

// findNewInterfaces returns interfaces in current that are not in existing.
func findNewInterfaces(current, existing []string) []string {
	existingSet := make(map[string]bool, len(existing))
	for _, iface := range existing {
		existingSet[iface] = true
	}

	var newIfaces []string
	for _, iface := range current {
		if !existingSet[iface] {
			newIfaces = append(newIfaces, iface)
		}
	}
	return newIfaces
}

// deleteProjectFile removes the project-specific rule file.
// Returns (removeShared, error) where removeShared indicates the shared file
// should also be removed (no other projects remain).
func (p *pfHelper) deleteProjectFile(env *util.Env, projectPath string) (removeShared bool, err error) {
	filename := p.projectFileName(projectPath)
	projectFilePath := filepath.Join(pfAnchorDir, filename)

	// Remove project file
	if err := env.Fs.Remove(projectFilePath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to remove project file: %w", err)
	}

	// Flush main anchor - LaunchDaemon will reload on file change
	flushAnchor(env, pfAnchorName)

	// Check if other project files exist
	entries, err := afero.ReadDir(env.Fs, pfAnchorDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read pf anchor directory: %w", err)
	}

	// Count non-shared files
	for _, entry := range entries {
		if entry.Name() != sharedRuleFile {
			return false, nil // Other projects exist
		}
	}

	return true, nil // No other projects, remove shared
}

// deleteSharedRule removes the shared NAT rule file.
func (p *pfHelper) deleteSharedRule(env *util.Env) error {
	sharedPath := filepath.Join(pfAnchorDir, sharedRuleFile)

	if err := env.Fs.Remove(sharedPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove shared rule: %w", err)
	}

	flushAnchor(env, pfAnchorName)
	return nil
}

// flushAnchor flushes a pf anchor. Logs a warning on failure (non-fatal).
// For operations requiring progress reporting, use flushPfRules instead.
func flushAnchor(env *util.Env, anchorName string) {
	output, err := env.Cmd.SudoRunQuiet("pfctl", "-a", anchorName, "-F", "all")
	if err != nil {
		log.Printf("Warning: pfctl -a %s -F all failed: %v: %s", anchorName, err, output)
	}
}

// fileExists checks if a file or directory exists.
func fileExists(env *util.Env, path string) bool {
	_, err := env.Fs.Stat(path)
	return err == nil
}
