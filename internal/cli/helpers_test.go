package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
)

func TestDisplayConfigDrift(t *testing.T) {
	tests := []struct {
		name           string
		drift          *state.DriftChanges
		runtimeChanged bool
		oldRuntime     string
		newRuntime     string
		wantOutput     bool
		wantContains   []string
	}{
		{
			name:           "no drift",
			drift:          nil,
			runtimeChanged: false,
			wantOutput:     false,
		},
		{
			name:           "runtime changed only",
			drift:          nil,
			runtimeChanged: true,
			oldRuntime:     "Docker",
			newRuntime:     "Podman",
			wantOutput:     true,
			wantContains:   []string{"Runtime: Docker → Podman"},
		},
		{
			name: "image changed",
			drift: &state.DriftChanges{
				Image: &[2]string{"ubuntu:20.04", "ubuntu:22.04"},
			},
			wantOutput:   true,
			wantContains: []string{"Image: ubuntu:20.04 → ubuntu:22.04"},
		},
		{
			name: "workdir changed",
			drift: &state.DriftChanges{
				Workdir: &[2]string{"/app", "/workspace"},
			},
			wantOutput:   true,
			wantContains: []string{"Workdir: /app → /workspace"},
		},
		{
			name: "mounts changed",
			drift: &state.DriftChanges{
				Mounts: true,
			},
			wantOutput:   true,
			wantContains: []string{"Mounts: changed"},
		},
		{
			name: "command up changed",
			drift: &state.DriftChanges{
				CommandUp: &[2]string{"sleep infinity", "tail -f /dev/null"},
			},
			wantOutput:   true,
			wantContains: []string{"Commands.up: changed"},
		},
		{
			name: "memory changed",
			drift: &state.DriftChanges{
				Memory: &[2]string{"1g", "2g"},
			},
			wantOutput:   true,
			wantContains: []string{"Resources.memory: 1g → 2g"},
		},
		{
			name: "cpus changed",
			drift: &state.DriftChanges{
				CPUs: &[2]int{2, 4},
			},
			wantOutput:   true,
			wantContains: []string{"Resources.cpus: 2 → 4"},
		},
		{
			name: "envs changed",
			drift: &state.DriftChanges{
				Envs: true,
			},
			wantOutput:   true,
			wantContains: []string{"Envs: changed"},
		},
		{
			name: "multiple changes",
			drift: &state.DriftChanges{
				Image:  &[2]string{"old", "new"},
				Mounts: true,
			},
			runtimeChanged: true,
			oldRuntime:     "Docker",
			newRuntime:     "Podman",
			wantOutput:     true,
			wantContains: []string{
				"Configuration has changed",
				"Runtime: Docker → Podman",
				"Image: old → new",
				"Mounts: changed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			result := displayConfigDrift(&buf, tt.drift, tt.runtimeChanged, tt.oldRuntime, tt.newRuntime)

			if result != tt.wantOutput {
				t.Errorf("displayConfigDrift() returned %v, want %v", result, tt.wantOutput)
			}

			output := buf.String()
			if tt.wantOutput && output == "" {
				t.Error("expected output but got empty string")
			}
			if !tt.wantOutput && output != "" {
				t.Errorf("expected no output but got: %s", output)
			}

			for _, want := range tt.wantContains {
				if !bytes.Contains(buf.Bytes(), []byte(want)) {
					t.Errorf("output missing expected string %q\nGot: %s", want, output)
				}
			}
		})
	}
}

func TestNewCLIDeps(t *testing.T) {
	deps := newCLIDeps()

	// Writes through Env.Fs are staged (transactional), not written to real disk
	testPath := "/tmp/alca-test-staged-write"
	realFs := afero.NewOsFs()
	defer realFs.Remove(testPath)

	err := afero.WriteFile(deps.Env.Fs, testPath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("write through Env.Fs should succeed (staged): %v", err)
	}

	if _, err := realFs.Stat(testPath); err == nil {
		t.Error("staged write should not appear on real filesystem before commit")
	}
}

func TestNewCLIReadDeps(t *testing.T) {
	deps := newCLIReadDeps()

	// Writes through Env.Fs should be rejected (read-only)
	err := afero.WriteFile(deps.Env.Fs, "/tmp/alca-test-readonly-write", []byte("test"), 0644)
	if err == nil {
		t.Error("write through Env.Fs should fail (read-only)")
	}
}

func TestProgressFunc(t *testing.T) {
	t.Run("writes formatted output to writer", func(t *testing.T) {
		var buf bytes.Buffer
		pf := progressFunc(&buf)

		pf("Installing %s version %d...\n", "package", 2)

		output := buf.String()
		if !strings.Contains(output, "Installing package version 2...") {
			t.Errorf("expected formatted progress output, got: %q", output)
		}
	})

	t.Run("includes step prefix from ProgressStep", func(t *testing.T) {
		var buf bytes.Buffer
		pf := progressFunc(&buf)

		pf("hello")

		output := buf.String()
		// ProgressStep prepends "→ " prefix
		if !strings.HasPrefix(output, "→ ") {
			t.Errorf("expected output to start with '→ ' prefix, got: %q", output)
		}
	})

	t.Run("nil writer does not panic", func(t *testing.T) {
		pf := progressFunc(nil)
		// Should not panic
		pf("test %s", "message")
	})

	t.Run("multiple calls append to writer", func(t *testing.T) {
		var buf bytes.Buffer
		pf := progressFunc(&buf)

		pf("first\n")
		pf("second\n")

		output := buf.String()
		if !strings.Contains(output, "first") || !strings.Contains(output, "second") {
			t.Errorf("expected both messages in output, got: %q", output)
		}
	})
}

