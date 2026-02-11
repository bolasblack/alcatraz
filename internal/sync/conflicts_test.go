package sync

import (
	"fmt"
	"sort"
	"testing"

	"github.com/spf13/afero"
)

// mockSyncSessionClient implements SyncSessionClient for testing.
type mockSyncSessionClient struct {
	listSessionJSONFn  func(sessionName string) ([]byte, error)
	listSyncSessionsFn func(namePrefix string) ([]string, error)
	flushSyncSessionFn func(name string) error
}

func (m *mockSyncSessionClient) ListSessionJSON(sessionName string) ([]byte, error) {
	if m.listSessionJSONFn != nil {
		return m.listSessionJSONFn(sessionName)
	}
	return nil, nil
}

func (m *mockSyncSessionClient) ListSyncSessions(namePrefix string) ([]string, error) {
	if m.listSyncSessionsFn != nil {
		return m.listSyncSessionsFn(namePrefix)
	}
	return nil, nil
}

func (m *mockSyncSessionClient) FlushSyncSession(name string) error {
	if m.flushSyncSessionFn != nil {
		return m.flushSyncSessionFn(name)
	}
	return nil
}

func newTestSyncEnv(sessions SyncSessionClient) *SyncEnv {
	return NewSyncEnv(afero.NewMemMapFs(), nil, sessions)
}

