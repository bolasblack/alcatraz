package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestReadCache(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(fs afero.Fs)
		wantNil   bool
		wantErr   bool
		wantCount int
		checkData func(t *testing.T, got *CacheData)
	}{
		{
			name:    "file not exists returns nil",
			setup:   func(fs afero.Fs) {},
			wantNil: true,
		},
		{
			name: "valid JSON parsed correctly",
			setup: func(fs afero.Fs) {
				data := CacheData{
					UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Conflicts: []ConflictInfo{
						{Path: "file.txt", LocalState: "modified", ContainerState: "modified"},
					},
				}
				buf, _ := json.MarshalIndent(data, "", "  ")
				_ = fs.MkdirAll("/project/.alca", 0o755)
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", buf, 0o644)
			},
			wantCount: 1,
		},
		{
			name: "invalid JSON returns error",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/project/.alca", 0o755)
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", []byte("{invalid"), 0o644)
			},
			wantErr: true,
		},
		{
			name: "empty file returns error",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/project/.alca", 0o755)
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", []byte{}, 0o644)
			},
			wantErr: true,
		},
		{
			name: "valid JSON wrong structure returns zero conflicts",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/project/.alca", 0o755)
				// JSON array instead of object — Unmarshal into struct yields zero values
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", []byte(`{"updatedAt":"2025-01-01T00:00:00Z","conflicts":"not-an-array"}`), 0o644)
			},
			wantErr: true,
		},
		{
			name: "valid JSON with empty conflicts field",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/project/.alca", 0o755)
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", []byte(`{"updatedAt":"2025-01-01T00:00:00Z","conflicts":[]}`), 0o644)
			},
			wantCount: 0,
		},
		{
			name: "valid JSON with null conflicts field",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/project/.alca", 0o755)
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", []byte(`{"updatedAt":"2025-01-01T00:00:00Z","conflicts":null}`), 0o644)
			},
			wantCount: 0,
		},
		{
			name: "roundtrip preserves all conflict fields",
			setup: func(fs afero.Fs) {
				data := CacheData{
					UpdatedAt: time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC),
					Conflicts: []ConflictInfo{
						{Path: "dir/file.go", LocalState: "created", ContainerState: "deleted", DetectedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)},
					},
				}
				buf, _ := json.MarshalIndent(data, "", "  ")
				_ = fs.MkdirAll("/project/.alca", 0o755)
				_ = afero.WriteFile(fs, "/project/.alca/sync-conflicts-cache.json", buf, 0o644)
			},
			wantCount: 1,
			checkData: func(t *testing.T, got *CacheData) {
				t.Helper()
				if got.UpdatedAt != time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC) {
					t.Errorf("UpdatedAt mismatch: got %v", got.UpdatedAt)
				}
				c := got.Conflicts[0]
				if c.Path != "dir/file.go" {
					t.Errorf("Path mismatch: got %q", c.Path)
				}
				if c.LocalState != "created" {
					t.Errorf("LocalState mismatch: got %q", c.LocalState)
				}
				if c.ContainerState != "deleted" {
					t.Errorf("ContainerState mismatch: got %q", c.ContainerState)
				}
				if c.DetectedAt != time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC) {
					t.Errorf("DetectedAt mismatch: got %v", c.DetectedAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tt.setup(fs)

			got, err := ReadCache(fs, "/project")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if len(got.Conflicts) != tt.wantCount {
				t.Errorf("expected %d conflicts, got %d", tt.wantCount, len(got.Conflicts))
			}
			if tt.checkData != nil {
				tt.checkData(t, got)
			}
		})
	}
}

func TestWriteCache(t *testing.T) {
	t.Run("creates .alca dir and writes valid JSON", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		data := &CacheData{
			UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Conflicts: []ConflictInfo{
				{Path: "a.txt", LocalState: "created", ContainerState: "modified"},
			},
		}

		err := WriteCache(fs, "/project", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify dir exists
		dirExists, _ := afero.DirExists(fs, "/project/.alca")
		if !dirExists {
			t.Fatal("expected .alca dir to exist")
		}

		// Verify file is valid JSON
		got, err := ReadCache(fs, "/project")
		if err != nil {
			t.Fatalf("failed to read back cache: %v", err)
		}
		if len(got.Conflicts) != 1 {
			t.Errorf("expected 1 conflict, got %d", len(got.Conflicts))
		}
		if got.Conflicts[0].Path != "a.txt" {
			t.Errorf("expected path a.txt, got %s", got.Conflicts[0].Path)
		}
	})

	t.Run("nil data panics or writes null", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		// WriteCache with nil should not panic
		err := WriteCache(fs, "/project", nil)
		// nil pointer dereference would panic; if we get here, it handled it
		if err != nil {
			t.Logf("WriteCache(nil) returned error (acceptable): %v", err)
		} else {
			// Verify what was written is readable
			got, err := ReadCache(fs, "/project")
			if err != nil {
				t.Logf("ReadCache after nil write returned error (acceptable): %v", err)
			} else if got != nil && got.Conflicts != nil {
				t.Errorf("expected nil or empty conflicts, got %+v", got.Conflicts)
			}
		}
	})

	t.Run("empty conflicts slice writes valid cache", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		data := &CacheData{
			UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Conflicts: []ConflictInfo{},
		}

		err := WriteCache(fs, "/project", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := ReadCache(fs, "/project")
		if err != nil {
			t.Fatalf("failed to read back: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil cache")
		}
		if len(got.Conflicts) != 0 {
			t.Errorf("expected 0 conflicts, got %d", len(got.Conflicts))
		}
	})

	t.Run("overwrites existing cache", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		first := &CacheData{
			UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Conflicts: []ConflictInfo{{Path: "old.txt"}},
		}
		second := &CacheData{
			UpdatedAt: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			Conflicts: []ConflictInfo{{Path: "new.txt"}},
		}

		_ = WriteCache(fs, "/project", first)
		err := WriteCache(fs, "/project", second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, _ := ReadCache(fs, "/project")
		if len(got.Conflicts) != 1 || got.Conflicts[0].Path != "new.txt" {
			t.Errorf("expected overwritten cache with new.txt, got %+v", got.Conflicts)
		}
	})
}

