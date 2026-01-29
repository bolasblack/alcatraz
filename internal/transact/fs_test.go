package transact

import (
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestNeedsCommit(t *testing.T) {
	// Test with changes
	tfs := New(WithActualFs(afero.NewMemMapFs()))
	if err := afero.WriteFile(tfs, "/etc/new", []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if !tfs.NeedsCommit() {
		t.Error("expected NeedsCommit=true for new file")
	}

	// Test without changes
	actualFs := afero.NewMemMapFs()
	if err := actualFs.MkdirAll("/etc", 0755); err != nil {
		t.Fatalf("failed to setup: %v", err)
	}
	if err := afero.WriteFile(actualFs, "/etc/existing", []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to setup: %v", err)
	}
	tfs2 := New(WithActualFs(actualFs))
	// Don't make any changes
	if tfs2.NeedsCommit() {
		t.Error("expected NeedsCommit=false when no staged changes")
	}
}

func TestTransactFs_WriteFile(t *testing.T) {
	tfs := New(WithActualFs(afero.NewMemMapFs()))

	err := afero.WriteFile(tfs, "/etc/test/file.txt", []byte("content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify tracked
	paths := tfs.TrackedPaths()
	if len(paths) != 1 || paths[0] != "/etc/test/file.txt" {
		t.Errorf("expected [/etc/test/file.txt], got %v", paths)
	}
}

func TestTransactFs_Chmod(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	if err := actualFs.MkdirAll("/etc", 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := afero.WriteFile(actualFs, "/etc/test", []byte("content"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	tfs := New(WithActualFs(actualFs))
	err := tfs.Chmod("/etc/test", 0755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	// Verify tracked
	paths := tfs.TrackedPaths()
	if len(paths) != 1 || paths[0] != "/etc/test" {
		t.Errorf("expected [/etc/test], got %v", paths)
	}

	// Verify diff shows chmod
	ops, err := tfs.Diff()
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if len(ops) != 1 || ops[0].Op != OpChmod {
		t.Errorf("expected OpChmod, got %v", ops)
	}
}

func TestTransactFs_Remove(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	if err := actualFs.MkdirAll("/etc", 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := afero.WriteFile(actualFs, "/etc/test", []byte("content"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	tfs := New(WithActualFs(actualFs))
	err := tfs.Remove("/etc/test")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify tracked
	deleted := tfs.DeletedPaths()
	if len(deleted) != 1 || deleted[0] != "/etc/test" {
		t.Errorf("expected [/etc/test], got %v", deleted)
	}

	// Verify diff shows delete
	ops, err := tfs.Diff()
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if len(ops) != 1 || ops[0].Op != OpDelete {
		t.Errorf("expected OpDelete, got %v", ops)
	}
}

// Test that ReadFile reads from staged first (CopyOnWrite semantics)
func TestReadFile_CopyOnWrite(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	// Create file in actual with original content
	if err := actualFs.MkdirAll("/etc", 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := afero.WriteFile(actualFs, "/etc/test", []byte("original"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	tfs := New(WithActualFs(actualFs))

	// ReadFile should return actual content before any writes
	content, err := afero.ReadFile(tfs, "/etc/test")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "original" {
		t.Errorf("ReadFile: expected 'original', got %q", string(content))
	}

	// Write new content to staged
	if err := afero.WriteFile(tfs, "/etc/test", []byte("updated"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// ReadFile should return staged content (CopyOnWrite semantics)
	content, err = afero.ReadFile(tfs, "/etc/test")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "updated" {
		t.Errorf("ReadFile: expected 'updated' from staged, got %q", string(content))
	}
}

func TestTransactFs_Rename(t *testing.T) {
	t.Run("rename staged file", func(t *testing.T) {
		tfs := New(WithActualFs(afero.NewMemMapFs()))

		// Create file in staged
		if err := afero.WriteFile(tfs, "/src/file.txt", []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Rename
		if err := tfs.Rename("/src/file.txt", "/dst/file.txt"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// New path should be readable
		content, err := afero.ReadFile(tfs, "/dst/file.txt")
		if err != nil {
			t.Fatalf("ReadFile new path failed: %v", err)
		}
		if string(content) != "content" {
			t.Errorf("got %q, want %q", string(content), "content")
		}

		// Old path should be marked for deletion
		deleted := tfs.DeletedPaths()
		found := false
		for _, p := range deleted {
			if p == "/src/file.txt" {
				found = true
				break
			}
		}
		if !found {
			t.Error("old path not marked for deletion")
		}
	})

	t.Run("rename actual file (copy-on-write)", func(t *testing.T) {
		actualFs := afero.NewMemMapFs()
		if err := actualFs.MkdirAll("/src", 0755); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if err := afero.WriteFile(actualFs, "/src/file.txt", []byte("actual content"), 0755); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		tfs := New(WithActualFs(actualFs))

		// Rename from actual
		if err := tfs.Rename("/src/file.txt", "/dst/file.txt"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// New path should have content
		content, err := afero.ReadFile(tfs, "/dst/file.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "actual content" {
			t.Errorf("got %q, want %q", string(content), "actual content")
		}

		// Old path should be deleted
		_, err = tfs.Stat("/src/file.txt")
		if err == nil {
			t.Error("old path should not exist after rename")
		}
	})

	t.Run("rename non-existent file returns error", func(t *testing.T) {
		tfs := New(WithActualFs(afero.NewMemMapFs()))

		err := tfs.Rename("/nonexistent", "/dst")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("cross-directory rename", func(t *testing.T) {
		actualFs := afero.NewMemMapFs()
		if err := actualFs.MkdirAll("/a/b/c", 0755); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if err := afero.WriteFile(actualFs, "/a/b/c/file.txt", []byte("deep"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		tfs := New(WithActualFs(actualFs))

		// Rename to completely different path
		if err := tfs.Rename("/a/b/c/file.txt", "/x/y/z/file.txt"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		content, err := afero.ReadFile(tfs, "/x/y/z/file.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "deep" {
			t.Errorf("got %q, want %q", string(content), "deep")
		}
	})
}

func TestTransactFs_Chown(t *testing.T) {
	t.Run("chown staged file", func(t *testing.T) {
		tfs := New(WithActualFs(afero.NewMemMapFs()))

		// Create file in staged
		if err := afero.WriteFile(tfs, "/test", []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Chown
		if err := tfs.Chown("/test", 1000, 1000); err != nil {
			t.Fatalf("Chown failed: %v", err)
		}

		// File should be tracked
		paths := tfs.TrackedPaths()
		found := false
		for _, p := range paths {
			if p == "/test" {
				found = true
				break
			}
		}
		if !found {
			t.Error("path not tracked after Chown")
		}
	})

	t.Run("chown actual file (copy-on-write)", func(t *testing.T) {
		actualFs := afero.NewMemMapFs()
		if err := actualFs.MkdirAll("/etc", 0755); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if err := afero.WriteFile(actualFs, "/etc/test", []byte("content"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		tfs := New(WithActualFs(actualFs))

		// Chown triggers copy from actual to staged
		if err := tfs.Chown("/etc/test", 1000, 1000); err != nil {
			t.Fatalf("Chown failed: %v", err)
		}

		// File should be tracked
		paths := tfs.TrackedPaths()
		if len(paths) != 1 || paths[0] != "/etc/test" {
			t.Errorf("expected [/etc/test], got %v", paths)
		}
	})

	t.Run("chown non-existent file returns error", func(t *testing.T) {
		tfs := New(WithActualFs(afero.NewMemMapFs()))

		err := tfs.Chown("/nonexistent", 1000, 1000)
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})
}

func TestTransactFs_Chtimes(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	t.Run("chtimes staged file", func(t *testing.T) {
		tfs := New(WithActualFs(afero.NewMemMapFs()))

		// Create file in staged
		if err := afero.WriteFile(tfs, "/test", []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Chtimes
		if err := tfs.Chtimes("/test", testTime, testTime); err != nil {
			t.Fatalf("Chtimes failed: %v", err)
		}

		// File should be tracked
		paths := tfs.TrackedPaths()
		found := false
		for _, p := range paths {
			if p == "/test" {
				found = true
				break
			}
		}
		if !found {
			t.Error("path not tracked after Chtimes")
		}

		// Verify time was set
		info, err := tfs.Stat("/test")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if !info.ModTime().Equal(testTime) {
			t.Errorf("ModTime = %v, want %v", info.ModTime(), testTime)
		}
	})

	t.Run("chtimes actual file (copy-on-write)", func(t *testing.T) {
		actualFs := afero.NewMemMapFs()
		if err := actualFs.MkdirAll("/etc", 0755); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if err := afero.WriteFile(actualFs, "/etc/test", []byte("content"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		tfs := New(WithActualFs(actualFs))

		// Chtimes triggers copy from actual to staged
		if err := tfs.Chtimes("/etc/test", testTime, testTime); err != nil {
			t.Fatalf("Chtimes failed: %v", err)
		}

		// File should be tracked
		paths := tfs.TrackedPaths()
		if len(paths) != 1 || paths[0] != "/etc/test" {
			t.Errorf("expected [/etc/test], got %v", paths)
		}

		// Verify time was set
		info, err := tfs.Stat("/etc/test")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if !info.ModTime().Equal(testTime) {
			t.Errorf("ModTime = %v, want %v", info.ModTime(), testTime)
		}
	})

	t.Run("chtimes non-existent file returns error", func(t *testing.T) {
		tfs := New(WithActualFs(afero.NewMemMapFs()))

		err := tfs.Chtimes("/nonexistent", testTime, testTime)
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})
}
