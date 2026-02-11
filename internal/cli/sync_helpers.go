package cli

import (
	"fmt"
	"io"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/sync"
	"github.com/bolasblack/alcatraz/internal/util"
)

// Compile-time assertion: MutagenSyncClient implements SyncSessionClient.
var _ sync.SyncSessionClient = (*runtime.MutagenSyncClient)(nil)

// dockerContainerExecutor implements sync.ContainerExecutor using docker/podman exec.
type dockerContainerExecutor struct {
	command string // CLI command name (e.g., "docker", "podman")
	cmd     util.CommandRunner
}

var _ sync.ContainerExecutor = (*dockerContainerExecutor)(nil)

func (e *dockerContainerExecutor) ExecInContainer(containerID string, cmd []string) error {
	args := append([]string{"exec", containerID}, cmd...)
	output, err := e.cmd.RunQuiet(e.command, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

// showSyncBanner detects conflicts synchronously and shows the banner on w.
// Uses synchronous update because the process exits immediately after status —
// an async goroutine would be killed before it finishes writing the cache.
// Best-effort: errors are logged to w but do not block the command.
func showSyncBanner(syncEnv *sync.SyncEnv, projectID string, projectRoot string, w io.Writer) {
	cache, err := sync.SyncUpdateCache(syncEnv, projectID, projectRoot)
	if err != nil {
		_, _ = fmt.Fprintf(w, "Warning: failed to check sync conflicts: %v\n", err)
		return
	}
	if cache != nil && len(cache.Conflicts) > 0 {
		sync.RenderBanner(cache.Conflicts, w)
	}
}
