package runtime

// MutagenSyncClient implements sync.SyncSessionClient using mutagen CLI.
// See AGD-029 for dependency injection patterns.
type MutagenSyncClient struct {
	env *RuntimeEnv
}

// NewMutagenSyncClient creates a new MutagenSyncClient.
func NewMutagenSyncClient(env *RuntimeEnv) *MutagenSyncClient {
	return &MutagenSyncClient{env: env}
}

func (c *MutagenSyncClient) ListSessionJSON(sessionName string) ([]byte, error) {
	return ListSessionJSON(c.env, sessionName)
}

func (c *MutagenSyncClient) ListSyncSessions(namePrefix string) ([]string, error) {
	return ListMutagenSyncs(c.env, namePrefix)
}

func (c *MutagenSyncClient) FlushSyncSession(name string) error {
	s := MutagenSync{Name: name}
	return s.Flush(c.env)
}