// pathCheckMockRuntime implements runtime.Runtime for testing checkProjectPathConsistency.
type pathCheckMockRuntime struct {
	containers []runtime.ContainerInfo
	listErr    error
}

var _ runtime.Runtime = (*pathCheckMockRuntime)(nil)

func (m *pathCheckMockRuntime) Name() string { return "MockRuntime" }
func (m *pathCheckMockRuntime) Available(_ context.Context, _ *runtime.RuntimeEnv) bool {
	return true
}
func (m *pathCheckMockRuntime) Down(_ context.Context, _ *runtime.RuntimeEnv, _ string, _ *state.State) error {
	return nil
}
func (m *pathCheckMockRuntime) Up(_ context.Context, _ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ io.Writer) error {
	return nil
}
func (m *pathCheckMockRuntime) Exec(_ context.Context, _ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ []string) error {
	return nil
}
func (m *pathCheckMockRuntime) Reload(_ context.Context, _ *runtime.RuntimeEnv, _ *config.Config, _ string, _ *state.State) error {
	return nil
}
func (m *pathCheckMockRuntime) ListContainers(_ context.Context, _ *runtime.RuntimeEnv) ([]runtime.ContainerInfo, error) {
	return m.containers, m.listErr
}
func (m *pathCheckMockRuntime) RemoveContainer(_ context.Context, _ *runtime.RuntimeEnv, _ string) error {
	return nil
}
func (m *pathCheckMockRuntime) GetContainerIP(_ context.Context, _ *runtime.RuntimeEnv, _ string) (string, error) {
	return "", nil
}
func (m *pathCheckMockRuntime) Status(_ context.Context, _ *runtime.RuntimeEnv, _ string, _ *state.State) (runtime.ContainerStatus, error) {
	return runtime.ContainerStatus{}, nil
}

func TestCheckProjectPathConsistency(t *testing.T) {
	ctx := context.Background()
	runtimeEnv := &runtime.RuntimeEnv{}
	projectID := "test-project-id"

	t.Run("paths match returns no error", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{
				{ProjectID: projectID, ProjectPath: "/home/user/myproject"},
			},
		}
		st := &state.State{ProjectID: projectID}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/myproject", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("path mismatch returns error", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{
				{ProjectID: projectID, ProjectPath: "/home/user/old-path"},
			},
		}
		st := &state.State{ProjectID: projectID}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/new-path", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "/home/user/old-path") {
			t.Errorf("error should mention old path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "/home/user/new-path") {
			t.Errorf("error should mention new path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "alca down") {
			t.Errorf("error should mention 'alca down', got: %v", err)
		}
	})

	t.Run("no container found returns no error", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{
				{ProjectID: "other-project", ProjectPath: "/somewhere/else"},
			},
		}
		st := &state.State{ProjectID: projectID}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/myproject", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("nil state returns no error", func(t *testing.T) {
		rt := &pathCheckMockRuntime{}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, nil, "/home/user/myproject", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("mismatch with mutagen config includes sync warning", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{
				{ProjectID: projectID, ProjectPath: "/home/user/old-path"},
			},
		}
		st := &state.State{ProjectID: projectID}
		cfg := &config.Config{
			WorkdirExclude: []string{"node_modules"},
		}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/new-path", cfg)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mutagen") {
			t.Errorf("error should mention mutagen, got: %v", err)
		}
	})

	t.Run("mismatch with mount excludes includes sync warning", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{
				{ProjectID: projectID, ProjectPath: "/home/user/old-path"},
			},
		}
		st := &state.State{ProjectID: projectID}
		cfg := &config.Config{
			Mounts: []config.MountConfig{
				{Source: ".", Target: "/app", Exclude: []string{".git"}},
			},
		}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/new-path", cfg)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mutagen") {
			t.Errorf("error should mention mutagen, got: %v", err)
		}
	})

	t.Run("mismatch without mutagen does not include sync warning", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{
				{ProjectID: projectID, ProjectPath: "/home/user/old-path"},
			},
		}
		st := &state.State{ProjectID: projectID}
		cfg := &config.Config{}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/new-path", cfg)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if strings.Contains(err.Error(), "mutagen") {
			t.Errorf("error should NOT mention mutagen, got: %v", err)
		}
	})

	t.Run("list containers error is propagated", func(t *testing.T) {
		rt := &pathCheckMockRuntime{listErr: errors.New("connection refused")}
		st := &state.State{ProjectID: projectID}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/any", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("error should contain original cause, got: %v", err)
		}
	})

	t.Run("empty container list returns no error", func(t *testing.T) {
		rt := &pathCheckMockRuntime{
			containers: []runtime.ContainerInfo{},
		}
		st := &state.State{ProjectID: projectID}

		err := checkProjectPathConsistency(ctx, runtimeEnv, rt, st, "/home/user/myproject", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}
