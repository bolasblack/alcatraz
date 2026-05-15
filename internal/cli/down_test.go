package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

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
	runtime.StubRuntime
	statusResult runtime.ContainerStatus
	statusError  error
}

var _ runtime.Runtime = (*mockRuntime)(nil)

func (m *mockRuntime) Status(_ context.Context, _ *runtime.RuntimeEnv, _ string, _ *state.State) (runtime.ContainerStatus, error) {
	return m.statusResult, m.statusError
}

func TestCleanupFirewall_NoFirewallAvailable(t *testing.T) {
	// When no firewall is available, fw is nil — cleanupFirewall returns nil immediately
	cmd := util.NewMockCommandRunner()
	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	rt := &mockRuntime{}
	st := &state.State{ContainerName: "alca-test"}

	var buf bytes.Buffer
	err := cleanupFirewall(context.Background(), nil, env, tfs, runtimeEnv, rt, st, &buf)

	if err != nil {
		t.Errorf("expected nil error when no firewall available, got: %v", err)
	}
}

func TestCleanupFirewall_StatusError(t *testing.T) {
	// Mock command runner where nft is available but runtime.Status fails
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess("which nft", []byte("/usr/sbin/nft"))
	cmd.ExpectSuccess("sudo nft list tables", []byte(""))
	defer cmd.AssertAllExpectationsMet(t)

	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	rt := &mockRuntime{
		statusError: fmt.Errorf("container not reachable"),
	}
	st := &state.State{ContainerName: "alca-test"}
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", "", runtime.PlatformLinux)
	fw, _ := network.New(context.Background(), networkEnv)

	var buf bytes.Buffer
	err := cleanupFirewall(context.Background(), fw, env, tfs, runtimeEnv, rt, st, &buf)

	// Status error causes early return nil (not propagated)
	if err != nil {
		t.Errorf("expected nil error when status fails, got: %v", err)
	}
}

func TestCleanupFirewall_ContainerNotFound(t *testing.T) {
	cmd := util.NewMockCommandRunner().AllowUnexpected()

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
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", "", runtime.PlatformLinux)
	fw, _ := network.New(context.Background(), networkEnv)

	var buf bytes.Buffer
	err := cleanupFirewall(context.Background(), fw, env, tfs, runtimeEnv, rt, st, &buf)

	// StateNotFound no longer short-circuits — CleanupForProject runs so the
	// per-project rule file is removed even when the container is gone.
	if err != nil {
		t.Errorf("expected nil error when container not found, got: %v", err)
	}
}

// TestCleanupFirewall_ContainerGoneTriggersProjectCleanup documents the key
// invariant for the nft-file-cleanup fix (see docs_internal/udp-proxy-sidecar-tun-plan.md
// Phase 2): when a container is already gone at `alca down` time, alcatraz must
// still unwind the per-project rule file so its rules don't hijack whoever
// inherits the container's IP next. The actual file-and-table removal is unit
// tested in internal/network/nft; here we just pin the control-flow: Cleanup is
// not called (we have no container ID), CleanupForProject is.
func TestCleanupFirewall_ContainerGoneTriggersProjectCleanup(t *testing.T) {
	cmd := util.NewMockCommandRunner().AllowUnexpected()
	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)

	fw := &trackingFirewall{}
	rt := &mockRuntime{statusResult: runtime.ContainerStatus{State: runtime.StateNotFound}}
	st := &state.State{ProjectID: "proj-gone", ContainerName: "alca-gone"}

	var buf bytes.Buffer
	if err := cleanupFirewall(context.Background(), fw, env, tfs, runtimeEnv, rt, st, &buf); err != nil {
		t.Fatalf("cleanupFirewall() error = %v", err)
	}

	if fw.cleanupCalls != 0 {
		t.Errorf("expected Cleanup not to be called when container is gone, got %d calls", fw.cleanupCalls)
	}
	if fw.cleanupForProjectCalls != 1 {
		t.Errorf("expected CleanupForProject to be called exactly once, got %d", fw.cleanupForProjectCalls)
	}
}

// trackingFirewall records calls to satisfy the control-flow test above.
// Kept local to avoid pulling a package-level test double into the CLI package.
type trackingFirewall struct {
	cleanupCalls           int
	cleanupForProjectCalls int
}

var _ network.Firewall = (*trackingFirewall)(nil)

func (f *trackingFirewall) ApplyRules(_ string, _ string, _ []network.LANAccessRule, _ *network.ProxyConfig) (*network.PostCommitAction, error) {
	return &network.PostCommitAction{}, nil
}

func (f *trackingFirewall) Cleanup(_ string) (*network.PostCommitAction, error) {
	f.cleanupCalls++
	return &network.PostCommitAction{}, nil
}

func (f *trackingFirewall) CleanupForProject(_ context.Context) (*network.PostCommitAction, error) {
	f.cleanupForProjectCalls++
	return &network.PostCommitAction{}, nil
}

func (f *trackingFirewall) CleanupStaleFiles(_ context.Context) (int, error) {
	return 0, nil
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

	if !errors.Is(err, errSyncConflicts) {
		t.Fatalf("expected errSyncConflicts, got: %v", err)
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
