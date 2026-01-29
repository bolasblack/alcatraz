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
	afero.WriteFile(tfs, "/tmp/test", []byte("content"), 0644)

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

func TestGroupOpsBySudo(t *testing.T) {
	tests := []struct {
		name     string
		ops      []FileOp
		expected []OpGroup
	}{
		{
			name:     "empty ops",
			ops:      []FileOp{},
			expected: nil,
		},
		{
			name: "all same sudo value",
			ops: []FileOp{
				{Path: "/a", NeedSudo: false},
				{Path: "/b", NeedSudo: false},
				{Path: "/c", NeedSudo: false},
			},
			expected: []OpGroup{
				{NeedSudo: false, Ops: []FileOp{{Path: "/a", NeedSudo: false}, {Path: "/b", NeedSudo: false}, {Path: "/c", NeedSudo: false}}},
			},
		},
		{
			name: "alternating sudo values",
			ops: []FileOp{
				{Path: "/a", NeedSudo: false},
				{Path: "/b", NeedSudo: true},
				{Path: "/c", NeedSudo: false},
			},
			expected: []OpGroup{
				{NeedSudo: false, Ops: []FileOp{{Path: "/a", NeedSudo: false}}},
				{NeedSudo: true, Ops: []FileOp{{Path: "/b", NeedSudo: true}}},
				{NeedSudo: false, Ops: []FileOp{{Path: "/c", NeedSudo: false}}},
			},
		},
		{
			name: "consecutive groups",
			ops: []FileOp{
				{Path: "/a", NeedSudo: false},
				{Path: "/b", NeedSudo: false},
				{Path: "/c", NeedSudo: true},
				{Path: "/d", NeedSudo: true},
				{Path: "/e", NeedSudo: false},
			},
			expected: []OpGroup{
				{NeedSudo: false, Ops: []FileOp{{Path: "/a", NeedSudo: false}, {Path: "/b", NeedSudo: false}}},
				{NeedSudo: true, Ops: []FileOp{{Path: "/c", NeedSudo: true}, {Path: "/d", NeedSudo: true}}},
				{NeedSudo: false, Ops: []FileOp{{Path: "/e", NeedSudo: false}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GroupOpsBySudo(tt.ops)
			if len(result) != len(tt.expected) {
				t.Errorf("GroupOpsBySudo() returned %d groups, want %d", len(result), len(tt.expected))
				return
			}
			for i, group := range result {
				if group.NeedSudo != tt.expected[i].NeedSudo {
					t.Errorf("group[%d].NeedSudo = %v, want %v", i, group.NeedSudo, tt.expected[i].NeedSudo)
				}
				if len(group.Ops) != len(tt.expected[i].Ops) {
					t.Errorf("group[%d] has %d ops, want %d", i, len(group.Ops), len(tt.expected[i].Ops))
				}
			}
		})
	}
}

func TestExecuteGroupedOps(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create groups with mixed sudo requirements
	groups := []OpGroup{
		{
			NeedSudo: false,
			Ops: []FileOp{
				{Path: "/tmp/a", Op: OpCreate, Content: []byte("a"), Mode: 0644},
				{Path: "/tmp/b", Op: OpCreate, Content: []byte("b"), Mode: 0644},
			},
		},
	}

	// Execute with mock sudo executor (should not be called for non-sudo ops)
	sudoCalled := false
	err := ExecuteGroupedOps(fs, groups, func(script string) error {
		sudoCalled = true
		return nil
	})

	if err != nil {
		t.Fatalf("ExecuteGroupedOps failed: %v", err)
	}

	if sudoCalled {
		t.Error("sudo executor should not be called for non-sudo ops")
	}

	// Verify files were created
	contentA, _ := afero.ReadFile(fs, "/tmp/a")
	if string(contentA) != "a" {
		t.Errorf("expected 'a', got %q", string(contentA))
	}
	contentB, _ := afero.ReadFile(fs, "/tmp/b")
	if string(contentB) != "b" {
		t.Errorf("expected 'b', got %q", string(contentB))
	}
}

func TestExecuteGroupedOps_WithSudo(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create groups with sudo requirement
	groups := []OpGroup{
		{
			NeedSudo: true,
			Ops: []FileOp{
				{Path: "/etc/test", Op: OpCreate, Content: []byte("sudo"), Mode: 0644},
			},
		},
	}

	// Execute with mock sudo executor
	var receivedScript string
	err := ExecuteGroupedOps(fs, groups, func(script string) error {
		receivedScript = script
		return nil
	})

	if err != nil {
		t.Fatalf("ExecuteGroupedOps failed: %v", err)
	}

	// Verify sudo was called with correct script
	if receivedScript == "" {
		t.Error("sudo executor should be called for sudo ops")
	}
	if !strings.Contains(receivedScript, "/etc/test") {
		t.Error("script should contain the file path")
	}
}
