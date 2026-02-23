package preset

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

func newTestCacheManager() (*CacheManager, *util.MockCommandRunner) {
	fs := afero.NewMemMapFs()
	cmd := util.NewMockCommandRunner()
	env := NewPresetEnv(fs, cmd)
	cm := NewCacheManager(env, "/home/user/.alcatraz/cache-presets")
	return cm, cmd
}

func TestEnsureRepo_FreshClone(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	cloneURL := "https://github.com/myorg/presets"
	cachePath := "github.com/git-https/-/myorg/presets"
	expectedDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"

	cmd.ExpectSuccess(gitInitBareCmd(expectedDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(expectedDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(expectedDir, "FETCH_HEAD"), []byte("abc123def456\n"))

	repoDir, commit, err := cm.EnsureRepo(ctx, cloneURL, cachePath, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoDir != expectedDir {
		t.Errorf("repoDir = %q, want %q", repoDir, expectedDir)
	}
	if commit != "abc123def456" {
		t.Errorf("commit = %q, want %q", commit, "abc123def456")
	}
}

func TestEnsureRepo_CacheExists_NoCommit(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	cloneURL := "https://github.com/myorg/presets"
	cachePath := "github.com/git-https/-/myorg/presets"
	expectedDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"

	// Pre-create the cache directory to simulate existing cache.
	if err := cm.env.Fs.MkdirAll(expectedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd.ExpectSuccess(gitFetchCmd(expectedDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(expectedDir, "FETCH_HEAD"), []byte("newcommit789\n"))

	repoDir, commit, err := cm.EnsureRepo(ctx, cloneURL, cachePath, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoDir != expectedDir {
		t.Errorf("repoDir = %q, want %q", repoDir, expectedDir)
	}
	if commit != "newcommit789" {
		t.Errorf("commit = %q, want %q", commit, "newcommit789")
	}

	cmd.AssertNotCalled(t, gitInitBareCmd(expectedDir))
}

func TestEnsureRepo_CommitSpecified_AlreadyPresent(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	cloneURL := "https://github.com/myorg/presets"
	cachePath := "github.com/git-https/-/myorg/presets"
	expectedDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"

	// Pre-create the cache directory.
	if err := cm.env.Fs.MkdirAll(expectedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd.ExpectSuccess(gitCatFileCmd(expectedDir, commitHash), []byte("commit\n"))

	repoDir, commit, err := cm.EnsureRepo(ctx, cloneURL, cachePath, commitHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoDir != expectedDir {
		t.Errorf("repoDir = %q, want %q", repoDir, expectedDir)
	}
	if commit != commitHash {
		t.Errorf("commit = %q, want %q", commit, commitHash)
	}

	// No fetch should have occurred.
	cmd.AssertNotCalled(t, gitFetchCmd(expectedDir, cloneURL, commitHash))
}

func TestEnsureRepo_CommitSpecified_NotPresent(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	cloneURL := "https://github.com/myorg/presets"
	cachePath := "github.com/git-https/-/myorg/presets"
	expectedDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"

	// Pre-create the cache directory.
	if err := cm.env.Fs.MkdirAll(expectedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// cat-file returns non-commit (or errors), so commit is not present.
	cmd.ExpectFailure(gitCatFileCmd(expectedDir, commitHash), fmt.Errorf("not found"))
	cmd.ExpectSuccess(gitFetchCmd(expectedDir, cloneURL, commitHash), nil)

	repoDir, commit, err := cm.EnsureRepo(ctx, cloneURL, cachePath, commitHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoDir != expectedDir {
		t.Errorf("repoDir = %q, want %q", repoDir, expectedDir)
	}
	if commit != commitHash {
		t.Errorf("commit = %q, want %q", commit, commitHash)
	}
}

func TestEnsureRepo_FetchFailure(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	cloneURL := "https://github.com/myorg/presets"
	cachePath := "github.com/git-https/-/myorg/presets"
	expectedDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"

	errNetwork := errors.New("network error")
	cmd.ExpectSuccess(gitInitBareCmd(expectedDir), nil)
	cmd.ExpectFailure(gitFetchCmd(expectedDir, cloneURL, "HEAD"), errNetwork)

	_, _, err := cm.EnsureRepo(ctx, cloneURL, cachePath, "")
	if !errors.Is(err, errNetwork) {
		t.Fatalf("expected errNetwork, got %v", err)
	}
}

func TestCheckoutFile(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	repoDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"
	filePath := ".alca.node.toml"
	fileContent := []byte("[node]\nversion = \"20\"\n")

	cmd.ExpectSuccess(gitShowCmd(repoDir, commitHash, filePath), fileContent)

	out, err := cm.CheckoutFile(ctx, repoDir, commitHash, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(fileContent) {
		t.Errorf("output = %q, want %q", string(out), string(fileContent))
	}
}

func TestCheckoutFile_MissingFile(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	repoDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"
	filePath := "nonexistent.toml"

	cmd.ExpectFailure(gitShowCmd(repoDir, commitHash, filePath), fmt.Errorf("path not found"))

	_, err := cm.CheckoutFile(ctx, repoDir, commitHash, filePath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListFiles_Filtering(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	repoDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"
	dirPath := ""

	lsOutput := ".alca.node.toml\n.alca.python.toml\n.alca.go.toml.example\nREADME.md\n.gitignore\nrandom.toml\n.alca.toml\n"
	cmd.ExpectSuccess(gitLsTreeCmd(repoDir, commitHash+":"), []byte(lsOutput))

	files, err := cm.ListFiles(ctx, repoDir, commitHash, dirPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{".alca.node.toml", ".alca.python.toml", ".alca.go.toml.example"}
	if len(files) != len(expected) {
		t.Fatalf("got %d files, want %d: %v", len(files), len(expected), files)
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %q, want %q", i, f, expected[i])
		}
	}
}

func TestListFiles_Subdirectory(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	repoDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"
	dirPath := "backend"

	lsOutput := ".alca.backend.toml\nsetup.sh\n"
	cmd.ExpectSuccess(gitLsTreeCmd(repoDir, commitHash+":"+dirPath), []byte(lsOutput))

	files, err := cm.ListFiles(ctx, repoDir, commitHash, dirPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 || files[0] != ".alca.backend.toml" {
		t.Errorf("files = %v, want [.alca.backend.toml]", files)
	}
}

func TestListFiles_EmptyDirectory(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	repoDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"

	cmd.ExpectSuccess(gitLsTreeCmd(repoDir, commitHash+":"), []byte(""))

	files, err := cm.ListFiles(ctx, repoDir, commitHash, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestListFiles_Error(t *testing.T) {
	cm, cmd := newTestCacheManager()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	repoDir := "/home/user/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets"
	commitHash := "abc123"

	cmd.ExpectFailure(gitLsTreeCmd(repoDir, commitHash+":"), fmt.Errorf("bad revision"))

	_, err := cm.ListFiles(ctx, repoDir, commitHash, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMatchPresetFile(t *testing.T) {
	tests := []struct {
		name  string
		match bool
	}{
		{".alca.node.toml", true},
		{".alca.python.toml", true},
		{".alca.go.toml.example", true},
		{".alca.rust.toml.example", true},
		{".alca.toml", false},
		{"README.md", false},
		{".gitignore", false},
		{"random.toml", false},
		{".alca..toml", true}, // filepath.Match * matches empty string
		{".alca.x.toml.bak", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchPresetFile(tt.name); got != tt.match {
				t.Errorf("matchPresetFile(%q) = %v, want %v", tt.name, got, tt.match)
			}
		})
	}
}
