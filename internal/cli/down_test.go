package cli

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

// mockRuntime implements runtime.Runtime for testing cleanupFirewall.
type mockRuntime struct {
	statusResult runtime.ContainerStatus
	statusError  error
}

var _ runtime.Runtime = (*mockRuntime)(nil)

func (m *mockRuntime) Name() string                         { return "MockRuntime" }
func (m *mockRuntime) Available(_ *runtime.RuntimeEnv) bool { return true }
func (m *mockRuntime) Down(_ *runtime.RuntimeEnv, _ string, _ *state.State) error {
	return nil
}
func (m *mockRuntime) Up(_ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ io.Writer) error {
	return nil
}
func (m *mockRuntime) Exec(_ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ []string) error {
	return nil
}
func (m *mockRuntime) Reload(_ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State) error {
	return nil
}
func (m *mockRuntime) ListContainers(_ *runtime.RuntimeEnv) ([]runtime.ContainerInfo, error) {
	return nil, nil
}
func (m *mockRuntime) RemoveContainer(_ *runtime.RuntimeEnv, _ string) error {
	return nil
}
func (m *mockRuntime) GetContainerIP(_ *runtime.RuntimeEnv, _ string) (string, error) {
	return "", nil
}
func (m *mockRuntime) Status(_ *runtime.RuntimeEnv, _ string, _ *state.State) (runtime.ContainerStatus, error) {
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
	err := cleanupFirewall(networkEnv, env, tfs, runtimeEnv, rt, st, &buf)

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
	err := cleanupFirewall(networkEnv, env, tfs, runtimeEnv, rt, st, &buf)

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
	err := cleanupFirewall(networkEnv, env, tfs, runtimeEnv, rt, st, &buf)

	// StateNotFound causes early return nil
	if err != nil {
		t.Errorf("expected nil error when container not found, got: %v", err)
	}
}