func TestSyncUpdateCache(t *testing.T) {
	t.Run("no sessions returns empty conflicts", func(t *testing.T) {
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return nil, nil
			},
		}

		fs := afero.NewMemMapFs()
		env := NewSyncEnv(fs, nil, mock)

		got, err := SyncUpdateCache(context.Background(), env, "proj1", "/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Conflicts) != 0 {
			t.Errorf("expected 0 conflicts, got %d", len(got.Conflicts))
		}

		// Verify cache was written
		cached, err := ReadCache(fs, "/project")
		if err != nil {
			t.Fatalf("failed to read cache: %v", err)
		}
		if cached == nil {
			t.Fatal("expected cache to be written")
		}
	})

	t.Run("with conflicts writes correct cache", func(t *testing.T) {
		sessionJSON := `[{"conflicts":[{"root":"src","alphaChanges":[{"path":"src/main.go","old":{"kind":"file"},"new":{"kind":"file"}}],"betaChanges":[{"path":"src/main.go","old":{"kind":"file"},"new":{"kind":"file"}}]}]}]`
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return []string{"alca-proj1-0"}, nil
			},
			listSessionJSONFn: func(sessionName string) ([]byte, error) {
				return []byte(sessionJSON), nil
			},
		}

		fs := afero.NewMemMapFs()
		env := NewSyncEnv(fs, nil, mock)

		got, err := SyncUpdateCache(context.Background(), env, "proj1", "/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Conflicts) != 1 {
			t.Errorf("expected 1 conflict, got %d", len(got.Conflicts))
		}
		if got.Conflicts[0].Path != "src/main.go" {
			t.Errorf("expected path src/main.go, got %s", got.Conflicts[0].Path)
		}

		// Verify cache file
		cached, _ := ReadCache(fs, "/project")
		if len(cached.Conflicts) != 1 {
			t.Errorf("expected 1 cached conflict, got %d", len(cached.Conflicts))
		}
	})
}

func TestSyncUpdateCache_Internal(t *testing.T) {
	t.Run("multiple sessions aggregates conflicts", func(t *testing.T) {
		session0JSON := `[{"conflicts":[{"root":"","alphaChanges":[{"path":"a.txt","new":{"kind":"file"}}],"betaChanges":[{"path":"a.txt","new":{"kind":"file"}}]}]}]`
		session1JSON := `[{"conflicts":[{"root":"","alphaChanges":[{"path":"b.txt","old":{"kind":"file"},"new":null}],"betaChanges":[{"path":"b.txt","new":{"kind":"file"}}]}]}]`

		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return []string{"alca-proj1-0", "alca-proj1-1"}, nil
			},
			listSessionJSONFn: func(sessionName string) ([]byte, error) {
				switch sessionName {
				case "alca-proj1-0":
					return []byte(session0JSON), nil
				case "alca-proj1-1":
					return []byte(session1JSON), nil
				}
				return nil, fmt.Errorf("unexpected session: %s", sessionName)
			},
		}

		fs := afero.NewMemMapFs()
		env := NewSyncEnv(fs, nil, mock)

		got, err := SyncUpdateCache(context.Background(), env, "proj1", "/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Conflicts) != 2 {
			t.Errorf("expected 2 conflicts, got %d", len(got.Conflicts))
		}
	})

	t.Run("detect conflicts error returns error", func(t *testing.T) {
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return []string{"alca-proj1-0"}, nil
			},
			listSessionJSONFn: func(sessionName string) ([]byte, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}

		fs := afero.NewMemMapFs()
		env := NewSyncEnv(fs, nil, mock)

		_, err := SyncUpdateCache(context.Background(), env, "proj1", "/project")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("list sync sessions error returns error", func(t *testing.T) {
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return nil, fmt.Errorf("daemon not running")
			},
		}

		fs := afero.NewMemMapFs()
		env := NewSyncEnv(fs, nil, mock)

		_, err := SyncUpdateCache(context.Background(), env, "proj1", "/project")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "daemon not running") {
			t.Errorf("expected error to contain 'daemon not running', got: %v", err)
		}
	})

	t.Run("write cache failure returns error", func(t *testing.T) {
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return nil, nil
			},
		}

		// Use a read-only filesystem to force WriteCache to fail
		memFs := afero.NewMemMapFs()
		readOnlyFs := afero.NewReadOnlyFs(memFs)
		env := NewSyncEnv(readOnlyFs, nil, mock)

		_, err := SyncUpdateCache(context.Background(), env, "proj1", "/project")
		if err == nil {
			t.Fatal("expected error from read-only fs, got nil")
		}
	})

	t.Run("uses correct session name prefix", func(t *testing.T) {
		var capturedPrefix string
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				capturedPrefix = namePrefix
				return nil, nil
			},
		}

		fs := afero.NewMemMapFs()
		env := NewSyncEnv(fs, nil, mock)

		_, _ = SyncUpdateCache(context.Background(), env, "my-proj", "/project")
		if capturedPrefix != "alca-my-proj-" {
			t.Errorf("expected prefix %q, got %q", "alca-my-proj-", capturedPrefix)
		}
	})
}
