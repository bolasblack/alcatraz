// Package network provides network configuration helpers for Alcatraz.
// See AGD-023 for LAN access design decisions.
//
// Naming conventions:
//   - Is*: checks existence or state (e.g., IsNetworkHelperInstalled)
//   - Has*: checks possession or configuration (e.g., HasLANAccess)
package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// pf anchor constants for macOS NAT configuration.
// See AGD-023 for design decisions.
const (
	// PfAnchorDir is the directory for pf anchor files.
	PfAnchorDir = "/etc/pf.anchors/alcatraz"
	// SharedRuleFile is the filename for the shared NAT rule.
	SharedRuleFile = "_shared"
	// LANAccessWildcard is the wildcard value for full LAN access.
	LANAccessWildcard = "*"
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

// GetOrbStackSubnet gets the OrbStack network subnet from orbctl config.
// Returns the subnet (e.g., "192.168.138.0/23") or error.
func GetOrbStackSubnet() (string, error) {
	cmd := exec.Command("orbctl", "config", "show")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run orbctl config show: %w", err)
	}

	if subnet, found := parseLineValue(string(output), "network.subnet4"); found {
		return subnet, nil
	}

	return "", fmt.Errorf("network.subnet4 not found in orbctl output")
}

// GetDefaultInterface returns the default network interface on macOS.
// Uses `route -n get default` to detect the active interface.
func GetDefaultInterface() (string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default route: %w", err)
	}

	if iface, found := parseLineValue(string(output), "interface:"); found {
		return iface, nil
	}

	return "", fmt.Errorf("interface not found in route output")
}

// GetPhysicalInterfaces returns all physical network interfaces on macOS.
// Uses `networksetup -listallhardwareports` to enumerate.
func GetPhysicalInterfaces() ([]string, error) {
	cmd := exec.Command("networksetup", "-listallhardwareports")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list hardware ports: %w", err)
	}

	interfaces := parseLineValues(string(output), "Device:")
	if len(interfaces) == 0 {
		return nil, fmt.Errorf("interfaces not found in networksetup output")
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

// ReadExistingRuleInterfaces reads the existing rule file.
// Returns (interfaces, exists, error) where exists indicates if the file was found.
func ReadExistingRuleInterfaces(env *util.Env) (interfaces []string, exists bool, err error) {
	sharedPath := filepath.Join(PfAnchorDir, SharedRuleFile)

	data, err := afero.ReadFile(env.Fs, sharedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to read rule file: %w", err)
	}

	return ParseRuleInterfaces(string(data)), true, nil
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
func NeedsRuleUpdate(env *util.Env) (bool, []string, error) {
	// Get current physical interfaces
	currentInterfaces, err := GetPhysicalInterfaces()
	if err != nil {
		return false, nil, err
	}

	// Read existing rule interfaces
	existingInterfaces, exists, err := ReadExistingRuleInterfaces(env)
	if err != nil {
		return false, nil, err
	}

	// If no existing file, needs update
	if !exists {
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
// Returns (removeShared, flushWarning, error) where:
//   - removeShared: true if the shared file should also be removed (no other projects)
//   - flushWarning: non-nil if anchor flush failed (non-fatal)
func DeleteProjectFile(env *util.Env, projectPath string) (removeShared bool, flushWarning error, err error) {
	filename := ProjectFileName(projectPath)
	projectFilePath := filepath.Join(PfAnchorDir, filename)

	// Remove project file
	if err := env.Fs.Remove(projectFilePath); err != nil && !os.IsNotExist(err) {
		return false, nil, fmt.Errorf("failed to remove project file: %w", err)
	}

	// Flush main anchor - LaunchDaemon will reload on file change
	flushWarning = flushAnchor("alcatraz")

	// Check if other project files exist
	entries, err := afero.ReadDir(env.Fs, PfAnchorDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, flushWarning, nil
		}
		return false, flushWarning, fmt.Errorf("failed to read pf anchor directory: %w", err)
	}

	// Count non-shared files
	for _, entry := range entries {
		if entry.Name() != SharedRuleFile {
			return false, flushWarning, nil // Other projects exist
		}
	}

	return true, flushWarning, nil // No other projects, remove shared
}

// DeleteSharedRule removes the shared NAT rule file.
// Returns (flushWarning, error) where flushWarning is non-nil if anchor flush failed (non-fatal).
func DeleteSharedRule(env *util.Env) (flushWarning error, err error) {
	sharedPath := filepath.Join(PfAnchorDir, SharedRuleFile)

	if err := env.Fs.Remove(sharedPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove shared rule: %w", err)
	}

	flushWarning = flushAnchor("alcatraz")
	return flushWarning, nil
}

// flushAnchor flushes a pf anchor using exec.Command directly.
// For operations requiring progress reporting, use FlushPfRules instead.
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
		if access == LANAccessWildcard {
			return true
		}
	}
	return false
}

// FileExists checks if a file or directory exists.
func FileExists(env *util.Env, path string) bool {
	_, err := env.Fs.Stat(path)
	return err == nil
}
