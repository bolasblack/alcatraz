package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/util"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
)

// Common error messages for CLI commands.
const (
	ErrMsgConfigNotFound = "configuration not found: run 'alca init' first"
	ErrMsgStateNotFound  = "no state file found: run 'alca up' first"
	ErrMsgNotRunning     = "container is not running: run 'alca up' first"
)

// loadConfigFromCwd loads configuration from the current working directory.
// Returns the config and config path, or an error with user-friendly message.
func loadConfigFromCwd(cwd string) (*config.Config, string, error) {
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, configPath, fmt.Errorf(ErrMsgConfigNotFound)
		}
		return nil, configPath, fmt.Errorf("failed to load config: %w", err)
	}
	return &cfg, configPath, nil
}

// loadConfigOptional loads configuration, returning zero config if not found.
// Use this for commands that can work without a config file.
func loadConfigOptional(cwd string) (config.Config, string) {
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, _ := config.LoadConfig(configPath)
	return cfg, configPath
}

// loadConfigAndRuntime loads config and selects the appropriate runtime.
// This is the most common pattern for commands that need both.
func loadConfigAndRuntime(cwd string) (*config.Config, runtime.Runtime, error) {
	cfg, _, err := loadConfigFromCwd(cwd)
	if err != nil {
		return nil, nil, err
	}

	rt, err := runtime.SelectRuntime(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select runtime: %w", err)
	}

	return cfg, rt, nil
}

// loadConfigAndRuntimeOptional loads config (optional) and selects runtime.
// Use for commands like 'list' and 'cleanup' that work without config.
func loadConfigAndRuntimeOptional(cwd string) (config.Config, runtime.Runtime, error) {
	cfg, _ := loadConfigOptional(cwd)

	rt, err := runtime.SelectRuntime(&cfg)
	if err != nil {
		return cfg, nil, fmt.Errorf("failed to select runtime: %w", err)
	}

	return cfg, rt, nil
}

// loadRequiredState loads state file and returns error if not found.
// Use for commands that require an existing container state.
func loadRequiredState(cwd string) (*state.State, error) {
	st, err := state.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	if st == nil {
		return nil, fmt.Errorf(ErrMsgStateNotFound)
	}
	return st, nil
}

// loadStateOptional loads state file, returning nil without error if not found.
// Use for commands where missing state is acceptable (e.g., down).
func loadStateOptional(cwd string) (*state.State, error) {
	st, err := state.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	return st, nil
}

// displayConfigDrift prints configuration drift information to the writer.
// Returns true if there was any drift to display.
func displayConfigDrift(w io.Writer, drift *state.DriftChanges, runtimeChanged bool, oldRuntime, newRuntime string) bool {
	if drift == nil && !runtimeChanged {
		return false
	}

	fmt.Fprintln(w, "Configuration has changed since last container creation:")

	if runtimeChanged {
		fmt.Fprintf(w, "  Runtime: %s → %s\n", oldRuntime, newRuntime)
	}

	if drift != nil {
		if drift.Image != nil {
			fmt.Fprintf(w, "  Image: %s → %s\n", drift.Image[0], drift.Image[1])
		}
		if drift.Mounts {
			fmt.Fprintf(w, "  Mounts: changed\n")
		}
		if drift.Workdir != nil {
			fmt.Fprintf(w, "  Workdir: %s → %s\n", drift.Workdir[0], drift.Workdir[1])
		}
		if drift.CommandUp != nil {
			fmt.Fprintf(w, "  Commands.up: changed\n")
		}
		if drift.Memory != nil {
			fmt.Fprintf(w, "  Resources.memory: %s → %s\n", drift.Memory[0], drift.Memory[1])
		}
		if drift.CPUs != nil {
			fmt.Fprintf(w, "  Resources.cpus: %d → %d\n", drift.CPUs[0], drift.CPUs[1])
		}
		if drift.Envs {
			fmt.Fprintf(w, "  Envs: changed\n")
		}
	}

	return true
}

// getCwd returns the current working directory or an error.
func getCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}
	return cwd, nil
}

// promptConfirm prompts the user for confirmation.
func promptConfirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// progress writes a progress message if not in quiet mode.
// Delegates to util.Progress for shared implementation.
var progress = util.Progress

// progressStep writes a progress message with → prefix (step in progress).
// Delegates to util.ProgressStep for shared implementation.
var progressStep = util.ProgressStep

// progressDone writes a progress message with ✓ prefix (step completed).
// Delegates to util.ProgressDone for shared implementation.
var progressDone = util.ProgressDone