func TestDetectConflicts(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		outputErr  error
		wantCount  int
		wantErr    bool
		checkInfos func(t *testing.T, infos []ConflictInfo)
	}{
		{
			name:      "no conflicts returns empty slice",
			output:    `[{"conflicts":null}]`,
			wantCount: 0,
		},
		{
			name:      "empty session list returns empty slice",
			output:    `[]`,
			wantCount: 0,
		},
		{
			name: "single conflict modified on both sides",
			output: `[{"conflicts":[{
				"root":"src/config.yaml",
				"alphaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}],
				"betaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}]
			}]}]`,
			wantCount: 1,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				if infos[0].Path != "src/config.yaml" {
					t.Errorf("got path %q, want %q", infos[0].Path, "src/config.yaml")
				}
				if infos[0].LocalState != "modified" {
					t.Errorf("got local state %q, want %q", infos[0].LocalState, "modified")
				}
				if infos[0].ContainerState != "modified" {
					t.Errorf("got container state %q, want %q", infos[0].ContainerState, "modified")
				}
				if infos[0].DetectedAt.IsZero() {
					t.Error("expected non-zero DetectedAt")
				}
			},
		},
		{
			name: "multiple conflicts with different states",
			output: `[{"conflicts":[
				{
					"root":"Dockerfile",
					"alphaChanges":[{"path":"","old":null,"new":{"kind":"file"}}],
					"betaChanges":[{"path":"","old":null,"new":{"kind":"file"}}]
				},
				{
					"root":"data",
					"alphaChanges":[{"path":"","old":{"kind":"file"},"new":null}],
					"betaChanges":[{"path":"","old":null,"new":{"kind":"directory"}}]
				},
				{
					"root":"scripts",
					"alphaChanges":[{"path":"scripts/run.sh","old":null,"new":{"kind":"file"}}],
					"betaChanges":[{"path":"scripts/run.sh","old":{"kind":"file"},"new":null}]
				}
			]}]`,
			wantCount: 3,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })

				// Dockerfile: created on both sides
				if infos[0].Path != "Dockerfile" {
					t.Errorf("got path %q, want %q", infos[0].Path, "Dockerfile")
				}
				if infos[0].LocalState != "created" {
					t.Errorf("Dockerfile local: got %q, want %q", infos[0].LocalState, "created")
				}
				if infos[0].ContainerState != "created" {
					t.Errorf("Dockerfile container: got %q, want %q", infos[0].ContainerState, "created")
				}

				// data: deleted locally, directory in container
				if infos[1].Path != "data" {
					t.Errorf("got path %q, want %q", infos[1].Path, "data")
				}
				if infos[1].LocalState != "deleted" {
					t.Errorf("data local: got %q, want %q", infos[1].LocalState, "deleted")
				}
				if infos[1].ContainerState != "directory" {
					t.Errorf("data container: got %q, want %q", infos[1].ContainerState, "directory")
				}

				// scripts/run.sh: created locally, deleted in container
				if infos[2].Path != "scripts/run.sh" {
					t.Errorf("got path %q, want %q", infos[2].Path, "scripts/run.sh")
				}
				if infos[2].LocalState != "created" {
					t.Errorf("scripts/run.sh local: got %q, want %q", infos[2].LocalState, "created")
				}
				if infos[2].ContainerState != "deleted" {
					t.Errorf("scripts/run.sh container: got %q, want %q", infos[2].ContainerState, "deleted")
				}
			},
		},
		{
			name:      "command error returns error",
			outputErr: fmt.Errorf("command failed"),
			wantErr:   true,
		},
		{
			name:    "invalid JSON returns error",
			output:  `not valid json`,
			wantErr: true,
		},
		{
			name:    "valid JSON but not an array returns error",
			output:  `{"conflicts":[]}`,
			wantErr: true,
		},
		{
			name:      "valid JSON array with unexpected fields parses without error",
			output:    `[{"unknown_field": "value"}]`,
			wantCount: 0,
		},
		{
			name: "multiple sessions aggregates conflicts",
			output: `[
				{"conflicts":[{"root":"a.txt","alphaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}],"betaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}]}]},
				{"conflicts":[{"root":"b.txt","alphaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}],"betaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}]}]}
			]`,
			wantCount: 2,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })
				if infos[0].Path != "a.txt" {
					t.Errorf("got path %q, want %q", infos[0].Path, "a.txt")
				}
				if infos[1].Path != "b.txt" {
					t.Errorf("got path %q, want %q", infos[1].Path, "b.txt")
				}
			},
		},
		{
			name: "asymmetric paths between alpha and beta produce separate entries",
			output: `[{"conflicts":[{
				"root":"",
				"alphaChanges":[{"path":"only-local.txt","old":null,"new":{"kind":"file"}}],
				"betaChanges":[{"path":"only-container.txt","old":null,"new":{"kind":"file"}}]
			}]}]`,
			wantCount: 2,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })
				// only-container.txt: no alpha change, beta created
				if infos[0].Path != "only-container.txt" {
					t.Errorf("got path %q, want %q", infos[0].Path, "only-container.txt")
				}
				if infos[0].LocalState != "" {
					t.Errorf("expected empty LocalState for container-only path, got %q", infos[0].LocalState)
				}
				if infos[0].ContainerState != "created" {
					t.Errorf("got container state %q, want %q", infos[0].ContainerState, "created")
				}
				// only-local.txt: alpha created, no beta change
				if infos[1].Path != "only-local.txt" {
					t.Errorf("got path %q, want %q", infos[1].Path, "only-local.txt")
				}
				if infos[1].LocalState != "created" {
					t.Errorf("got local state %q, want %q", infos[1].LocalState, "created")
				}
				if infos[1].ContainerState != "" {
					t.Errorf("expected empty ContainerState for local-only path, got %q", infos[1].ContainerState)
				}
			},
		},
		{
			name: "empty root with non-empty path uses path directly",
			output: `[{"conflicts":[{
				"root":"",
				"alphaChanges":[{"path":"standalone.txt","old":null,"new":{"kind":"file"}}],
				"betaChanges":[{"path":"standalone.txt","old":{"kind":"file"},"new":{"kind":"file"}}]
			}]}]`,
			wantCount: 1,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				if infos[0].Path != "standalone.txt" {
					t.Errorf("got path %q, want %q", infos[0].Path, "standalone.txt")
				}
			},
		},
		{
			name: "both old and new nil produces unknown state",
			output: `[{"conflicts":[{
				"root":"mystery.txt",
				"alphaChanges":[{"path":"","old":null,"new":null}],
				"betaChanges":[{"path":"","old":null,"new":null}]
			}]}]`,
			wantCount: 1,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				if infos[0].LocalState != "unknown" {
					t.Errorf("got local state %q, want %q", infos[0].LocalState, "unknown")
				}
				if infos[0].ContainerState != "unknown" {
					t.Errorf("got container state %q, want %q", infos[0].ContainerState, "unknown")
				}
			},
		},
		{
			name: "symlink entry kind treated as modified when old exists",
			output: `[{"conflicts":[{
				"root":"link.txt",
				"alphaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"symlink"}}],
				"betaChanges":[{"path":"","old":{"kind":"symlink"},"new":{"kind":"file"}}]
			}]}]`,
			wantCount: 1,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				if infos[0].LocalState != "modified" {
					t.Errorf("got local state %q, want %q", infos[0].LocalState, "modified")
				}
				if infos[0].ContainerState != "modified" {
					t.Errorf("got container state %q, want %q", infos[0].ContainerState, "modified")
				}
			},
		},
		{
			name: "old is directory new is file treated as modified not directory",
			output: `[{"conflicts":[{
				"root":"changed",
				"alphaChanges":[{"path":"","old":{"kind":"directory"},"new":{"kind":"file"}}],
				"betaChanges":[{"path":"","old":{"kind":"directory"},"new":{"kind":"file"}}]
			}]}]`,
			wantCount: 1,
			checkInfos: func(t *testing.T, infos []ConflictInfo) {
				t.Helper()
				if infos[0].LocalState != "modified" {
					t.Errorf("got local state %q, want %q", infos[0].LocalState, "modified")
				}
			},
		},
		{
			name: "conflict with empty changes produces no entries",
			output: `[{"conflicts":[{
				"root":"empty",
				"alphaChanges":[],
				"betaChanges":[]
			}]}]`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSyncSessionClient{
				listSessionJSONFn: func(sessionName string) ([]byte, error) {
					if tt.outputErr != nil {
						return nil, tt.outputErr
					}
					return []byte(tt.output), nil
				},
			}

			env := newTestSyncEnv(mock)
			infos, err := env.DetectConflicts("test-session")

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(infos) != tt.wantCount {
				t.Fatalf("got %d conflicts, want %d", len(infos), tt.wantCount)
			}
			if tt.checkInfos != nil {
				tt.checkInfos(t, infos)
			}
		})
	}
}
