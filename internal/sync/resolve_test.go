package sync

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

type mockExecutor struct {
	err       error
	execFn    func(containerID string, cmd []string) error // overrides err when set
	gotID     string
	gotCmd    []string
	callCount int
}

var _ ContainerExecutor = (*mockExecutor)(nil)

func (m *mockExecutor) ExecInContainer(containerID string, cmd []string) error {
	m.gotID = containerID
	m.gotCmd = cmd
	m.callCount++
	if m.execFn != nil {
		return m.execFn(containerID, cmd)
	}
	return m.err
}

func TestResolveLocal(t *testing.T) {
	tests := []struct {
		name          string
		containerPath string
		execErr       error
		wantErr       bool
	}{
		{
			name:          "successful deletion",
			containerPath: "/workspace/src/config.yaml",
		},
		{
			name:          "executor failure",
			containerPath: "/workspace/src/config.yaml",
			execErr:       fmt.Errorf("container not found"),
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{err: tt.execErr}

			err := ResolveLocal(executor, "test-container", tt.containerPath)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveContainer(t *testing.T) {
	tests := []struct {
		name      string
		localPath string
		setup     func(fs afero.Fs)
		wantErr   bool
	}{
		{
			name:      "successful deletion",
			localPath: "/project/src/config.yaml",
			setup: func(fs afero.Fs) {
				_ = afero.WriteFile(fs, "/project/src/config.yaml", []byte("content"), 0o644)
			},
		},
		{
			name:      "file does not exist",
			localPath: "/project/nonexistent.txt",
			setup:     func(fs afero.Fs) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tt.setup(fs)

			err := ResolveContainer(fs, tt.localPath)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify file was actually deleted
			exists, _ := afero.Exists(fs, tt.localPath)
			if exists {
				t.Error("file still exists after resolve")
			}
		})
	}
}

func TestResolveLocal_PassesCorrectCommandArgs(t *testing.T) {
	executor := &mockExecutor{}
	containerID := "my-container-123"
	containerPath := "/workspace/src/deep/nested/file.yaml"

	err := ResolveLocal(executor, containerID, containerPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executor.gotID != containerID {
		t.Errorf("got container ID %q, want %q", executor.gotID, containerID)
	}
	if len(executor.gotCmd) != 2 || executor.gotCmd[0] != "rm" || executor.gotCmd[1] != containerPath {
		t.Errorf("got cmd %v, want [rm %s]", executor.gotCmd, containerPath)
	}
	if executor.callCount != 1 {
		t.Errorf("executor called %d times, want 1", executor.callCount)
	}
}

func TestResolveLocal_ErrorWrapping(t *testing.T) {
	underlying := fmt.Errorf("permission denied")
	executor := &mockExecutor{err: underlying}

	err := ResolveLocal(executor, "ctr", "/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to delete container file") {
		t.Errorf("error should contain wrapper message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should contain underlying cause, got: %v", err)
	}
}

func TestResolveContainer_ErrorWrapping(t *testing.T) {
	fs := afero.NewMemMapFs()
	// Don't create the file — Remove will fail

	err := ResolveContainer(fs, "/nonexistent/path.txt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to delete local file") {
		t.Errorf("error should contain wrapper message, got: %v", err)
	}
}
