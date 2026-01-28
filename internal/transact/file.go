package transact

import (
	"io"
	"os"

	"github.com/spf13/afero"
)

// Compile-time check: TransactFsFile implements afero.File
var _ afero.File = (*TransactFsFile)(nil)

// TransactFsFile wraps file operations with CopyOnWrite semantics.
type TransactFsFile struct {
	tfs      *TransactFs
	path     string
	pos      int64
	isWrite  bool
	inner    afero.File // Only set for write mode
	deleted  bool       // True if file was deleted while handle was open
	snapshot []byte     // Content snapshot (set when deleted)
	dirFile  afero.File // For directory operations in read mode
}

func (w *TransactFsFile) Close() error {
	w.tfs.mu.Lock()
	delete(w.tfs.openHandles, w)
	w.tfs.mu.Unlock()

	if w.dirFile != nil {
		w.dirFile.Close()
	}
	if w.inner != nil {
		return w.inner.Close()
	}
	return nil
}

func (w *TransactFsFile) Read(p []byte) (int, error) {
	if w.isWrite && w.inner != nil {
		return w.inner.Read(p)
	}

	// Read from appropriate source
	var content []byte
	var err error

	if w.deleted {
		// Read from snapshot
		content = w.snapshot
	} else {
		// Read from cow overlay
		w.tfs.mu.RLock()
		content, err = w.tfs.readFileLocked(w.path)
		w.tfs.mu.RUnlock()
		if err != nil {
			return 0, err
		}
	}

	if w.pos >= int64(len(content)) {
		return 0, io.EOF
	}

	n := copy(p, content[w.pos:])
	w.pos += int64(n)
	return n, nil
}

func (w *TransactFsFile) ReadAt(p []byte, off int64) (int, error) {
	if w.isWrite && w.inner != nil {
		return w.inner.ReadAt(p, off)
	}

	var content []byte
	var err error

	if w.deleted {
		content = w.snapshot
	} else {
		w.tfs.mu.RLock()
		content, err = w.tfs.readFileLocked(w.path)
		w.tfs.mu.RUnlock()
		if err != nil {
			return 0, err
		}
	}

	if off >= int64(len(content)) {
		return 0, io.EOF
	}

	n := copy(p, content[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (w *TransactFsFile) Seek(offset int64, whence int) (int64, error) {
	if w.isWrite && w.inner != nil {
		return w.inner.Seek(offset, whence)
	}

	var size int64
	if w.deleted {
		size = int64(len(w.snapshot))
	} else {
		w.tfs.mu.RLock()
		content, err := w.tfs.readFileLocked(w.path)
		w.tfs.mu.RUnlock()
		if err != nil {
			return 0, err
		}
		size = int64(len(content))
	}

	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = w.pos + offset
	case io.SeekEnd:
		newPos = size + offset
	}

	if newPos < 0 {
		return 0, os.ErrInvalid
	}
	w.pos = newPos
	return newPos, nil
}

func (w *TransactFsFile) Write(p []byte) (int, error) {
	if w.inner != nil {
		n, err := w.inner.Write(p)
		if err == nil {
			w.tfs.mu.Lock()
			w.tfs.trackPath(w.path)
			w.tfs.mu.Unlock()
		}
		return n, err
	}
	return 0, os.ErrPermission
}

func (w *TransactFsFile) WriteAt(p []byte, off int64) (int, error) {
	if w.inner != nil {
		n, err := w.inner.WriteAt(p, off)
		if err == nil {
			w.tfs.mu.Lock()
			w.tfs.trackPath(w.path)
			w.tfs.mu.Unlock()
		}
		return n, err
	}
	return 0, os.ErrPermission
}

func (w *TransactFsFile) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *TransactFsFile) Name() string {
	return w.path
}

func (w *TransactFsFile) Readdir(count int) ([]os.FileInfo, error) {
	if w.inner != nil {
		return w.inner.Readdir(count)
	}

	// For read mode, open directory from cow if not already open
	if w.dirFile == nil {
		w.tfs.mu.RLock()
		cow := afero.NewCopyOnWriteFs(w.tfs.actual, w.tfs.staged)
		f, err := cow.Open(w.path)
		w.tfs.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		w.dirFile = f
	}
	return w.dirFile.Readdir(count)
}

func (w *TransactFsFile) Readdirnames(n int) ([]string, error) {
	if w.inner != nil {
		return w.inner.Readdirnames(n)
	}

	// For read mode, open directory from cow if not already open
	if w.dirFile == nil {
		w.tfs.mu.RLock()
		cow := afero.NewCopyOnWriteFs(w.tfs.actual, w.tfs.staged)
		f, err := cow.Open(w.path)
		w.tfs.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		w.dirFile = f
	}
	return w.dirFile.Readdirnames(n)
}

func (w *TransactFsFile) Stat() (os.FileInfo, error) {
	if w.inner != nil {
		return w.inner.Stat()
	}

	w.tfs.mu.RLock()
	defer w.tfs.mu.RUnlock()

	// Try staged first
	if info, err := w.tfs.staged.Stat(w.path); err == nil {
		return info, nil
	}
	// Fallback to actual
	return w.tfs.actual.Stat(w.path)
}

func (w *TransactFsFile) Sync() error {
	if w.inner != nil {
		return w.inner.Sync()
	}
	return nil
}

func (w *TransactFsFile) Truncate(size int64) error {
	if w.inner != nil {
		return w.inner.Truncate(size)
	}
	return os.ErrPermission
}
