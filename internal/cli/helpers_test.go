package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/transact"
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

	if deps.Tfs == nil {
		t.Error("Tfs should not be nil")
	}
	if deps.CmdRunner == nil {
		t.Error("CmdRunner should not be nil")
	}
	if deps.Env == nil {
		t.Error("Env should not be nil")
	}
	if deps.RuntimeEnv == nil {
		t.Error("RuntimeEnv should not be nil")
	}

	// Env.Fs should be the TransactFs instance
	if deps.Env.Fs != deps.Tfs {
		t.Error("Env.Fs should be the same TransactFs instance as deps.Tfs")
	}

	// Env.Cmd should be the same CommandRunner
	if deps.Env.Cmd != deps.CmdRunner {
		t.Error("Env.Cmd should be the same CommandRunner instance as deps.CmdRunner")
	}

	// RuntimeEnv.Cmd should be the same CommandRunner
	if deps.RuntimeEnv.Cmd != deps.CmdRunner {
		t.Error("RuntimeEnv.Cmd should be the same CommandRunner instance as deps.CmdRunner")
	}

	// Tfs should be a TransactFs (verify by type assertion)
	if _, ok := deps.Env.Fs.(*transact.TransactFs); !ok {
		t.Error("Env.Fs should be a *transact.TransactFs")
	}
}

func TestNewCLIReadDeps(t *testing.T) {
	deps := newCLIReadDeps()

	if deps.CmdRunner == nil {
		t.Error("CmdRunner should not be nil")
	}
	if deps.Env == nil {
		t.Error("Env should not be nil")
	}
	if deps.RuntimeEnv == nil {
		t.Error("RuntimeEnv should not be nil")
	}

	// Env.Cmd should be the same CommandRunner
	if deps.Env.Cmd != deps.CmdRunner {
		t.Error("Env.Cmd should be the same CommandRunner instance as deps.CmdRunner")
	}

	// RuntimeEnv.Cmd should be the same CommandRunner
	if deps.RuntimeEnv.Cmd != deps.CmdRunner {
		t.Error("RuntimeEnv.Cmd should be the same CommandRunner instance as deps.CmdRunner")
	}

	// Env.Fs should be read-only (wrapped in afero.ReadOnlyFs)
	if _, ok := deps.Env.Fs.(*afero.ReadOnlyFs); !ok {
		t.Error("Env.Fs should be a *afero.ReadOnlyFs for read-only commands")
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
