package nft

import (
	"bufio"
	"strings"

	"github.com/spf13/afero"
)

const alcatrazIncludeLineOnLinux = `include "/etc/nftables.d/alcatraz/*.nft"`

// hasIncludeLineOnLinux checks if nftables.conf contains the alcatraz include line.
func (h *nftLinuxHelper) hasIncludeLineOnLinux(fs afero.Fs) bool {
	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), alcatrazIncludeLineOnLinux)
}

// addIncludeLineOnLinux appends the alcatraz include line to nftables.conf.
func (h *nftLinuxHelper) addIncludeLineOnLinux(fs afero.Fs) error {
	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	if err != nil {
		// If file doesn't exist, create it with shebang + include
		newFile := "#!/usr/sbin/nft -f\n# Alcatraz nftables configuration\n\n" + alcatrazIncludeLineOnLinux + "\n"
		return afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(newFile), 0644)
	}

	// Append include line
	newContent := string(content)
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += alcatrazIncludeLineOnLinux + "\n"

	return afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(newContent), 0644)
}

// removeIncludeLineOnLinux removes the alcatraz include line from nftables.conf.
func (h *nftLinuxHelper) removeIncludeLineOnLinux(fs afero.Fs) error {
	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	if err != nil {
		return nil // File doesn't exist, nothing to do
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != alcatrazIncludeLineOnLinux {
			lines = append(lines, line)
		}
	}

	newContent := strings.Join(lines, "\n")
	if len(lines) > 0 {
		newContent += "\n"
	}

	return afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(newContent), 0644)
}
