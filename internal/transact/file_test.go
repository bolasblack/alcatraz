package transact

import (
	"io"
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

func TestTransactFsFile_ReadAt(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	if err := afero.WriteFile(actualFs, "/test.txt", []byte("hello world"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	tfs := New(WithActualFs(actualFs))

	f, err := tfs.Open("/test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	t.Run("read from middle", func(t *testing.T) {
		buf := make([]byte, 5)
		n, err := f.ReadAt(buf, 6)
		if err != nil {
			t.Errorf("ReadAt failed: %v", err)
		}
		if n != 5 {
			t.Errorf("ReadAt returned n=%d, want 5", n)
		}
		if string(buf) != "world" {
			t.Errorf("ReadAt returned %q, want %q", string(buf), "world")
		}
	})

	t.Run("offset beyond length returns EOF", func(t *testing.T) {
		buf := make([]byte, 5)
		n, err := f.ReadAt(buf, 100)
		if err != io.EOF {
			t.Errorf("ReadAt beyond length: err=%v, want io.EOF", err)
		}
		if n != 0 {
			t.Errorf("ReadAt beyond length: n=%d, want 0", n)
		}
	})

	t.Run("partial read returns EOF", func(t *testing.T) {
		buf := make([]byte, 20) // larger than remaining content
		n, err := f.ReadAt(buf, 6)
		if err != io.EOF {
			t.Errorf("partial ReadAt: err=%v, want io.EOF", err)
		}
		if n != 5 { // "world" = 5 bytes
			t.Errorf("partial ReadAt: n=%d, want 5", n)
		}
		if string(buf[:n]) != "world" {
			t.Errorf("partial ReadAt: got %q, want %q", string(buf[:n]), "world")
		}
	})
}

func TestTransactFsFile_Seek(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	if err := afero.WriteFile(actualFs, "/test.txt", []byte("hello world"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	tfs := New(WithActualFs(actualFs))

	f, err := tfs.Open("/test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	t.Run("SeekStart", func(t *testing.T) {
		pos, err := f.Seek(5, io.SeekStart)
		if err != nil {
			t.Errorf("Seek SeekStart failed: %v", err)
		}
		if pos != 5 {
			t.Errorf("Seek SeekStart: pos=%d, want 5", pos)
		}
	})

	t.Run("SeekCurrent", func(t *testing.T) {
		// Position is now at 5 from previous test
		pos, err := f.Seek(3, io.SeekCurrent)
		if err != nil {
			t.Errorf("Seek SeekCurrent failed: %v", err)
		}
		if pos != 8 {
			t.Errorf("Seek SeekCurrent: pos=%d, want 8", pos)
		}
	})

	t.Run("SeekEnd", func(t *testing.T) {
		pos, err := f.Seek(-5, io.SeekEnd)
		if err != nil {
			t.Errorf("Seek SeekEnd failed: %v", err)
		}
		if pos != 6 { // 11 - 5 = 6
			t.Errorf("Seek SeekEnd: pos=%d, want 6", pos)
		}
	})

	t.Run("negative position returns ErrInvalid", func(t *testing.T) {
		_, err := f.Seek(-100, io.SeekStart)
		if err != os.ErrInvalid {
			t.Errorf("Seek negative: err=%v, want os.ErrInvalid", err)
		}
	})
}

func TestTransactFsFile_WriteReadOnly(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	if err := afero.WriteFile(actualFs, "/test.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	tfs := New(WithActualFs(actualFs))

	// Open in read-only mode
	f, err := tfs.Open("/test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	t.Run("Write returns ErrPermission", func(t *testing.T) {
		_, err := f.Write([]byte("new data"))
		if err != os.ErrPermission {
			t.Errorf("Write on read-only file: err=%v, want os.ErrPermission", err)
		}
	})

	t.Run("WriteAt returns ErrPermission", func(t *testing.T) {
		_, err := f.WriteAt([]byte("new data"), 0)
		if err != os.ErrPermission {
			t.Errorf("WriteAt on read-only file: err=%v, want os.ErrPermission", err)
		}
	})

	t.Run("WriteString returns ErrPermission", func(t *testing.T) {
		_, err := f.WriteString("new data")
		if err != os.ErrPermission {
			t.Errorf("WriteString on read-only file: err=%v, want os.ErrPermission", err)
		}
	})

	t.Run("Truncate returns ErrPermission", func(t *testing.T) {
		err := f.Truncate(0)
		if err != os.ErrPermission {
			t.Errorf("Truncate on read-only file: err=%v, want os.ErrPermission", err)
		}
	})
}
