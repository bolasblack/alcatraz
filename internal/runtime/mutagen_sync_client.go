package runtime

import (
	"context"

	"github.com/bolasblack/alcatraz/internal/util"
)

// MutagenSyncClient implements sync.SyncSessionClient using mutagen CLI.
// See AGD-029 for dependency injection patterns.
type MutagenSyncClient struct {
	env *RuntimeEnv
}

// NewMutagenSyncClient creates a new MutagenSyncClient.
func NewMutagenSyncClient(env *RuntimeEnv) *MutagenSyncClient {
	return &MutagenSyncClient{env: env}
}

func (c *MutagenSyncClient) ListSessionJSON(ctx context.Context, sessionName string) ([]byte, error) {
	return ListSessionJSON(c.envWithContext(ctx), sessionName)
}

func (c *MutagenSyncClient) ListSyncSessions(ctx context.Context, namePrefix string) ([]string, error) {
	return ListMutagenSyncs(c.envWithContext(ctx), namePrefix)
}

func (c *MutagenSyncClient) FlushSyncSession(ctx context.Context, name string) error {
	s := MutagenSync{Name: name}
	return s.Flush(c.envWithContext(ctx))
}

// envWithContext returns a RuntimeEnv whose CommandRunner respects the given context.
// When ctx has no cancellation (e.g. context.Background()), the original env is returned as-is.
func (c *MutagenSyncClient) envWithContext(ctx context.Context) *RuntimeEnv {
	if ctx.Done() == nil {
		return c.env
	}
	if ccr, ok := c.env.Cmd.(*util.ContextCommandRunner); ok {
		return NewRuntimeEnv(ccr.WithContext(ctx))
	}
	return c.env
}
