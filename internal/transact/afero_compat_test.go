// Copyright © 2014 Steve Francia <spf@spf13.com>.
// Copyright 2009 The Go Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This file is adapted from
//     https://github.com/spf13/afero/blob/2f116ee30d2f6341e95be5cc6772c798365e49fc/afero_test.go
// to test TransactFs compatibility with the afero.Fs interface.
//
// Minimal changes from original:
// - Fss contains only TransactFs (backed by MemMapFs)

package transact

import (
	"bytes"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"time"
	"sort"
	"github.com/spf13/afero"
)

// =============================================================================
// Test setup
// =============================================================================

// Fss contains only TransactFs (original tests MemMapFs and OsFs)
var (
	testName = "test.txt"
	Fss      = []afero.Fs{New(WithActualFs(afero.NewMemMapFs()))}
)

var testRegistry map[afero.Fs][]string = make(map[afero.Fs][]string)

func testDir(fs afero.Fs) string {
	name, err := afero.TempDir(fs, "", "afero")
	if err != nil {
		panic(fmt.Sprint("unable to work with test dir", err))
	}
	testRegistry[fs] = append(testRegistry[fs], name)
	return name
}

func tmpFile(fs afero.Fs) afero.File {
	x, err := afero.TempFile(fs, "", "afero")
	if err != nil {
		panic(fmt.Sprint("unable to work with temp file", err))
	}
	testRegistry[fs] = append(testRegistry[fs], x.Name())
	return x
}

// =============================================================================
// Tests - minimal changes from original
// =============================================================================

// Read with length 0 should not return EOF.
func TestAferoCompat_Read0(t *testing.T) {
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()
		f.WriteString(
			"Lorem ipsum dolor sit amet, consectetur adipisicing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.",
		)

		var b []byte
		// b := make([]byte, 0)
		n, err := f.Read(b)
		if n != 0 || err != nil {
			t.Errorf("%v: Read(0) = %d, %v, want 0, nil", fs.Name(), n, err)
		}
		f.Seek(0, 0)
		b = make([]byte, 100)
		n, err = f.Read(b)
		if n <= 0 || err != nil {
			t.Errorf("%v: Read(100) = %d, %v, want >0, nil", fs.Name(), n, err)
		}
	}
}

func TestAferoCompat_OpenFile(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		tmp := testDir(fs)
		path := filepath.Join(tmp, testName)

		f, err := fs.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Error(fs.Name(), "OpenFile (O_CREATE) failed:", err)
			continue
		}
		io.WriteString(f, "initial")
		f.Close()

		f, err = fs.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Error(fs.Name(), "OpenFile (O_APPEND) failed:", err)
			continue
		}
		io.WriteString(f, "|append")
		f.Close()

		f, _ = fs.OpenFile(path, os.O_RDONLY, 0o600)
		contents, _ := io.ReadAll(f)
		expectedContents := "initial|append"
		if string(contents) != expectedContents {
			t.Errorf(
				"%v: appending, expected '%v', got: '%v'",
				fs.Name(),
				expectedContents,
				string(contents),
			)
		}
		f.Close()

		f, err = fs.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0o600)
		if err != nil {
			t.Error(fs.Name(), "OpenFile (O_TRUNC) failed:", err)
			continue
		}
		contents, _ = io.ReadAll(f)
		if string(contents) != "" {
			t.Errorf("%v: expected truncated file, got: '%v'", fs.Name(), string(contents))
		}
		f.Close()
	}
}

func TestAferoCompat_Create(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		tmp := testDir(fs)
		path := filepath.Join(tmp, testName)

		f, err := fs.Create(path)
		if err != nil {
			t.Error(fs.Name(), "Create failed:", err)
			f.Close()
			continue
		}
		io.WriteString(f, "initial")
		f.Close()

		f, err = fs.Create(path)
		if err != nil {
			t.Error(fs.Name(), "Create failed:", err)
			f.Close()
			continue
		}
		secondContent := "second create"
		io.WriteString(f, secondContent)
		f.Close()

		f, err = fs.Open(path)
		if err != nil {
			t.Error(fs.Name(), "Open failed:", err)
			f.Close()
			continue
		}
		buf, err := io.ReadAll(f)
		if err != nil {
			t.Error(fs.Name(), "ReadAll failed:", err)
			f.Close()
			continue
		}
		if string(buf) != secondContent {
			t.Error(
				fs.Name(),
				"Content should be",
				"\""+secondContent+"\" but is \""+string(buf)+"\"",
			)
			f.Close()
			continue
		}
		f.Close()
	}
}

