// Package cli implements the Alcatraz command-line interface.
// See AGD-009 for CLI design decisions.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/preset"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

// ConfigFilename is the standard configuration file name.
// See AGD-009 for configuration format design.
const ConfigFilename = ".alca.toml"

var initCmd = &cobra.Command{
	Use:   "init [git+<url>]",
	Short: "Initialize Alcatraz configuration in current directory",
	Long: `Initialize Alcatraz by creating a .alca.toml configuration file in the current directory with default settings.

When called with a git+<url> argument, downloads preset configuration files from a git repository.
Use --template/-t to select a template non-interactively (e.g., --template alpine).
Use --update to refresh previously downloaded preset files to their latest versions.

The --template and --update flags are mutually exclusive.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().Bool("update", false, "Update all preset files to latest versions")
	initCmd.Flags().StringP("template", "t", "", "Template to use (alpine, debian-mise, debian-slim, nix)")
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := getCwd()
	if err != nil {
		return err
	}

	update, _ := cmd.Flags().GetBool("update")
	templateFlag, _ := cmd.Flags().GetString("template")

	// Mutual exclusion: --template and --update cannot be combined
	if update && templateFlag != "" {
		return fmt.Errorf("cannot use --template with --update")
	}

	// Update flow
	if update {
		return runInitUpdate(cmd.Context(), cwd)
	}

	// Preset flow: first arg is a preset URL
	if len(args) > 0 && preset.IsPresetURL(args[0]) {
		return runInitPreset(cmd.Context(), cwd, args[0])
	}

	// Template flow
	return runInitTemplate(cmd.Context(), cwd, templateFlag)
}

func runInitTemplate(ctx context.Context, cwd string, templateFlag string) error {
	// Create transact filesystem for writing
	tfs := transact.New()
	env := util.NewEnv(tfs)

	configPath := filepath.Join(cwd, ConfigFilename)

	// Check if config already exists
	if _, err := env.Fs.Stat(configPath); err == nil {
		return fmt.Errorf("configuration file already exists: %s", configPath)
	}

	var selectedTemplate string

	if templateFlag != "" {
		// Validate the flag value against known templates
		switch config.Template(templateFlag) {
		case config.TemplateAlpine, config.TemplateDebianMise, config.TemplateDebianSlim, config.TemplateNix:
			selectedTemplate = templateFlag
		default:
			return fmt.Errorf("unknown template %q, valid options: alpine, debian-mise, debian-slim, nix", templateFlag)
		}
	} else {
		// Interactive template selection
		err := huh.NewSelect[string]().
			Title("Select a template").
			Options(
				huh.NewOption("Alpine - Lightweight Alpine environment with mise", string(config.TemplateAlpine)),
				huh.NewOption("Debian+mise - Pre-built Debian image with mise, build-essential", string(config.TemplateDebianMise)),
				huh.NewOption("Nix - NixOS-based development environment", string(config.TemplateNix)),
				huh.NewOption("Debian slim - Plain Debian with mise installed on first run", string(config.TemplateDebianSlim)),
			).
			Value(&selectedTemplate).
			Run()
		if err != nil {
			return fmt.Errorf("template selection cancelled: %w", err)
		}
	}

	// Generate configuration from template
	tc := config.GetTemplateConfig(config.Template(selectedTemplate))
	if err := config.GenerateConfig(env.Fs, configPath, tc); err != nil {
		return fmt.Errorf("failed to generate configuration: %w", err)
	}

	// Commit the changes (project dir, normally no sudo needed)
	if err := commitWithSudo(ctx, env, tfs, os.Stdout, ""); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	util.ProgressDone(os.Stdout, "Created %s\n", configPath)
	fmt.Println("Edit this file to customize your container settings.")
	return nil
}

// presetEnvAndCacheDir creates the common dependencies for preset operations.
func presetEnvAndCacheDir() (*preset.PresetEnv, string, error) {
	cmdRunner := util.NewCommandRunner()
	fs := afero.NewOsFs()
	env := preset.NewPresetEnv(fs, cmdRunner)

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("getting home directory: %w", err)
	}
	cacheDir := filepath.Join(home, util.AlcatrazDir, "cache-presets")
	return env, cacheDir, nil
}

// runInitPreset handles `alca init git+<url>` — downloads preset files from a git repo.
// See AGD-035 for design decisions.
func runInitPreset(ctx context.Context, cwd string, rawURL string) error {
	env, cacheDir, err := presetEnvAndCacheDir()
	if err != nil {
		return err
	}

	selectFiles := func(files []string) ([]string, error) {
		options := make([]huh.Option[string], len(files))
		for i, f := range files {
			label := f
			if strings.HasSuffix(f, ".example") {
				label += " (example file — .example suffix kept)"
			}
			options[i] = huh.NewOption(label, f)
		}

		var selected []string
		err := huh.NewMultiSelect[string]().
			Title("Select preset files to download").
			Options(options...).
			Value(&selected).
			Run()
		return selected, err
	}

	confirmOverwrite := func(filename string) (bool, error) {
		var overwrite bool
		err := huh.NewConfirm().
			Title(fmt.Sprintf("File %s already exists. Overwrite?", filename)).
			Value(&overwrite).
			Run()
		return overwrite, err
	}

	return preset.RunPresetFlow(ctx, env, cacheDir, rawURL, cwd, selectFiles, confirmOverwrite, os.Stderr)
}

// runInitUpdate handles `alca init --update` — refreshes preset files to latest versions.
// See AGD-035 for design decisions.
func runInitUpdate(ctx context.Context, cwd string) error {
	env, cacheDir, err := presetEnvAndCacheDir()
	if err != nil {
		return err
	}

	return preset.RunUpdateFlow(ctx, env, cacheDir, cwd, os.Stdout)
}
