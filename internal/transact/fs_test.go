package transact

import (
	"testing"

	"github.com/spf13/afero"
)

func TestNeedsCommit(t *testing.T) {
	// Test with changes
	tfs := New(WithActualFs(afero.NewMemMapFs()))
	if err := afero.WriteFile(tfs,"/etc/new", []byte("content"), 0644); err != nil {
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

	err := afero.WriteFile(tfs,"/etc/test/file.txt", []byte("content"), 0644)
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
	content, err := afero.ReadFile(tfs,"/etc/test")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "original" {
		t.Errorf("ReadFile: expected 'original', got %q", string(content))
	}

	// Write new content to staged
	if err := afero.WriteFile(tfs,"/etc/test", []byte("updated"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// ReadFile should return staged content (CopyOnWrite semantics)
	content, err = afero.ReadFile(tfs,"/etc/test")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "updated" {
		t.Errorf("ReadFile: expected 'updated' from staged, got %q", string(content))
	}
}
