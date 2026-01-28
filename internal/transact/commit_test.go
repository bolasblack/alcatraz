package transact

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestGenerateBatchScript(t *testing.T) {
	ops := []FileOp{
		{Path: "/etc/test", Op: OpCreate, Content: []byte("hello"), Mode: 0644},
		{Path: "/etc/old", Op: OpDelete},
		{Path: "/etc/perm", Op: OpChmod, Mode: 0755},
	}

	script := GenerateBatchScript(ops)

	// Check for set -e
	if !strings.Contains(script, "set -e") {
		t.Error("script should start with 'set -e'")
	}

	// Check for base64 encoding (content)
	if !strings.Contains(script, "base64 -d") {
		t.Error("script should use base64 for content")
	}

	// Check for chmod
	if !strings.Contains(script, "chmod 644") {
		t.Error("script should have chmod 644 for create")
	}
	if !strings.Contains(script, "chmod 755") {
		t.Error("script should have chmod 755")
	}

	// Check for rm
	if !strings.Contains(script, "rm -f") {
		t.Error("script should have rm -f for delete")
	}
}

func TestTransactFs_Commit_Success(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	tfs := New(WithActualFs(actualFs))

	// Stage a file
	afero.WriteFile(tfs,"/tmp/test", []byte("content"), 0644)

	// Commit with success callback
	_, err := tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		for _, op := range ctx.Ops {
			if err := ExecuteOp(ctx.BaseFs, op); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify file was written to actual fs
	content, err := afero.ReadFile(actualFs, "/tmp/test")
	if err != nil {
		t.Fatalf("failed to read committed file: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("expected 'content', got %q", string(content))
	}

	// Verify staged is reset
	if tfs.NeedsCommit() {
		t.Error("expected NeedsCommit=false after successful commit")
	}
}

func TestExecuteOp(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Test create
	op := FileOp{Path: "/test/file", Op: OpCreate, Content: []byte("test"), Mode: 0644}
	if err := ExecuteOp(fs, op); err != nil {
		t.Fatalf("ExecuteOp create failed: %v", err)
	}

	content, _ := afero.ReadFile(fs, "/test/file")
	if string(content) != "test" {
		t.Errorf("expected 'test', got %q", string(content))
	}

	// Test chmod
	op = FileOp{Path: "/test/file", Op: OpChmod, Mode: 0755}
	if err := ExecuteOp(fs, op); err != nil {
		t.Fatalf("ExecuteOp chmod failed: %v", err)
	}

	info, _ := fs.Stat("/test/file")
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %o", info.Mode().Perm())
	}

	// Test delete
	op = FileOp{Path: "/test/file", Op: OpDelete}
	if err := ExecuteOp(fs, op); err != nil {
		t.Fatalf("ExecuteOp delete failed: %v", err)
	}

	if exists, _ := afero.Exists(fs, "/test/file"); exists {
		t.Error("file should not exist after delete")
	}
}
