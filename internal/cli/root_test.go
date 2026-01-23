package cli

import (
	"testing"
)

func TestRootCommandHasSubcommands(t *testing.T) {
	expectedCommands := []string{
		"init",
		"status",
		"up",
		"down",
		"run",
		"experimental",
	}

	actualCommands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		actualCommands[cmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !actualCommands[expected] {
			t.Errorf("expected subcommand %q not found in root command", expected)
		}
	}
}

func TestExperimentalCommandHasReload(t *testing.T) {
	var reloadFound bool
	for _, cmd := range experimentalCmd.Commands() {
		if cmd.Name() == "reload" {
			reloadFound = true
			break
		}
	}

	if !reloadFound {
		t.Error("expected 'reload' subcommand not found in experimental command")
	}
}

func TestRootCommandInfo(t *testing.T) {
	if rootCmd.Use != "alca" {
		t.Errorf("expected root command use to be 'alca', got %q", rootCmd.Use)
	}

	if rootCmd.Short == "" {
		t.Error("root command should have a short description")
	}

	if rootCmd.Long == "" {
		t.Error("root command should have a long description")
	}
}

func TestConfigFilenameConstant(t *testing.T) {
	if ConfigFilename != ".alca.toml" {
		t.Errorf("expected ConfigFilename to be '.alca.toml', got %q", ConfigFilename)
	}
}