func TestAferoCompat_MemFileRead(t *testing.T) {
	for _, fs := range Fss {
		f := tmpFile(fs)
		f.WriteString("abcd")
		f.Seek(0, 0)
		b := make([]byte, 8)
		n, err := f.Read(b)
		if n != 4 {
			t.Errorf("%v: didn't read all bytes: %v %v %v", fs.Name(), n, err, b)
		}
		if err != nil {
			t.Errorf("%v: err is not nil: %v %v %v", fs.Name(), n, err, b)
		}
		n, err = f.Read(b)
		if n != 0 {
			t.Errorf("%v: read more bytes: %v %v %v", fs.Name(), n, err, b)
		}
		if err != io.EOF {
			t.Errorf("%v: error is not EOF: %v %v %v", fs.Name(), n, err, b)
		}
		f.Close()
	}
}

func TestAferoCompat_Rename(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		tDir := testDir(fs)
		from := filepath.Join(tDir, "/renamefrom")
		to := filepath.Join(tDir, "/renameto")
		exists := filepath.Join(tDir, "/renameexists")
		file, err := fs.Create(from)
		if err != nil {
			t.Fatalf("%s: open %q failed: %v", fs.Name(), to, err)
		}
		if err = file.Close(); err != nil {
			t.Errorf("%s: close %q failed: %v", fs.Name(), to, err)
		}
		file, err = fs.Create(exists)
		if err != nil {
			t.Fatalf("%s: open %q failed: %v", fs.Name(), to, err)
		}
		if err = file.Close(); err != nil {
			t.Errorf("%s: close %q failed: %v", fs.Name(), to, err)
		}
		err = fs.Rename(from, to)
		if err != nil {
			t.Fatalf("%s: rename %q, %q failed: %v", fs.Name(), to, from, err)
		}
		file, err = fs.Create(from)
		if err != nil {
			t.Fatalf("%s: open %q failed: %v", fs.Name(), to, err)
		}
		if err = file.Close(); err != nil {
			t.Errorf("%s: close %q failed: %v", fs.Name(), to, err)
		}
		err = fs.Rename(from, exists)
		if err != nil {
			t.Errorf("%s: rename %q, %q failed: %v", fs.Name(), exists, from, err)
		}
		names, err := readDirNames(fs, tDir)
		if err != nil {
			t.Errorf("%s: readDirNames error: %v", fs.Name(), err)
		}
		found := false
		for _, e := range names {
			if e == "renamefrom" {
				t.Error("File is still called renamefrom")
			}
			if e == "renameto" {
				found = true
			}
		}
		if !found {
			t.Error("File was not renamed to renameto")
		}

		_, err = fs.Stat(to)
		if err != nil {
			t.Errorf("%s: stat %q failed: %v", fs.Name(), to, err)
		}
	}
}

func TestAferoCompat_Remove(t *testing.T) {
	for _, fs := range Fss {

		x, err := afero.TempFile(fs, "", "afero")
		if err != nil {
			t.Error(fmt.Sprint("unable to work with temp file", err))
		}

		path := x.Name()
		x.Close()

		tDir := filepath.Dir(path)

		err = fs.Remove(path)
		if err != nil {
			t.Errorf("%v: Remove() failed: %v", fs.Name(), err)
			continue
		}

		_, err = fs.Stat(path)
		if !os.IsNotExist(err) {
			t.Errorf("%v: Remove() didn't remove file", fs.Name())
			continue
		}

		// Deleting non-existent file should raise error
		err = fs.Remove(path)
		if !os.IsNotExist(err) {
			t.Errorf("%v: Remove() didn't raise error for non-existent file", fs.Name())
		}

		f, err := fs.Open(tDir)
		if err != nil {
			t.Error("TestDir should still exist:", err)
		}

		names, err := f.Readdirnames(-1)
		if err != nil {
			t.Error("Readdirnames failed:", err)
		}

		for _, e := range names {
			if e == testName {
				t.Error("File was not removed from parent directory")
			}
		}
	}
}

