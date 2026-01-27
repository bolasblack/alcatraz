// Package network provides network configuration helpers for Alcatraz.
// See AGD-023 for LAN access design decisions.
package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// PfAnchorDir is the directory for pf anchor files.
	PfAnchorDir = "/etc/pf.anchors/alcatraz"
	// SharedRuleFile is the filename for the shared NAT rule.
	SharedRuleFile = "_shared"
	// LaunchDaemonPlist is the path to the LaunchDaemon plist.
	LaunchDaemonPlist = "/Library/LaunchDaemons/com.alcatraz.pf-watcher.plist"
)

// GetOrbStackSubnet gets the OrbStack network subnet from orbctl config.
// Returns the subnet (e.g., "192.168.138.0/23") or error.
func GetOrbStackSubnet() (string, error) {
	cmd := exec.Command("orbctl", "config", "show")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run orbctl config show: %w", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "network.subnet4") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("network.subnet4 not found in orbctl config")
}

// IsNetworkHelperInstalled checks if the network helper LaunchDaemon is installed.
func IsNetworkHelperInstalled() bool {
	_, err := os.Stat(LaunchDaemonPlist)
	return err == nil
}

// GetDefaultInterface returns the default network interface on macOS.
// Uses `route -n get default` to detect the active interface.
func GetDefaultInterface() (string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default route: %w", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("default interface not found in route output")
}

// GetPhysicalInterfaces returns all physical network interfaces on macOS.
// Uses `networksetup -listallhardwareports` to enumerate.
func GetPhysicalInterfaces() ([]string, error) {
	cmd := exec.Command("networksetup", "-listallhardwareports")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list hardware ports: %w", err)
	}

	var interfaces []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Device:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				iface := strings.TrimSpace(parts[1])
				if iface != "" {
					interfaces = append(interfaces, iface)
				}
			}
		}
	}

	if len(interfaces) == 0 {
		return nil, fmt.Errorf("no physical interfaces found")
	}

	return interfaces, nil
}

// ProjectFileName converts a project path to a safe filename.
// Replaces "/" with "-" (e.g., "/Users/alice/project" becomes "-Users-alice-project").
func ProjectFileName(projectPath string) string {
	return strings.ReplaceAll(projectPath, "/", "-")
}

// GenerateNATRules generates NAT rule content for the given subnet and interfaces.
func GenerateNATRules(subnet string, interfaces []string) string {
	var rules strings.Builder
	for _, iface := range interfaces {
		rules.WriteString(fmt.Sprintf("nat on %s from %s to any -> (%s)\n", iface, subnet, iface))
	}
	return rules.String()
}

// ReadExistingRuleInterfaces reads the existing rule file and extracts interface names.
// Returns nil if file doesn't exist.
func ReadExistingRuleInterfaces() ([]string, error) {
	sharedPath := filepath.Join(PfAnchorDir, SharedRuleFile)

	data, err := os.ReadFile(sharedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read rule file: %w", err)
	}

	return ParseRuleInterfaces(string(data)), nil
}

// ParseRuleInterfaces extracts interface names from NAT rule content.
func ParseRuleInterfaces(content string) []string {
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

// NeedsRuleUpdate checks if the rule file needs to be updated.
// Returns (needsUpdate, newInterfaces, error).
func NeedsRuleUpdate() (bool, []string, error) {
	// Get current physical interfaces
	currentInterfaces, err := GetPhysicalInterfaces()
	if err != nil {
		return false, nil, err
	}

	// Read existing rule interfaces
	existingInterfaces, err := ReadExistingRuleInterfaces()
	if err != nil {
		return false, nil, err
	}

	// If no existing file, needs update
	if existingInterfaces == nil {
		return true, currentInterfaces, nil
	}

	// Find new interfaces not in existing rules
	existingSet := make(map[string]bool)
	for _, iface := range existingInterfaces {
		existingSet[iface] = true
	}

	var newInterfaces []string
	for _, iface := range currentInterfaces {
		if !existingSet[iface] {
			newInterfaces = append(newInterfaces, iface)
		}
	}

	return len(newInterfaces) > 0, newInterfaces, nil
}

// DeleteProjectFile removes the project-specific rule file.
// Returns true if the shared file should also be removed (no other projects).
func DeleteProjectFile(projectPath string) (removeShared bool, err error) {
	filename := ProjectFileName(projectPath)
	projectFilePath := filepath.Join(PfAnchorDir, filename)

	// Remove project file
	if err := os.Remove(projectFilePath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to remove project file: %w", err)
	}

	// Flush main anchor - LaunchDaemon will reload on file change
	if err := flushAnchor("alcatraz"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to flush anchor: %v\n", err)
	}

	// Check if other project files exist
	entries, err := os.ReadDir(PfAnchorDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read pf anchor directory: %w", err)
	}

	// Count non-shared files
	for _, entry := range entries {
		if entry.Name() != SharedRuleFile {
			return false, nil // Other projects exist
		}
	}

	return true, nil // No other projects, remove shared
}

// DeleteSharedRule removes the shared NAT rule file and flushes the anchor.
func DeleteSharedRule() error {
	sharedPath := filepath.Join(PfAnchorDir, SharedRuleFile)

	if err := os.Remove(sharedPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove shared rule: %w", err)
	}

	if err := flushAnchor("alcatraz"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to flush anchor: %v\n", err)
	}

	return nil
}

// flushAnchor flushes a pf anchor.
func flushAnchor(anchorName string) error {
	cmd := exec.Command("pfctl", "-a", anchorName, "-F", "all")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pfctl -a %s -F all failed: %w: %s", anchorName, err, string(output))
	}
	return nil
}

// HasLANAccess checks if the config has LAN access enabled.
func HasLANAccess(lanAccess []string) bool {
	for _, access := range lanAccess {
		if access == "*" {
			return true
		}
	}
	return false
}
