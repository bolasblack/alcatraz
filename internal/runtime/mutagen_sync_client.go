package runtime

import (
	"context"
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
	return ListSessionJSON(ctx, c.env, sessionName)
}

func (c *MutagenSyncClient) ListSyncSessions(ctx context.Context, namePrefix string) ([]string, error) {
	return ListMutagenSyncs(ctx, c.env, namePrefix)
}

func (c *MutagenSyncClient) FlushSyncSession(ctx context.Context, name string) error {
	s := MutagenSync{Name: name}
	return s.Flush(ctx, c.env)
}
