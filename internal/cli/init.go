// Package cli implements the Alcatraz command-line interface.
// See AGD-009 for CLI design decisions.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
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
	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create transact filesystem for writing
	tfs := transact.New()
	env := util.NewEnv(tfs)

	configPath := filepath.Join(cwd, ConfigFilename)

	// Check if config already exists
	if _, err := env.Fs.Stat(configPath); err == nil {
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

	if err := afero.WriteFile(env.Fs, configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	// Commit the changes (project dir, normally no sudo needed)
	if err := commitWithSudo(cmd.Context(), env, tfs, os.Stdout, ""); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	util.ProgressDone(os.Stdout, "Created %s\n", configPath)
	fmt.Println("Edit this file to customize your container settings.")
	return nil
}