func TestAferoCompat_Truncate(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		checkSize(t, f, 0)
		f.Write([]byte("hello, world\n"))
		checkSize(t, f, 13)
		f.Truncate(10)
		checkSize(t, f, 10)
		f.Truncate(1024)
		checkSize(t, f, 1024)
		f.Truncate(0)
		checkSize(t, f, 0)
		_, err := f.Write([]byte("surprise!"))
		if err == nil {
			checkSize(t, f, 13+9) // wrote at offset past where hello, world was.
		}
	}
}

func TestAferoCompat_Seek(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		const data = "hello, world\n"
		io.WriteString(f, data)

		type test struct {
			in     int64
			whence int
			out    int64
		}
		tests := []test{
			{0, 1, int64(len(data))},
			{0, 0, 0},
			{5, 0, 5},
			{0, 2, int64(len(data))},
			{0, 0, 0},
			{-1, 2, int64(len(data)) - 1},
			{1 << 33, 0, 1 << 33},
			{1 << 33, 2, 1<<33 + int64(len(data))},
		}
		for i, tt := range tests {
			off, err := f.Seek(tt.in, tt.whence)
			if off != tt.out || err != nil {
				if e, ok := err.(*os.PathError); ok && e.Err == syscall.EINVAL && tt.out > 1<<32 {
					// Reiserfs rejects the big seeks.
					// http://code.google.com/p/go/issues/detail?id=91
					break
				}
				t.Errorf(
					"#%d: Seek(%v, %v) = %v, %v want %v, nil",
					i,
					tt.in,
					tt.whence,
					off,
					err,
					tt.out,
				)
			}
		}
	}
}

func TestAferoCompat_ReadAt(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		const data = "hello, world\n"
		io.WriteString(f, data)

		b := make([]byte, 5)
		n, err := f.ReadAt(b, 7)
		if err != nil || n != len(b) {
			t.Fatalf("ReadAt 7: %d, %v", n, err)
		}
		if string(b) != "world" {
			t.Fatalf("ReadAt 7: have %q want %q", string(b), "world")
		}
	}
}

func TestAferoCompat_WriteAt(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		const data = "hello, world\n"
		io.WriteString(f, data)

		n, err := f.WriteAt([]byte("WORLD"), 7)
		if err != nil || n != 5 {
			t.Fatalf("WriteAt 7: %d, %v", n, err)
		}

		f2, err := fs.Open(f.Name())
		if err != nil {
			t.Fatalf("%v: ReadFile %s: %v", fs.Name(), f.Name(), err)
		}
		defer f2.Close()
		buf := new(bytes.Buffer)
		buf.ReadFrom(f2)
		b := buf.Bytes()
		if string(b) != "hello, WORLD\n" {
			t.Fatalf("after write: have %q want %q", string(b), "hello, WORLD\n")
		}

	}
}

// =============================================================================
// Directory tests
// =============================================================================

func setupTestDir(t *testing.T, fs afero.Fs) string {
	path := testDir(fs)
	return setupTestFiles(t, fs, path)
}

