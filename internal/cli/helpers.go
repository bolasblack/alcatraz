package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

// Common error messages for CLI commands.
const (
	ErrMsgConfigNotFound = "configuration not found: run 'alca init' first"
	ErrMsgStateNotFound  = "no state file found: run 'alca up' first"
	ErrMsgNotRunning     = "container is not running: run 'alca up' first"
)

// loadConfigFromCwd loads configuration from the current working directory.
// Returns the config and config path, or an error with user-friendly message.
func loadConfigFromCwd(env *util.Env, cwd string) (*config.Config, string, error) {
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, err := config.LoadConfig(env, configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, configPath, errors.New(ErrMsgConfigNotFound)
		}
		return nil, configPath, fmt.Errorf("failed to load config: %w", err)
	}
	return &cfg, configPath, nil
}

// loadConfigOptional loads configuration, returning zero config if not found.
// Use this for commands that can work without a config file.
func loadConfigOptional(env *util.Env, cwd string) (*config.Config, string) {
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, _ := config.LoadConfig(env, configPath)
	return &cfg, configPath
}

// loadConfigAndRuntime loads config and selects the appropriate runtime.
// This is the most common pattern for commands that need both.
func loadConfigAndRuntime(env *util.Env, runtimeEnv *runtime.RuntimeEnv, cwd string) (*config.Config, runtime.Runtime, error) {
	cfg, _, err := loadConfigFromCwd(env, cwd)
	if err != nil {
		return nil, nil, err
	}

	rt, err := runtime.SelectRuntime(runtimeEnv, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select runtime: %w", err)
	}

	return cfg, rt, nil
}

// loadConfigAndRuntimeOptional loads config (optional) and selects runtime.
// Use for commands like 'list' and 'cleanup' that work without config.
func loadConfigAndRuntimeOptional(env *util.Env, runtimeEnv *runtime.RuntimeEnv, cwd string) (*config.Config, runtime.Runtime, error) {
	cfg, _ := loadConfigOptional(env, cwd)

	rt, err := runtime.SelectRuntime(runtimeEnv, cfg)
	if err != nil {
		return cfg, nil, fmt.Errorf("failed to select runtime: %w", err)
	}

	return cfg, rt, nil
}

// loadRequiredState loads state file and returns error if not found.
// Use for commands that require an existing container state.
func loadRequiredState(env *util.Env, cwd string) (*state.State, error) {
	st, err := state.Load(env, cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	if st == nil {
		return nil, errors.New(ErrMsgStateNotFound)
	}
	return st, nil
}

// loadStateOptional loads state file, returning nil without error if not found.
// Use for commands where missing state is acceptable (e.g., down).
func loadStateOptional(env *util.Env, cwd string) (*state.State, error) {
	st, err := state.Load(env, cwd)
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

	_, _ = fmt.Fprintln(w, "Configuration has changed since last container creation:")

	if runtimeChanged {
		_, _ = fmt.Fprintf(w, "  Runtime: %s → %s\n", oldRuntime, newRuntime)
	}

	if drift != nil {
		if drift.Image != nil {
			_, _ = fmt.Fprintf(w, "  Image: %s → %s\n", drift.Image[0], drift.Image[1])
		}
		if drift.Mounts {
			_, _ = fmt.Fprintf(w, "  Mounts: changed\n")
		}
		if drift.Workdir != nil {
			_, _ = fmt.Fprintf(w, "  Workdir: %s → %s\n", drift.Workdir[0], drift.Workdir[1])
		}
		if drift.WorkdirExclude {
			_, _ = fmt.Fprintf(w, "  Workdir exclude: changed\n")
		}
		if drift.CommandUp != nil {
			_, _ = fmt.Fprintf(w, "  Commands.up: changed\n")
		}
		if drift.Memory != nil {
			_, _ = fmt.Fprintf(w, "  Resources.memory: %s → %s\n", drift.Memory[0], drift.Memory[1])
		}
		if drift.CPUs != nil {
			_, _ = fmt.Fprintf(w, "  Resources.cpus: %d → %d\n", drift.CPUs[0], drift.CPUs[1])
		}
		if drift.Envs {
			_, _ = fmt.Fprintf(w, "  Envs: changed\n")
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

// commitWithSudo commits TransactFs changes, intelligently grouping operations
// by sudo requirement and executing each group with the appropriate method.
// If any operation requires sudo, the msg is printed first to explain why.
func commitWithSudo(env *util.Env, tfs *transact.TransactFs, out io.Writer, msg string) error {
	_, err := tfs.Commit(func(ctx transact.CommitContext) (*transact.CommitOpsResult, error) {
		if len(ctx.Ops) == 0 {
			return nil, nil
		}

		// Check if any op needs sudo and output explanation
		if out != nil {
			for _, op := range ctx.Ops {
				if op.NeedSudo {
					if msg != "" {
						util.ProgressStep(out, "%s (sudo required)...\n", msg)
						break
					}

					// Not expected to happen, so we need to print the path
					util.ProgressStep(out, "Writing to %s (sudo required)...\n", op.Path)
				}
			}
		}

		// Group operations by NeedSudo while preserving order
		groups := transact.GroupOpsBySudo(ctx.Ops)

		// Execute each group with the appropriate method
		if err := transact.ExecuteGroupedOps(ctx.BaseFs, groups, env.Cmd.SudoRunScriptQuiet); err != nil {
			return nil, err
		}

		return nil, nil
	})
	return err
}

// commitIfNeeded checks if there are pending changes and commits them with sudo support.
// The msg is only printed if sudo is actually required.
func commitIfNeeded(env *util.Env, tfs *transact.TransactFs, out io.Writer, msg string) error {
	if !tfs.NeedsCommit() {
		return nil
	}
	return commitWithSudo(env, tfs, out, msg)
}

// cliDeps holds shared CLI dependencies for commands that perform writes.
type cliDeps struct {
	Tfs        *transact.TransactFs
	CmdRunner  util.CommandRunner
	Env        *util.Env
	RuntimeEnv *runtime.RuntimeEnv
}

// newCLIDeps creates the shared transactional dependencies used by most CLI commands.
func newCLIDeps() cliDeps {
	tfs := transact.New()
	cmdRunner := util.NewCommandRunner()
	return cliDeps{
		Tfs:        tfs,
		CmdRunner:  cmdRunner,
		Env:        &util.Env{Fs: tfs, Cmd: cmdRunner},
		RuntimeEnv: runtime.NewRuntimeEnv(cmdRunner),
	}
}

// cliReadDeps holds shared CLI dependencies for read-only commands.
type cliReadDeps struct {
	CmdRunner  util.CommandRunner
	Env        *util.Env
	RuntimeEnv *runtime.RuntimeEnv
}

// newCLIReadDeps creates shared dependencies for read-only CLI commands.
func newCLIReadDeps() cliReadDeps {
	cmdRunner := util.NewCommandRunner()
	return cliReadDeps{
		CmdRunner:  cmdRunner,
		Env:        &util.Env{Fs: afero.NewReadOnlyFs(afero.NewOsFs()), Cmd: cmdRunner},
		RuntimeEnv: runtime.NewRuntimeEnv(cmdRunner),
	}
}

// progressFunc returns a progress callback that writes to the given writer.
func progressFunc(w io.Writer) func(format string, args ...any) {
	return func(format string, args ...any) {
		util.ProgressStep(w, format, args...)
	}
}
