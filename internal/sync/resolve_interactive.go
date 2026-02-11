package sync

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

// ResolveChoice represents the user's resolution choice.
type ResolveChoice string

const (
	ResolveChoiceLocal     ResolveChoice = "local"
	ResolveChoiceContainer ResolveChoice = "container"
	ResolveChoiceSkip      ResolveChoice = "skip"
)

// ResolvePromptFunc prompts the user for a resolution choice.
// Returns the choice or an error (e.g. user cancelled with Ctrl+C).
type ResolvePromptFunc func(conflict ConflictInfo, index, total int) (ResolveChoice, error)

// ResolveResult holds the summary of a resolution session.
type ResolveResult struct {
	Resolved int
	Skipped  int
}

// ResolveParams holds the parameters for an interactive conflict resolution session.
type ResolveParams struct {
	Ctx         context.Context
	Env         *SyncEnv
	Executor    ContainerExecutor
	State       *state.State // carries ProjectID, ContainerName, Config.Workdir
	ProjectRoot string       // cwd, not in State
	Conflicts   []ConflictInfo
	PromptFn    ResolvePromptFunc
	W           io.Writer
}

// ResolveAllInteractive walks through conflicts one by one, prompting the user
// for each and executing the chosen resolution.
func ResolveAllInteractive(p ResolveParams) (*ResolveResult, error) {
	w := p.W
	total := len(p.Conflicts)
	result := &ResolveResult{}

	for i, conflict := range p.Conflicts {
		_, _ = fmt.Fprintf(w, "[%d/%d] %s\n", i+1, total, conflict.Path)
		_, _ = fmt.Fprintf(w, "  Local (your machine):  %s\n", conflict.LocalState)
		_, _ = fmt.Fprintf(w, "  Container:             %s\n", conflict.ContainerState)

		choice, err := p.PromptFn(conflict, i, total)
		if err != nil {
			// User cancelled (Ctrl+C)
			_, _ = fmt.Fprintf(w, "\nAborted. %d resolved, %d skipped.\n", result.Resolved, result.Skipped)
			return result, nil
		}

		switch choice {
		case ResolveChoiceLocal:
			containerPath := filepath.Join(p.State.Config.Workdir, conflict.Path)
			if err := ResolveLocal(p.Executor, p.State.ContainerName, containerPath); err != nil {
				_, _ = fmt.Fprintf(w, "  Error: %v\n", err)
				continue
			}
			_, _ = fmt.Fprintln(w, "  Resolved: local overwrites container")
			result.Resolved++

		case ResolveChoiceContainer:
			localPath := filepath.Join(p.ProjectRoot, conflict.Path)
			if err := ResolveContainer(p.Env.Fs, localPath); err != nil {
				_, _ = fmt.Fprintf(w, "  Error: %v\n", err)
				continue
			}
			_, _ = fmt.Fprintln(w, "  Resolved: container overwrites local")
			result.Resolved++

		case ResolveChoiceSkip:
			result.Skipped++
			_, _ = fmt.Fprintln(w)
			continue
		}

		// After each resolution, flush syncs and update cache
		FlushProjectSyncs(p.Ctx, p.Env.Sessions, p.State.ProjectID)
		_, _ = SyncUpdateCache(p.Ctx, p.Env, p.State.ProjectID, p.ProjectRoot)

		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintf(w, "Done: %d resolved, %d skipped.\n", result.Resolved, result.Skipped)
	return result, nil
}

// FlushProjectSyncs flushes all sync sessions for a project.
func FlushProjectSyncs(ctx context.Context, sessions SyncSessionClient, projectID string) {
	if sessions == nil {
		return
	}
	names, err := sessions.ListSyncSessions(ctx, util.MutagenSessionPrefix(projectID))
	if err != nil {
		return
	}
	for _, name := range names {
		_ = sessions.FlushSyncSession(ctx, name)
	}
}
