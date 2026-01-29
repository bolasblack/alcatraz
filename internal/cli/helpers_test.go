package cli

import (
	"bytes"
	"testing"

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
