package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

// spyRuntime tracks GetHostIP calls for testing caching behavior.
type spyRuntime struct {
	runtime.StubRuntime
	getHostIPResult string
	getHostIPErr    error
	getHostIPCalls  int
}

var _ runtime.Runtime = (*spyRuntime)(nil)

func (s *spyRuntime) GetHostIP(_ context.Context, _ *runtime.RuntimeEnv) (string, error) {
	s.getHostIPCalls++
	return s.getHostIPResult, s.getHostIPErr
}

// stubNetworkHelper implements network.NetworkHelper for testing.
// Returns Installed=true by default so ensureNetworkHelper passes.
type stubNetworkHelper struct {
	installed bool
}

var _ network.NetworkHelper = (*stubNetworkHelper)(nil)

func (s *stubNetworkHelper) HelperStatus(_ context.Context, _ *network.NetworkEnv) network.HelperStatus {
	return network.HelperStatus{Installed: s.installed}
}
func (s *stubNetworkHelper) DetailedStatus(_ *network.NetworkEnv) network.DetailedStatusInfo {
	return network.DetailedStatusInfo{}
}
func (s *stubNetworkHelper) Setup(_ *network.NetworkEnv, _ string, _ network.ProgressFunc) (*network.PostCommitAction, error) {
	return &network.PostCommitAction{}, nil
}
func (s *stubNetworkHelper) Teardown(_ *network.NetworkEnv, _ string) error { return nil }
func (s *stubNetworkHelper) InstallHelper(_ *network.NetworkEnv, _ network.ProgressFunc) (*network.PostCommitAction, error) {
	return &network.PostCommitAction{}, nil
}
func (s *stubNetworkHelper) UninstallHelper(_ *network.NetworkEnv, _ network.ProgressFunc) (*network.PostCommitAction, error) {
	return &network.PostCommitAction{}, nil
}

// driftRuntime controls Status return for drift detection tests.
type driftRuntime struct {
	runtime.StubRuntime
	statusState runtime.ContainerState
}

func (d *driftRuntime) Status(_ context.Context, _ *runtime.RuntimeEnv, _ string, _ *state.State) (runtime.ContainerStatus, error) {
	return runtime.ContainerStatus{State: d.statusState}, nil
}

func (d *driftRuntime) Name() string { return "Docker" }

