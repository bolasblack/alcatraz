package cli

import (
	"testing"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

func TestParseContainerSelection(t *testing.T) {
	orphans := []runtime.ContainerInfo{
		{Name: "alca-project1-abc123"},
		{Name: "alca-project2-def456"},
		{Name: "alca-project3-ghi789"},
	}

	tests := []struct {
		name          string
		input         string
		expectedNames []string
	}{
		{
			name:          "single selection",
			input:         "1",
			expectedNames: []string{"alca-project1-abc123"},
		},
		{
			name:          "multiple selections",
			input:         "1,3",
			expectedNames: []string{"alca-project1-abc123", "alca-project3-ghi789"},
		},
		{
			name:          "selections with spaces",
			input:         "1, 2, 3",
			expectedNames: []string{"alca-project1-abc123", "alca-project2-def456", "alca-project3-ghi789"},
		},
		{
			name:          "duplicate selections",
			input:         "1,1,2",
			expectedNames: []string{"alca-project1-abc123", "alca-project2-def456"},
		},
		{
			name:          "out of range ignored",
			input:         "1,5,2",
			expectedNames: []string{"alca-project1-abc123", "alca-project2-def456"},
		},
		{
			name:          "zero ignored",
			input:         "0,1",
			expectedNames: []string{"alca-project1-abc123"},
		},
		{
			name:          "negative ignored",
			input:         "-1,2",
			expectedNames: []string{"alca-project2-def456"},
		},
		{
			name:          "invalid input ignored",
			input:         "abc,2",
			expectedNames: []string{"alca-project2-def456"},
		},
		{
			name:          "all invalid returns empty",
			input:         "abc,xyz",
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseContainerSelection(tt.input, orphans)

			if len(result) != len(tt.expectedNames) {
				t.Errorf("parseContainerSelection(%q) returned %d items, want %d",
					tt.input, len(result), len(tt.expectedNames))
				return
			}

			for i, c := range result {
				if c.Name != tt.expectedNames[i] {
					t.Errorf("parseContainerSelection(%q)[%d].Name = %q, want %q",
						tt.input, i, c.Name, tt.expectedNames[i])
				}
			}
		})
	}
}

func TestCheckOrphanStatus(t *testing.T) {
	projectID := "test-project-id-12345"
	projectDir := "/test/project"

	env := util.NewTestEnv()

	// Create project directory and state file
	st := &state.State{
		ProjectID:     projectID,
		ContainerName: "alca-" + projectID[:12],
		Runtime:       "Docker",
	}
	if err := state.Save(env, projectDir, st); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	tests := []struct {
		name       string
		container  runtime.ContainerInfo
		wantOrphan bool
		wantReason string
	}{
		{
			name: "valid container with matching state",
			container: runtime.ContainerInfo{
				Name:        "alca-test-project",
				ProjectPath: projectDir,
				ProjectID:   projectID,
			},
			wantOrphan: false,
		},
		{
			name: "no project path",
			container: runtime.ContainerInfo{
				Name:        "alca-no-path",
				ProjectPath: "",
				ProjectID:   "some-id",
			},
			wantOrphan: true,
			wantReason: "no project path label",
		},
		{
			name: "project directory does not exist",
			container: runtime.ContainerInfo{
				Name:        "alca-missing-dir",
				ProjectPath: "/nonexistent/path/12345",
				ProjectID:   "some-id",
			},
			wantOrphan: true,
			wantReason: "project directory does not exist",
		},
		{
			name: "project ID mismatch",
			container: runtime.ContainerInfo{
				Name:        "alca-mismatch",
				ProjectPath: projectDir,
				ProjectID:   "different-project-id",
			},
			wantOrphan: true,
			wantReason: "project ID mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isOrphan, reason := checkOrphanStatus(env, tt.container)

			if isOrphan != tt.wantOrphan {
				t.Errorf("checkOrphanStatus() isOrphan = %v, want %v", isOrphan, tt.wantOrphan)
			}

			if tt.wantOrphan && reason != tt.wantReason {
				t.Errorf("checkOrphanStatus() reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestCheckOrphanStatus_NoStateFile(t *testing.T) {
	env := util.NewTestEnv()

	projectDir := "/test/project-no-state"
	if err := env.Fs.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	container := runtime.ContainerInfo{
		Name:        "alca-no-state",
		ProjectPath: projectDir,
		ProjectID:   "some-project-id",
	}

	isOrphan, reason := checkOrphanStatus(env, container)

	if !isOrphan {
		t.Error("expected container to be orphan when state file is missing")
	}
	if reason != "state file (.alca/state.json) does not exist" {
		t.Errorf("unexpected reason: %q", reason)
	}
}
