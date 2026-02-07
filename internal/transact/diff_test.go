package transact

import (
	"os"
	"testing"

	"github.com/spf13/afero"
)

func TestComputeDiff_NewFile(t *testing.T) {
	staged := afero.NewMemMapFs()
	actual := afero.NewMemMapFs()

	// Setup: file exists only in staged
	if err := afero.WriteFile(staged, "/etc/test", []byte("new content"), 0644); err != nil {
		t.Fatalf("failed to write staged file: %v", err)
	}

	ops, err := ComputeDiff(staged, actual, []string{"/etc/test"}, nil)
	if err != nil {
		t.Fatalf("ComputeDiff failed: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Op != OpCreate {
		t.Errorf("expected OpCreate, got %v", ops[0].Op)
	}
	if ops[0].Path != "/etc/test" {
		t.Errorf("expected /etc/test, got %s", ops[0].Path)
	}
	if string(ops[0].Content) != "new content" {
		t.Errorf("expected 'new content', got %q", string(ops[0].Content))
	}
	if os.Getuid() != 0 && !ops[0].NeedSudo {
		t.Error("expected NeedSudo=true for /etc/ path")
	}
}

func TestComputeDiff_UpdateFile(t *testing.T) {
	staged := afero.NewMemMapFs()
	actual := afero.NewMemMapFs()

	// Setup: file exists in both with different content
	if err := afero.WriteFile(staged, "/etc/pf.anchors/test", []byte("updated"), 0644); err != nil {
		t.Fatalf("failed to write staged file: %v", err)
	}
	if err := afero.WriteFile(actual, "/etc/pf.anchors/test", []byte("original"), 0644); err != nil {
		t.Fatalf("failed to write actual file: %v", err)
	}

	ops, err := ComputeDiff(staged, actual, []string{"/etc/pf.anchors/test"}, nil)
	if err != nil {
		t.Fatalf("ComputeDiff failed: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Op != OpUpdate {
		t.Errorf("expected OpUpdate, got %v", ops[0].Op)
	}
	if string(ops[0].Content) != "updated" {
		t.Errorf("expected 'updated', got %q", string(ops[0].Content))
	}
}

func TestComputeDiff_ChmodOnly(t *testing.T) {
	staged := afero.NewMemMapFs()
	actual := afero.NewMemMapFs()

	// Setup: same content, different permissions
	if err := afero.WriteFile(staged, "/etc/test", []byte("same"), 0755); err != nil {
		t.Fatalf("failed to write staged file: %v", err)
	}
	if err := afero.WriteFile(actual, "/etc/test", []byte("same"), 0644); err != nil {
		t.Fatalf("failed to write actual file: %v", err)
	}

	ops, err := ComputeDiff(staged, actual, []string{"/etc/test"}, nil)
	if err != nil {
		t.Fatalf("ComputeDiff failed: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Op != OpChmod {
		t.Errorf("expected OpChmod, got %v", ops[0].Op)
	}
	if ops[0].Mode != os.FileMode(0755) {
		t.Errorf("expected 0755, got %o", ops[0].Mode)
	}
}

func TestComputeDiff_Delete(t *testing.T) {
	staged := afero.NewMemMapFs()
	actual := afero.NewMemMapFs()

	// Setup: file exists only in actual (marked for deletion)
	if err := afero.WriteFile(actual, "/etc/old", []byte("delete me"), 0644); err != nil {
		t.Fatalf("failed to write actual file: %v", err)
	}

	ops, err := ComputeDiff(staged, actual, nil, []string{"/etc/old"})
	if err != nil {
		t.Fatalf("ComputeDiff failed: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Op != OpDelete {
		t.Errorf("expected OpDelete, got %v", ops[0].Op)
	}
	if ops[0].Path != "/etc/old" {
		t.Errorf("expected /etc/old, got %s", ops[0].Path)
	}
}

func TestComputeDiff_NoChanges(t *testing.T) {
	staged := afero.NewMemMapFs()
	actual := afero.NewMemMapFs()

	// Setup: same content and permissions in both
	if err := afero.WriteFile(staged, "/etc/test", []byte("same"), 0644); err != nil {
		t.Fatalf("failed to write staged file: %v", err)
	}
	if err := afero.WriteFile(actual, "/etc/test", []byte("same"), 0644); err != nil {
		t.Fatalf("failed to write actual file: %v", err)
	}

	ops, err := ComputeDiff(staged, actual, []string{"/etc/test"}, nil)
	if err != nil {
		t.Fatalf("ComputeDiff failed: %v", err)
	}

	if len(ops) != 0 {
		t.Errorf("expected 0 ops for no changes, got %d", len(ops))
	}
}

func TestNeedsSudo(t *testing.T) {
	// Test paths that require sudo (non-writable system paths)
	// Skip when running as root — root has write access everywhere,
	// so needsSudo correctly returns false.
	if os.Getuid() == 0 {
		t.Log("skipping system path tests: running as root")
	} else {
		systemPaths := []string{
			"/etc/pf.conf",
			"/etc/pf.anchors/alcatraz",
			"/Library/LaunchDaemons/com.test.plist",
		}
		for _, path := range systemPaths {
			t.Run(path, func(t *testing.T) {
				got := needsSudo(path)
				// These paths should require sudo since they're not writable
				if !got {
					t.Errorf("needsSudo(%q) = false, expected true for system path", path)
				}
			})
		}
	}

	// Test writable paths (like /tmp)
	t.Run("/tmp/test", func(t *testing.T) {
		got := needsSudo("/tmp/test")
		// /tmp should be writable without sudo
		if got {
			t.Errorf("needsSudo(\"/tmp/test\") = true, expected false for writable path")
		}
	})

	// Test non-existent intermediate directory under a writable parent.
	// This is the state.json bug: .alca/ doesn't exist yet, but the project
	// directory (parent) is writable — should NOT require sudo.
	t.Run("non-existent subdir under /tmp", func(t *testing.T) {
		got := needsSudo("/tmp/nonexistent-dir-abc123/subdir/file.json")
		if got {
			t.Errorf("needsSudo with non-existent subdir under /tmp = true, expected false")
		}
	})
}

func TestOpTypeString(t *testing.T) {
	tests := []struct {
		op       OpType
		expected string
	}{
		{OpCreate, "create"},
		{OpUpdate, "update"},
		{OpChmod, "chmod"},
		{OpDelete, "delete"},
		{OpType(99), "unknown"},
	}

	for _, tc := range tests {
		got := tc.op.String()
		if got != tc.expected {
			t.Errorf("OpType(%d).String() = %q, want %q", tc.op, got, tc.expected)
		}
	}
}