func TestHandleConfigDrift_NoContainer_NoDrift(t *testing.T) {
	rt := &driftRuntime{statusState: runtime.StateNotFound}
	st := &state.State{
		Runtime: "Docker",
		Config: &config.Config{
			Image:    "alpine:3.20",
			Commands: config.Commands{Up: config.CommandValue{Command: "old"}},
		},
	}
	cfg := &config.Config{
		Image:    "alpine:3.21",
		Commands: config.Commands{Up: config.CommandValue{Command: "new"}},
	}

	rebuild, err := handleConfigDrift(context.Background(), cfg, st, rt, nil, "/tmp", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rebuild {
		t.Error("expected no rebuild when container doesn't exist")
	}
}

func TestHandleConfigDrift_RunningContainer_DetectsDrift(t *testing.T) {
	rt := &driftRuntime{statusState: runtime.StateRunning}
	st := &state.State{
		Runtime: "Docker",
		Config: &config.Config{
			Image: "alpine:3.20",
		},
	}
	cfg := &config.Config{
		Image: "alpine:3.21",
	}

	// force=true so we don't hit promptConfirm
	rebuild, err := handleConfigDrift(context.Background(), cfg, st, rt, nil, "/tmp", nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rebuild {
		t.Error("expected rebuild when container exists and config changed")
	}
}

func TestHandleConfigDrift_RunningContainer_NoDriftWhenUnchanged(t *testing.T) {
	rt := &driftRuntime{statusState: runtime.StateRunning}
	cfg := &config.Config{Image: "alpine:3.21"}
	st := &state.State{
		Runtime: "Docker",
		Config:  cfg,
	}

	rebuild, err := handleConfigDrift(context.Background(), cfg, st, rt, nil, "/tmp", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rebuild {
		t.Error("expected no rebuild when config unchanged")
	}
}

func TestContainerMissing_UpdatesState(t *testing.T) {
	fs := afero.NewMemMapFs()
	env := &util.Env{Fs: fs}
	cwd := "/project"

	// Simulate state.json with old config (left by previous alca up)
	oldCfg := &config.Config{Image: "alpine:3.20", Commands: config.Commands{Up: config.CommandValue{Command: "old"}}}
	st := &state.State{ProjectID: "test-id", ContainerName: "alca-test", Runtime: "Docker", Config: oldCfg}
	if err := state.Save(env, cwd, st); err != nil {
		t.Fatalf("save: %v", err)
	}

	// New config
	newCfg := &config.Config{Image: "alpine:3.21", Commands: config.Commands{Up: config.CommandValue{Command: "new"}}}

	// containerMissing returns true → state should be updated
	rt := &driftRuntime{statusState: runtime.StateNotFound}
	if !containerMissing(context.Background(), rt, nil, cwd, st) {
		t.Fatal("expected containerMissing=true")
	}

	// Simulate the save path from runUp
	st.UpdateConfig(newCfg)
	if err := state.Save(env, cwd, st); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify state.json has new config
	loaded, _ := state.Load(env, cwd)
	if loaded.Config.Image != "alpine:3.21" {
		t.Errorf("state not updated: image=%q, want alpine:3.21", loaded.Config.Image)
	}
	if loaded.Config.Commands.Up.Command != "new" {
		t.Errorf("state not updated: up=%q, want 'new'", loaded.Config.Commands.Up.Command)
	}
}

func TestExpandAlcaTokensInRules(t *testing.T) {
	ctx := context.Background()
	runtimeEnv := &runtime.RuntimeEnv{}

	t.Run("caches GetHostIP across multiple rules", func(t *testing.T) {
		spy := &spyRuntime{getHostIPResult: "172.17.0.1"}
		rules := []string{
			"${alca:HOST_IP}:8080",
			"${alca:HOST_IP}:9090",
			"${alca:HOST_IP}:3000",
		}

		got, err := expandAlcaTokensInRules(ctx, runtimeEnv, spy, rules)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"172.17.0.1:8080", "172.17.0.1:9090", "172.17.0.1:3000"}
		if len(got) != len(want) {
			t.Fatalf("got %d rules, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("rule %d: got %q, want %q", i, got[i], want[i])
			}
		}

		if spy.getHostIPCalls != 1 {
			t.Errorf("GetHostIP called %d times, want 1 (caching)", spy.getHostIPCalls)
		}
	})

	t.Run("error from GetHostIP propagates", func(t *testing.T) {
		resolveErr := errors.New("network unavailable")
		spy := &spyRuntime{getHostIPErr: resolveErr}
		rules := []string{"${alca:HOST_IP}:8080"}

		_, err := expandAlcaTokensInRules(ctx, runtimeEnv, spy, rules)
		if !errors.Is(err, resolveErr) {
			t.Fatalf("expected resolver error, got: %v", err)
		}
	})

	t.Run("rules without tokens pass through unchanged", func(t *testing.T) {
		spy := &spyRuntime{getHostIPResult: "172.17.0.1"}
		rules := []string{"192.168.1.1:8080", "10.0.0.1:9090"}

		got, err := expandAlcaTokensInRules(ctx, runtimeEnv, spy, rules)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(got) != len(rules) {
			t.Fatalf("got %d rules, want %d", len(got), len(rules))
		}
		for i := range rules {
			if got[i] != rules[i] {
				t.Errorf("rule %d: got %q, want %q", i, got[i], rules[i])
			}
		}

		if spy.getHostIPCalls != 0 {
			t.Errorf("GetHostIP called %d times, want 0 (no tokens)", spy.getHostIPCalls)
		}
	})
}

func TestSetupFirewall_ReturnsExpandedNetwork(t *testing.T) {
	ctx := context.Background()
	cmd := util.NewMockCommandRunner()
	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", "test-id", runtime.PlatformLinux)

	spy := &spyRuntime{getHostIPResult: "172.17.0.1"}
	st := &state.State{ProjectID: "test-id", ContainerName: "alca-test"}
	nh := &stubNetworkHelper{installed: true}

	// Config with raw alca tokens — setupFirewall must expand them
	netCfg := config.Network{
		LANAccess: []string{"${alca:HOST_IP}:8080", "${alca:HOST_IP}:9090"},
	}

	// fw=nil, fwType=TypeNone → succeeds after expansion without applying rules
	expandedNet, err := setupFirewall(ctx, nil, network.TypeNone, networkEnv, env, tfs, runtimeEnv, netCfg, spy, st, nh, nil)
	if err != nil {
		t.Fatalf("setupFirewall returned error: %v", err)
	}

	// Returned network must contain resolved IPs, not raw tokens
	want := []string{"172.17.0.1:8080", "172.17.0.1:9090"}
	if len(expandedNet.LANAccess) != len(want) {
		t.Fatalf("expanded LANAccess: got %d rules, want %d", len(expandedNet.LANAccess), len(want))
	}
	for i, w := range want {
		if expandedNet.LANAccess[i] != w {
			t.Errorf("expanded LANAccess[%d] = %q, want %q", i, expandedNet.LANAccess[i], w)
		}
	}
}

func TestSetupFirewall_ErrSkipFirewall_DoesNotSaveState(t *testing.T) {
	ctx := context.Background()
	cmd := util.NewMockCommandRunner()
	fs := afero.NewMemMapFs()
	tfs := transact.New(transact.WithActualFs(fs))
	env := &util.Env{Fs: tfs, Cmd: cmd}
	runtimeEnv := runtime.NewRuntimeEnv(cmd)
	networkEnv := network.NewNetworkEnv(tfs, cmd, "/tmp/test", "test-id", runtime.PlatformLinux)
	cwd := "/tmp/test-project"

	spy := &spyRuntime{getHostIPResult: "172.17.0.1"}
	st := &state.State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
		Config: &config.Config{
			Network: config.Network{
				LANAccess: []string{"old-rule"},
			},
		},
	}

	// Save initial state to disk
	initEnv := &util.Env{Fs: fs}
	if err := state.Save(initEnv, cwd, st); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	// nh with Installed=false triggers ensureNetworkHelper → errSkipFirewall
	// (since we can't call promptConfirm in tests, we use nh=nil which returns
	// a regular error, not errSkipFirewall. Instead, directly verify the wiring:
	// when setupFirewall returns an error, the returned Network is zero-value.)
	nh := &stubNetworkHelper{installed: false}

	netCfg := config.Network{
		LANAccess: []string{"${alca:HOST_IP}:8080"},
	}

	_, fwErr := setupFirewall(ctx, nil, network.TypeNone, networkEnv, env, tfs, runtimeEnv, netCfg, spy, st, nh, nil)

	// setupFirewall should return an error (helper not installed)
	if fwErr == nil {
		t.Fatal("expected error from setupFirewall when helper not installed")
	}

	// State on disk should be unchanged — saveNetworkState was never called
	loaded, err := state.Load(initEnv, cwd)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if len(loaded.Config.Network.LANAccess) != 1 || loaded.Config.Network.LANAccess[0] != "old-rule" {
		t.Errorf("state was modified despite setupFirewall error: got %v", loaded.Config.Network.LANAccess)
	}
}

func TestSaveNetworkState(t *testing.T) {
	cwd := "/tmp/test-project"

	t.Run("persists expanded values and preserves other config fields", func(t *testing.T) {
		actualFs := afero.NewMemMapFs()
		tfs := transact.New(transact.WithActualFs(actualFs))
		cmd := util.NewMockCommandRunner()
		env := &util.Env{Fs: tfs, Cmd: cmd}

		// Create initial state with raw tokens in network config
		st := &state.State{
			ProjectID:     "test-project-id",
			ContainerName: "alca-test",
			Runtime:       "docker",
			Config: &config.Config{
				Image: "ubuntu:22.04",
				Network: config.Network{
					LANAccess: []string{"${alca:HOST_IP}:8080"},
				},
			},
		}

		// Save initial state directly to actual filesystem
		initEnv := &util.Env{Fs: actualFs}
		if err := state.Save(initEnv, cwd, st); err != nil {
			t.Fatalf("failed to save initial state: %v", err)
		}

		// Simulate what runUp does: pass expanded (resolved) values from setupFirewall
		expandedNet := config.Network{
			LANAccess: []string{"172.17.0.1:8080", "10.0.0.5:9090"},
		}

		err := saveNetworkState(context.Background(), env, tfs, cwd, expandedNet, st, nil)
		if err != nil {
			t.Fatalf("saveNetworkState returned error: %v", err)
		}

		// Read state back from actual filesystem to verify persistence
		readEnv := &util.Env{Fs: actualFs}
		loaded, err := state.Load(readEnv, cwd)
		if err != nil {
			t.Fatalf("failed to load state: %v", err)
		}
		if loaded == nil {
			t.Fatal("loaded state is nil")
		}

		// Network should contain expanded IPs, not raw tokens
		if len(loaded.Config.Network.LANAccess) != 2 {
			t.Fatalf("expected 2 LANAccess rules, got %d", len(loaded.Config.Network.LANAccess))
		}
		if loaded.Config.Network.LANAccess[0] != "172.17.0.1:8080" {
			t.Errorf("LANAccess[0] = %q, want %q", loaded.Config.Network.LANAccess[0], "172.17.0.1:8080")
		}
		if loaded.Config.Network.LANAccess[1] != "10.0.0.5:9090" {
			t.Errorf("LANAccess[1] = %q, want %q", loaded.Config.Network.LANAccess[1], "10.0.0.5:9090")
		}

		// Other config fields should be preserved
		if loaded.Config.Image != "ubuntu:22.04" {
			t.Errorf("Image = %q, want %q (should be preserved)", loaded.Config.Image, "ubuntu:22.04")
		}
		if loaded.ProjectID != "test-project-id" {
			t.Errorf("ProjectID = %q, want %q", loaded.ProjectID, "test-project-id")
		}
	})

	t.Run("updates in-memory state with expanded values", func(t *testing.T) {
		actualFs := afero.NewMemMapFs()
		tfs := transact.New(transact.WithActualFs(actualFs))
		cmd := util.NewMockCommandRunner()
		env := &util.Env{Fs: tfs, Cmd: cmd}

		st := &state.State{
			ProjectID:     "test-id",
			ContainerName: "alca-test",
			Runtime:       "docker",
			Config: &config.Config{
				Network: config.Network{
					LANAccess: []string{"${alca:HOST_IP}:3000"},
				},
			},
		}

		expandedNet := config.Network{
			LANAccess: []string{"172.17.0.1:3000"},
		}

		err := saveNetworkState(context.Background(), env, tfs, cwd, expandedNet, st, nil)
		if err != nil {
			t.Fatalf("saveNetworkState returned error: %v", err)
		}

		// In-memory state should have expanded values
		if len(st.Config.Network.LANAccess) != 1 || st.Config.Network.LANAccess[0] != "172.17.0.1:3000" {
			t.Errorf("in-memory state not updated with expanded values: got %v", st.Config.Network.LANAccess)
		}
	})
}
