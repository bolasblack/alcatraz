// Package cli implements the Alcatraz command-line interface.
// See AGD-009 for CLI design decisions.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// ConfigFilename is the standard configuration file name.
// See AGD-009 for configuration format design.
const ConfigFilename = ".alca.toml"

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Alcatraz configuration in current directory",
	Long:  `Initialize Alcatraz by creating a .alca.toml configuration file in the current directory with default settings.`,
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(cwd, ConfigFilename)

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("configuration file already exists: %s", configPath)
	}

	// Interactive template selection
	var selectedTemplate string
	err = huh.NewSelect[string]().
		Title("Select a template").
		Options(
			huh.NewOption("Nix - NixOS-based development environment", string(config.TemplateNix)),
			huh.NewOption("Debian - Debian-based environment with mise", string(config.TemplateDebian)),
		).
		Value(&selectedTemplate).
		Run()
	if err != nil {
		return fmt.Errorf("template selection cancelled: %w", err)
	}

	// Generate configuration from template
	template := config.Template(selectedTemplate)
	content, err := config.GenerateConfig(template)
	if err != nil {
		return fmt.Errorf("failed to generate configuration: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Println("Edit this file to customize your container settings.")
	return nil
}
