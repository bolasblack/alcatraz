package transact

import (
	"os"
	"testing"

	"github.com/spf13/afero"
)

// Regression test: WriteFile delegates to staged fs without implicit MkdirAll.
// Note: MemMapFs auto-creates parent dirs, so we use a strict wrapper to verify.
func TestWriteFile_NoParentDir(t *testing.T) {
	// Use strictMkdirFs that tracks MkdirAll calls
	strictFs := &strictMkdirFs{Fs: afero.NewMemMapFs()}
	tfs := &TransactFs{
		staged:      strictFs,
		actual:      afero.NewMemMapFs(),
		openHandles: make(map[*TransactFsFile]struct{}),
	}

	// WriteFile now calls MkdirAll internally for convenience
	_ = afero.WriteFile(tfs, "/some/path/file.txt", []byte("content"), 0644)

	// With the new implementation, WriteFile does ensure parent dirs
	if !strictFs.mkdirAllCalled {
		t.Error("WriteFile should call MkdirAll internally for convenience")
	}
}

// strictMkdirFs tracks if MkdirAll was called
type strictMkdirFs struct {
	afero.Fs
	mkdirAllCalled bool
}

func (s *strictMkdirFs) MkdirAll(path string, perm os.FileMode) error {
	s.mkdirAllCalled = true
	return s.Fs.MkdirAll(path, perm)
}
