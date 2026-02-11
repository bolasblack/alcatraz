package sync

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRenderBanner(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		conflicts  []ConflictInfo
		wantEmpty  bool
		wantChecks []func(t *testing.T, output string)
	}{
		{
			name:      "empty conflicts produces no output",
			conflicts: nil,
			wantEmpty: true,
		},
		{
			name: "single conflict uses singular form",
			conflicts: []ConflictInfo{
				{Path: "src/main.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "1 sync conflict need") {
						t.Errorf("expected singular 'conflict', got: %s", output)
					}
					if strings.Contains(output, "conflicts") {
						t.Errorf("expected singular, not plural: %s", output)
					}
				},
				func(t *testing.T, output string) {
					if !strings.Contains(output, "src/main.go") {
						t.Errorf("expected path in output: %s", output)
					}
				},
			},
		},
		{
			name: "three conflicts all shown with plural form",
			conflicts: []ConflictInfo{
				{Path: "a.go", LocalState: "modified", ContainerState: "deleted", DetectedAt: now},
				{Path: "b.go", LocalState: "created", ContainerState: "modified", DetectedAt: now},
				{Path: "c.go", LocalState: "deleted", ContainerState: "created", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "3 sync conflicts") {
						t.Errorf("expected plural '3 sync conflicts', got: %s", output)
					}
				},
				func(t *testing.T, output string) {
					for _, path := range []string{"a.go", "b.go", "c.go"} {
						if !strings.Contains(output, path) {
							t.Errorf("expected path %s in output", path)
						}
					}
				},
				func(t *testing.T, output string) {
					if strings.Contains(output, "...and") {
						t.Errorf("should not show 'and more' for exactly 3: %s", output)
					}
				},
			},
		},
		{
			name: "five conflicts shows first three plus overflow",
			conflicts: []ConflictInfo{
				{Path: "1.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
				{Path: "2.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
				{Path: "3.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
				{Path: "4.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
				{Path: "5.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "5 sync conflicts") {
						t.Errorf("expected '5 sync conflicts', got: %s", output)
					}
				},
				func(t *testing.T, output string) {
					if !strings.Contains(output, "...and 2 more") {
						t.Errorf("expected '...and 2 more', got: %s", output)
					}
				},
				func(t *testing.T, output string) {
					for _, path := range []string{"1.go", "2.go", "3.go"} {
						if !strings.Contains(output, path) {
							t.Errorf("expected path %s in output", path)
						}
					}
				},
				func(t *testing.T, output string) {
					if strings.Contains(output, "4.go") || strings.Contains(output, "5.go") {
						t.Errorf("should not show paths beyond first 3: %s", output)
					}
				},
			},
		},
		{
			name: "non-TTY output strips ANSI codes",
			conflicts: []ConflictInfo{
				{Path: "x.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if strings.Contains(output, "\033[") {
						t.Errorf("expected no ANSI escape codes for non-TTY writer: %s", output)
					}
				},
			},
		},
		{
			name: "output contains actionable command",
			conflicts: []ConflictInfo{
				{Path: "x.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "alca experimental sync resolve") {
						t.Errorf("expected resolve command in output: %s", output)
					}
				},
			},
		},
		{
			name: "conflict descriptions in output",
			conflicts: []ConflictInfo{
				{Path: "same.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
				{Path: "diff.go", LocalState: "created", ContainerState: "deleted", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "modified on both sides") {
						t.Errorf("expected 'modified on both sides': %s", output)
					}
				},
				func(t *testing.T, output string) {
					if !strings.Contains(output, "created locally, deleted in container") {
						t.Errorf("expected 'created locally, deleted in container': %s", output)
					}
				},
			},
		},
		{
			name:      "empty slice produces no output",
			conflicts: []ConflictInfo{},
			wantEmpty: true,
		},
		{
			name: "ten conflicts shows overflow count of 7",
			conflicts: func() []ConflictInfo {
				var cs []ConflictInfo
				for i := 0; i < 10; i++ {
					cs = append(cs, ConflictInfo{
						Path: fmt.Sprintf("file%d.go", i), LocalState: "modified",
						ContainerState: "modified", DetectedAt: now,
					})
				}
				return cs
			}(),
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "10 sync conflicts") {
						t.Errorf("expected '10 sync conflicts', got: %s", output)
					}
				},
				func(t *testing.T, output string) {
					if !strings.Contains(output, "...and 7 more") {
						t.Errorf("expected '...and 7 more', got: %s", output)
					}
				},
			},
		},
		{
			name: "paths with special characters rendered correctly",
			conflicts: []ConflictInfo{
				{Path: "path with spaces/file.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "path with spaces/file.go") {
						t.Errorf("expected path with spaces in output: %s", output)
					}
				},
			},
		},
		{
			name: "unicode paths rendered correctly",
			conflicts: []ConflictInfo{
				{Path: "目录/文件.go", LocalState: "created", ContainerState: "deleted", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !strings.Contains(output, "目录/文件.go") {
						t.Errorf("expected unicode path in output: %s", output)
					}
				},
			},
		},
		{
			name: "very long path is not truncated",
			conflicts: []ConflictInfo{
				{Path: strings.Repeat("a", 200) + "/file.go", LocalState: "modified", ContainerState: "modified", DetectedAt: now},
			},
			wantChecks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					longPath := strings.Repeat("a", 200) + "/file.go"
					if !strings.Contains(output, longPath) {
						t.Errorf("expected long path in output")
					}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			RenderBanner(tt.conflicts, &buf)
			output := buf.String()

			if tt.wantEmpty {
				if output != "" {
					t.Errorf("expected empty output, got: %s", output)
				}
				return
			}

			for _, check := range tt.wantChecks {
				check(t, output)
			}
		})
	}
}

func TestConflictDescription(t *testing.T) {
	tests := []struct {
		name           string
		localState     string
		containerState string
		want           string
	}{
		{
			name:           "same states",
			localState:     "modified",
			containerState: "modified",
			want:           "modified on both sides",
		},
		{
			name:           "different states",
			localState:     "created",
			containerState: "deleted",
			want:           "created locally, deleted in container",
		},
		{
			name:           "deleted locally created in container",
			localState:     "deleted",
			containerState: "created",
			want:           "deleted locally, created in container",
		},
		{
			name:           "empty strings",
			localState:     "",
			containerState: "",
			want:           " on both sides",
		},
		{
			name:           "empty local different container",
			localState:     "",
			containerState: "modified",
			want:           " locally, modified in container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conflictDescription(tt.localState, tt.containerState)
			if got != tt.want {
				t.Errorf("conflictDescription(%q, %q) = %q, want %q",
					tt.localState, tt.containerState, got, tt.want)
			}
		})
	}
}
