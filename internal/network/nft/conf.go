//go:build linux

package nft

import (
	"bufio"
	"strings"

	"github.com/spf13/afero"
)

// hasIncludeLine checks if nftables.conf contains the alcatraz include line.
func (h *nftHelper) hasIncludeLine(fs afero.Fs) bool {
	content, err := afero.ReadFile(fs, nftablesConfPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), alcatrazIncludeLine)
}

// addIncludeLine appends the alcatraz include line to nftables.conf.
func (h *nftHelper) addIncludeLine(fs afero.Fs) error {
	content, err := afero.ReadFile(fs, nftablesConfPath)
	if err != nil {
		// If file doesn't exist, create it with shebang + include
		newFile := "#!/usr/sbin/nft -f\n# Alcatraz nftables configuration\n\n" + alcatrazIncludeLine + "\n"
		return afero.WriteFile(fs, nftablesConfPath, []byte(newFile), 0644)
	}

	// Append include line
	newContent := string(content)
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += alcatrazIncludeLine + "\n"

	return afero.WriteFile(fs, nftablesConfPath, []byte(newContent), 0644)
}

// removeIncludeLine removes the alcatraz include line from nftables.conf.
func (h *nftHelper) removeIncludeLine(fs afero.Fs) error {
	content, err := afero.ReadFile(fs, nftablesConfPath)
	if err != nil {
		return nil // File doesn't exist, nothing to do
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != alcatrazIncludeLine {
			lines = append(lines, line)
		}
	}

	newContent := strings.Join(lines, "\n")
	if len(lines) > 0 {
		newContent += "\n"
	}

	return afero.WriteFile(fs, nftablesConfPath, []byte(newContent), 0644)
}
