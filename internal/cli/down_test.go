package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/sync"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

// mockSyncSessionClient implements sync.SyncSessionClient for testing.
type mockSyncSessionClient struct {
	sessions []string
	listErr  error
}

var _ sync.SyncSessionClient = (*mockSyncSessionClient)(nil)

func (m *mockSyncSessionClient) ListSessionJSON(_ context.Context, _ string) ([]byte, error) {
	return []byte("{}"), nil
}

func (m *mockSyncSessionClient) ListSyncSessions(_ context.Context, _ string) ([]string, error) {
	return m.sessions, m.listErr
}

func (m *mockSyncSessionClient) FlushSyncSession(_ context.Context, _ string) error {
	return nil
}

// mockRuntime implements runtime.Runtime for testing cleanupFirewall.
type mockRuntime struct {
	statusResult runtime.ContainerStatus
	statusError  error
}

var _ runtime.Runtime = (*mockRuntime)(nil)

func (m *mockRuntime) Name() string                                            { return "MockRuntime" }
func (m *mockRuntime) Available(_ context.Context, _ *runtime.RuntimeEnv) bool { return true }
func (m *mockRuntime) Down(_ context.Context, _ *runtime.RuntimeEnv, _ string, _ *state.State) error {
	return nil
}
func (m *mockRuntime) Up(_ context.Context, _ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ io.Writer) error {
	return nil
}
func (m *mockRuntime) Exec(_ context.Context, _ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ []string) error {
	return nil
}
func (m *mockRuntime) Reload(_ context.Context, _ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State) error {
	return nil
}
func (m *mockRuntime) ListContainers(_ context.Context, _ *runtime.RuntimeEnv) ([]runtime.ContainerInfo, error) {
	return nil, nil
}
func (m *mockRuntime) RemoveContainer(_ context.Context, _ *runtime.RuntimeEnv, _ string) error {
	return nil
}
func (m *mockRuntime) GetContainerIP(_ context.Context, _ *runtime.RuntimeEnv, _ string) (string, error) {
	return "", nil
}
func (m *mockRuntime) Status(_ context.Context, _ *runtime.RuntimeEnv, _ string, _ *state.State) (runtime.ContainerStatus, error) {
	return m.statusResult, m.statusError
}

func TestCleanupFirewall_NoFirewallAvailable(t *testing.T) {
	// Mock command runner where "which nft" fails → Detect returns TypeNone
	cmd := util.NewMockCommandRunner()
	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	rt := &mockRuntime{}
	st := &state.State{ContainerName: "alca-test"}
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", runtime.PlatformLinux)

	var buf bytes.Buffer
	err := cleanupFirewall(context.Background(), networkEnv, env, tfs, runtimeEnv, rt, st, &buf)

	if err != nil {
		t.Errorf("expected nil error when no firewall available, got: %v", err)
	}
}

func TestCleanupFirewall_StatusError(t *testing.T) {
	// Mock command runner where nft is available but runtime.Status fails
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess("which nft", []byte("/usr/sbin/nft"))
	cmd.ExpectSuccess("nft list tables", []byte(""))

	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	rt := &mockRuntime{
		statusError: fmt.Errorf("container not reachable"),
	}
	st := &state.State{ContainerName: "alca-test"}
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", runtime.PlatformLinux)

	var buf bytes.Buffer
	err := cleanupFirewall(context.Background(), networkEnv, env, tfs, runtimeEnv, rt, st, &buf)

	// Status error causes early return nil (not propagated)
	if err != nil {
		t.Errorf("expected nil error when status fails, got: %v", err)
	}
}

func TestCleanupFirewall_ContainerNotFound(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess("which nft", []byte("/usr/sbin/nft"))
	cmd.ExpectSuccess("nft list tables", []byte(""))

	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	rt := &mockRuntime{
		statusResult: runtime.ContainerStatus{
			State: runtime.StateNotFound,
		},
	}
	st := &state.State{ContainerName: "alca-test"}
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", runtime.PlatformLinux)

	var buf bytes.Buffer
	err := cleanupFirewall(context.Background(), networkEnv, env, tfs, runtimeEnv, rt, st, &buf)

	// StateNotFound causes early return nil
	if err != nil {
		t.Errorf("expected nil error when container not found, got: %v", err)
	}
}

func TestGuardSyncConflicts_BlocksWhenConflictsExist(t *testing.T) {
	fs := afero.NewMemMapFs()
	projectRoot := "/tmp/test-project"

	// Write cache with conflicts
	cache := &sync.CacheData{
		UpdatedAt: time.Now(),
		Conflicts: []sync.ConflictInfo{
			{Path: "src/config.yaml", LocalState: "modified", ContainerState: "modified"},
		},
	}
	if err := sync.WriteCache(fs, projectRoot, cache); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	var buf bytes.Buffer
	err := guardSyncConflicts(context.Background(), fs, nil, projectRoot, "test-id", false, &buf)

	if err == nil {
		t.Fatal("expected error when conflicts exist, got nil")
	}
	if !strings.Contains(err.Error(), "resolve sync conflicts") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !strings.Contains(buf.String(), "sync") {
		t.Errorf("expected banner output, got: %q", buf.String())
	}
}

func TestGuardSyncConflicts_ForceBypassesCheck(t *testing.T) {
	fs := afero.NewMemMapFs()
	projectRoot := "/tmp/test-project"

	// Write cache with conflicts
	cache := &sync.CacheData{
		UpdatedAt: time.Now(),
		Conflicts: []sync.ConflictInfo{
			{Path: "src/config.yaml", LocalState: "modified", ContainerState: "modified"},
		},
	}
	if err := sync.WriteCache(fs, projectRoot, cache); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	var buf bytes.Buffer
	err := guardSyncConflicts(context.Background(), fs, nil, projectRoot, "test-id", true, &buf)

	if err != nil {
		t.Errorf("expected no error with --force, got: %v", err)
	}
	if !strings.Contains(buf.String(), "Warning") {
		t.Errorf("expected warning output with --force, got: %q", buf.String())
	}
}

func TestGuardSyncConflicts_ProceedsWhenNoConflicts(t *testing.T) {
	fs := afero.NewMemMapFs()
	projectRoot := "/tmp/test-project"

	// Write cache with no conflicts
	cache := &sync.CacheData{
		UpdatedAt: time.Now(),
		Conflicts: []sync.ConflictInfo{},
	}
	if err := sync.WriteCache(fs, projectRoot, cache); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	var buf bytes.Buffer
	err := guardSyncConflicts(context.Background(), fs, nil, projectRoot, "test-id", false, &buf)

	if err != nil {
		t.Errorf("expected no error when no conflicts, got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when no conflicts, got: %q", buf.String())
	}
}

func TestCheckSyncConflictsBeforeDown_FallsBackToPoll(t *testing.T) {
	fs := afero.NewMemMapFs()
	projectRoot := "/tmp/test-project"

	// No cache file exists — should fall back to synchronous poll
	mockClient := &mockSyncSessionClient{} // returns empty sessions
	syncEnv := sync.NewSyncEnv(fs, util.NewMockCommandRunner(), mockClient)

	conflicts := checkSyncConflictsBeforeDown(context.Background(), fs, syncEnv, projectRoot, "test-id")

	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts from empty poll, got %d", len(conflicts))
	}
}
