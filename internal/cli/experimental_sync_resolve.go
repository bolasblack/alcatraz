package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/sync"
)

var syncResolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Resolve sync conflicts interactively",
	Long:  "Walk through sync conflicts one by one and choose how to resolve each.",
	RunE:  runSyncResolve,
}

func runSyncResolve(cmd *cobra.Command, args []string) error {
	_, _ = fmt.Fprint(cmd.OutOrStderr(), experimentalWarning)
	_, _ = fmt.Fprintln(cmd.OutOrStderr())

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	deps := newCLIReadDeps()
	env, runtimeEnv := deps.Env, deps.RuntimeEnv

	_, rt, err := loadConfigAndRuntime(env, runtimeEnv, cwd)
	if err != nil {
		return err
	}

	st, err := loadRequiredState(env, cwd)
	if err != nil {
		return err
	}

	// Check container is running
	status, err := rt.Status(runtimeEnv, cwd, st)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}
	if status.State != runtime.StateRunning {
		return errors.New(ErrMsgNotRunning)
	}

	// SyncEnv needs a writable fs for conflict resolution (file deletion).
	syncEnv := sync.NewSyncEnv(afero.NewOsFs(), deps.CmdRunner, runtime.NewMutagenSyncClient(runtimeEnv))
	executor := &dockerContainerExecutor{
		command: strings.ToLower(rt.Name()),
		cmd:     deps.CmdRunner,
	}

	// Fresh conflict check
	cacheData, err := sync.SyncUpdateCache(syncEnv, st.ProjectID, cwd)
	if err != nil {
		return fmt.Errorf("failed to check sync conflicts: %w", err)
	}

	if len(cacheData.Conflicts) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No sync conflicts.")
		return nil
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%d sync conflicts found:\n\n", len(cacheData.Conflicts))

	// Delegate to sync module
	_, err = sync.ResolveAllInteractive(sync.ResolveParams{
		Env:         syncEnv,
		Executor:    executor,
		State:       st,
		ProjectRoot: cwd,
		Conflicts:   cacheData.Conflicts,
		PromptFn:    huhResolvePrompt,
		W:           cmd.OutOrStdout(),
	})
	return err
}

// huhResolvePrompt uses charmbracelet/huh for interactive conflict resolution.
func huhResolvePrompt(conflict sync.ConflictInfo, index, total int) (sync.ResolveChoice, error) {
	var choice string
	err := huh.NewSelect[string]().
		Title("How to resolve?").
		Options(
			huh.NewOption("Local overwrites container", "local"),
			huh.NewOption("Container overwrites local", "container"),
			huh.NewOption("Skip", "skip"),
		).
		Value(&choice).
		Run()
	if err != nil {
		return "", err
	}
	return sync.ResolveChoice(choice), nil
}