func setupTestFiles(t *testing.T, fs afero.Fs, path string) string {
	testSubDir := filepath.Join(path, "more", "subdirectories", "for", "testing", "we")
	err := fs.MkdirAll(testSubDir, 0o700)
	if err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}

	f, err := fs.Create(filepath.Join(testSubDir, "testfile1"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Testfile 1 content")
	f.Close()

	f, err = fs.Create(filepath.Join(testSubDir, "testfile2"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Testfile 2 content")
	f.Close()

	f, err = fs.Create(filepath.Join(testSubDir, "testfile3"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Testfile 3 content")
	f.Close()

	f, err = fs.Create(filepath.Join(testSubDir, "testfile4"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Testfile 4 content")
	f.Close()
	return testSubDir
}

func TestAferoCompat_Readdirnames(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		testSubDir := setupTestDir(t, fs)
		tDir := filepath.Dir(testSubDir)

		root, err := fs.Open(tDir)
		if err != nil {
			t.Fatal(fs.Name(), tDir, err)
		}
		defer root.Close()

		namesRoot, err := root.Readdirnames(-1)
		if err != nil {
			t.Fatal(fs.Name(), namesRoot, err)
		}

		sub, err := fs.Open(testSubDir)
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Close()

		namesSub, err := sub.Readdirnames(-1)
		if err != nil {
			t.Fatal(fs.Name(), namesSub, err)
		}

		findNames(fs, t, tDir, testSubDir, namesRoot, namesSub)
	}
}

func TestAferoCompat_ReaddirSimple(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		testSubDir := setupTestDir(t, fs)
		tDir := filepath.Dir(testSubDir)

		root, err := fs.Open(tDir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		rootInfo, err := root.Readdir(1)
		if err != nil {
			t.Log(myFileInfo(rootInfo))
			t.Error(err)
		}

		rootInfo, err = root.Readdir(5)
		if err != io.EOF {
			t.Log(myFileInfo(rootInfo))
			t.Error(err)
		}

		sub, err := fs.Open(testSubDir)
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Close()

		subInfo, err := sub.Readdir(5)
		if err != nil {
			t.Log(myFileInfo(subInfo))
			t.Error(err)
		}
	}
}

// TestAferoCompat_Readdir tests Readdir with various buffer sizes.
func TestAferoCompat_Readdir(t *testing.T) {
	defer removeAllTestFiles(t)
	const nums = 6
	for num := 0; num < nums; num++ {
		outputs := make([]string, len(Fss))
		infos := make([]string, len(Fss))
		for i, fs := range Fss {
			testSubDir := setupTestDir(t, fs)
			root, err := fs.Open(testSubDir)
			if err != nil {
				t.Fatal(err)
			}

			infosn := make([]string, nums)

			for j := 0; j < nums; j++ {
				info, err := root.Readdir(num)
				outputs[i] += fmt.Sprintf("%v  Error: %v\n", myFileInfo(info), err)
				s := fmt.Sprintln(len(info), err)
				infosn[j] = s
				infos[i] += s
			}
			root.Close()

			// Also check fs.ReadDirFile interface if implemented
			if _, ok := root.(iofs.ReadDirFile); ok {
				root, err = fs.Open(testSubDir)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()

				for j := 0; j < nums; j++ {
					dirEntries, err := root.(iofs.ReadDirFile).ReadDir(num)
					s := fmt.Sprintln(len(dirEntries), err)
					if s != infosn[j] {
						t.Fatalf("%s: %s != %s", fs.Name(), s, infosn[j])
					}
				}
			}
		}

		fail := false
		for i, o := range infos {
			if i == 0 {
				continue
			}
			if o != infos[i-1] {
				fail = true
				break
			}
		}
		if fail {
			t.Log("Readdir outputs not equal for Readdir(", num, ")")
			for i, o := range outputs {
				t.Log(Fss[i].Name())
				t.Log(o)
			}
			t.Fail()
		}
	}
}

type myFileInfo []os.FileInfo

func (m myFileInfo) String() string {
	out := "Fileinfos:\n"
	for _, e := range m {
		out += "  " + e.Name() + "\n"
	}
	return out
}

func TestAferoCompat_ReaddirAll(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		testSubDir := setupTestDir(t, fs)
		tDir := filepath.Dir(testSubDir)

		root, err := fs.Open(tDir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		rootInfo, err := root.Readdir(-1)
		if err != nil {
			t.Fatal(err)
		}
		namesRoot := []string{}
		for _, e := range rootInfo {
			namesRoot = append(namesRoot, e.Name())
		}

		sub, err := fs.Open(testSubDir)
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Close()

		subInfo, err := sub.Readdir(-1)
		if err != nil {
			t.Fatal(err)
		}
		namesSub := []string{}
		for _, e := range subInfo {
			namesSub = append(namesSub, e.Name())
		}

		findNames(fs, t, tDir, testSubDir, namesRoot, namesSub)
	}
}

// https://github.com/spf13/afero/issues/169
func TestAferoCompat_ReaddirRegularFile(t *testing.T) {
	defer removeAllTestFiles(t)
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		_, err := f.Readdirnames(-1)
		if err == nil {
			t.Fatal("Expected error")
		}

		_, err = f.Readdir(-1)
		if err == nil {
			t.Fatal("Expected error")
		}
	}
}

func findNames(fs afero.Fs, t *testing.T, tDir, testSubDir string, root, sub []string) {
	var foundRoot bool
	for _, e := range root {
		f, err := fs.Open(filepath.Join(tDir, e))
		if err != nil {
			t.Error("Open", filepath.Join(tDir, e), ":", err)
		}
		defer f.Close()

		if equal(e, "we") {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Logf("Names root: %v", root)
		t.Logf("Names sub: %v", sub)
		t.Error("Didn't find subdirectory we")
	}

	var found1, found2 bool
	for _, e := range sub {
		f, err := fs.Open(filepath.Join(testSubDir, e))
		if err != nil {
			t.Error("Open", filepath.Join(testSubDir, e), ":", err)
		}
		defer f.Close()

		if equal(e, "testfile1") {
			found1 = true
		}
		if equal(e, "testfile2") {
			found2 = true
		}
	}

	if !found1 {
		t.Logf("Names root: %v", root)
		t.Logf("Names sub: %v", sub)
		t.Error("Didn't find testfile1")
	}
	if !found2 {
		t.Logf("Names root: %v", root)
		t.Logf("Names sub: %v", sub)
		t.Error("Didn't find testfile2")
	}
}

// =============================================================================
// Helper functions
// =============================================================================

// readDirNames reads the directory named by dirname and returns
// a sorted list of directory entries.
// adapted from https://golang.org/src/path/filepath/path.go
func readDirNames(fs afero.Fs, dirname string) ([]string, error) {
	f, err := fs.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func removeAllTestFiles(t *testing.T) {
	for fs, list := range testRegistry {
		for _, path := range list {
			if err := fs.RemoveAll(path); err != nil {
				t.Error(fs.Name(), err)
			}
		}
	}
	testRegistry = make(map[afero.Fs][]string)
}

func equal(name1, name2 string) (r bool) {
	switch runtime.GOOS {
	case "windows":
		r = strings.EqualFold(name1, name2)
	default:
		r = name1 == name2
	}
	return
}

func checkSize(t *testing.T, f afero.File, size int64) {
	t.Helper()
	dir, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat %q (looking for size %d): %s", f.Name(), size, err)
	}
	if dir.Size() != size {
		t.Errorf("Stat %q: size %d want %d", f.Name(), dir.Size(), size)
	}
}

// =============================================================================
// TransactFs-specific afero compatibility tests
// =============================================================================

// TestAferoCompat_InterfaceCompliance verifies TransactFs implements afero.Fs.
func TestAferoCompat_InterfaceCompliance(t *testing.T) {
	var _ afero.Fs = (*TransactFs)(nil)
}

// TestAferoCompat_Stat tests the Stat method.
func TestAferoCompat_Stat(t *testing.T) {
	for _, fs := range Fss {
		path := "/test-stat.txt"
		content := []byte("test content")
		afero.WriteFile(fs, path, content, 0o644)

		info, err := fs.Stat(path)
		if err != nil {
			t.Fatalf("%v: Stat failed: %v", fs.Name(), err)
		}
		if info.Size() != int64(len(content)) {
			t.Errorf("%v: Stat size = %d, want %d", fs.Name(), info.Size(), len(content))
		}

		_, err = fs.Stat("/nonexistent")
		if !os.IsNotExist(err) {
			t.Errorf("%v: Stat nonexistent: expected IsNotExist, got %v", fs.Name(), err)
		}
	}
}

// TestAferoCompat_Name tests the Name method.
func TestAferoCompat_Name(t *testing.T) {
	for _, fs := range Fss {
		name := fs.Name()
		if name == "" {
			t.Errorf("%v: Name() returned empty string", fs.Name())
		}
	}
}

// TestAferoCompat_Mkdir tests the Mkdir method.
func TestAferoCompat_Mkdir(t *testing.T) {
	for _, fs := range Fss {
		err := fs.Mkdir("/testdir-mkdir", 0o755)
		if err != nil {
			t.Fatalf("%v: Mkdir failed: %v", fs.Name(), err)
		}

		info, err := fs.Stat("/testdir-mkdir")
		if err != nil {
			t.Fatalf("%v: Stat failed: %v", fs.Name(), err)
		}
		if !info.IsDir() {
			t.Errorf("%v: Mkdir didn't create a directory", fs.Name())
		}

		err = fs.Mkdir("/testdir-mkdir", 0o755)
		if err == nil {
			t.Errorf("%v: Mkdir existing dir should fail", fs.Name())
		}
	}
}

// TestAferoCompat_RemoveAll tests the RemoveAll method.
func TestAferoCompat_RemoveAll(t *testing.T) {
	for _, fs := range Fss {
		fs.MkdirAll("/testdir-removeall/subdir", 0o755)
		afero.WriteFile(fs, "/testdir-removeall/file1.txt", []byte("1"), 0o644)
		afero.WriteFile(fs, "/testdir-removeall/subdir/file2.txt", []byte("2"), 0o644)

		err := fs.RemoveAll("/testdir-removeall")
		if err != nil {
			t.Fatalf("%v: RemoveAll failed: %v", fs.Name(), err)
		}

		_, err = fs.Stat("/testdir-removeall")
		if !os.IsNotExist(err) {
			t.Errorf("%v: RemoveAll didn't remove directory", fs.Name())
		}

		err = fs.RemoveAll("/nonexistent-removeall")
		if err != nil {
			t.Errorf("%v: RemoveAll nonexistent should not error: %v", fs.Name(), err)
		}
	}
}

// TestAferoCompat_CopyOnWrite_ReadFromActual tests CopyOnWrite read semantics.
func TestAferoCompat_CopyOnWrite_ReadFromActual(t *testing.T) {
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/actual-file.txt", []byte("actual content"), 0o644)

	tfs := New(WithActualFs(actual))

	content, err := afero.ReadFile(tfs, "/actual-file.txt")
	if err != nil {
		t.Fatalf("ReadFile from actual failed: %v", err)
	}
	if string(content) != "actual content" {
		t.Errorf("ReadFile = %q, want %q", string(content), "actual content")
	}
}

// TestAferoCompat_CopyOnWrite_WriteToStaged tests CopyOnWrite write semantics.
func TestAferoCompat_CopyOnWrite_WriteToStaged(t *testing.T) {
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/file.txt", []byte("original"), 0o644)

	tfs := New(WithActualFs(actual))

	f, err := tfs.OpenFile("/file.txt", os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	f.WriteString("modified")
	f.Close()

	content, _ := afero.ReadFile(tfs, "/file.txt")
	if string(content) != "modified" {
		t.Errorf("tfs.ReadFile = %q, want %q", string(content), "modified")
	}

	actualContent, _ := afero.ReadFile(actual, "/file.txt")
	if string(actualContent) != "original" {
		t.Errorf("actual content changed: %q", string(actualContent))
	}
}

// TestAferoCompat_FileSync tests that Sync doesn't error.
func TestAferoCompat_FileSync(t *testing.T) {
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		f.WriteString("test")
		if err := f.Sync(); err != nil {
			t.Errorf("%v: Sync failed: %v", fs.Name(), err)
		}
	}
}

// TestAferoCompat_FileWriteString tests WriteString method.
func TestAferoCompat_FileWriteString(t *testing.T) {
	for _, fs := range Fss {
		f := tmpFile(fs)
		defer f.Close()

		n, err := f.WriteString("hello world")
		if err != nil {
			t.Fatalf("%v: WriteString failed: %v", fs.Name(), err)
		}
		if n != 11 {
			t.Errorf("%v: WriteString returned %d, want 11", fs.Name(), n)
		}

		f.Seek(0, 0)
		content, _ := io.ReadAll(f)
		if string(content) != "hello world" {
			t.Errorf("%v: content = %q, want %q", fs.Name(), string(content), "hello world")
		}
	}
}

// TestAferoCompat_Chown tests the Chown method.
func TestAferoCompat_Chown(t *testing.T) {
	for _, fs := range Fss {
		afero.WriteFile(fs, "/test-chown.txt", []byte("content"), 0o644)
		// Chown may or may not be supported, just ensure it doesn't panic
		_ = fs.Chown("/test-chown.txt", os.Getuid(), os.Getgid())
	}
}

// TestAferoCompat_Chtimes tests the Chtimes method.
func TestAferoCompat_Chtimes(t *testing.T) {
	for _, fs := range Fss {
		afero.WriteFile(fs, "/test-chtimes.txt", []byte("content"), 0o644)
		// Chtimes may or may not be supported, just ensure it doesn't panic
		now := time.Now()
		_ = fs.Chtimes("/test-chtimes.txt", now, now)
	}
}
