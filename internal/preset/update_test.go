package preset

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

const (
	updateCacheDir = "/home/user/.alcatraz/cache-presets"
	testScanDir    = "/project"
)

func newTestUpdateEnv() (*PresetEnv, *util.MockCommandRunner, afero.Fs) {
	fs := afero.NewMemMapFs()
	cmd := util.NewMockCommandRunner()
	env := NewPresetEnv(fs, cmd)
	return env, cmd, fs
}

// writeLocalFile creates a local .alca.*.toml file with a source comment.
func writeLocalFile(t *testing.T, fs afero.Fs, name, cloneURL, commit, repoFilePath, body string) {
	t.Helper()
	content := FormatSourceComment(cloneURL, commit, repoFilePath) + body
	if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func TestRunUpdateFlow_HappyPath(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	repoFilePath := ".alca.node.toml"
	newCommit := "newcommit123"
	newContent := "image = \"node:22\"\n"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, "oldcommit", repoFilePath, "image = \"node:20\"\n")

	// EnsureRepo: cache dir exists (created by writeLocalFile dir structure doesn't matter,
	// but EnsureRepo checks via DirExists — mock the git commands)
	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "FETCH_HEAD"), []byte(newCommit+"\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir, newCommit, repoFilePath), []byte(newContent))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was updated with new content and new commit hash.
	got, err := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}

	wantComment := FormatSourceComment(cloneURL, newCommit, repoFilePath)
	if !strings.HasPrefix(string(got), wantComment) {
		t.Errorf("file should start with updated source comment:\n got: %q\nwant prefix: %q", string(got), wantComment)
	}
	if !strings.Contains(string(got), newContent) {
		t.Errorf("file should contain new content %q, got: %q", newContent, string(got))
	}
}

func TestRunUpdateFlow_NoManagedFiles(t *testing.T) {
	env, _, fs := newTestUpdateEnv()
	ctx := context.Background()
	var buf bytes.Buffer

	// Create a file without source comment.
	if err := afero.WriteFile(fs, testScanDir+"/.alca.node.toml", []byte("image = \"node:20\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %q", buf.String())
	}
}

func TestRunUpdateFlow_EmptyDirectory(t *testing.T) {
	env, _, fs := newTestUpdateEnv()
	ctx := context.Background()
	var buf bytes.Buffer

	if err := fs.MkdirAll(testScanDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUpdateFlow_SourceFileDeleted(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	repoFilePath := ".alca.node.toml"
	originalContent := "image = \"node:20\"\n"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, "oldcommit", repoFilePath, originalContent)

	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "FETCH_HEAD"), []byte("newcommit\n"))
	// File no longer exists in repo.
	cmd.ExpectFailure(gitShowCmd(repoDir, "newcommit", repoFilePath), fmt.Errorf("path not found"))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warning should be printed.
	if !strings.Contains(buf.String(), "no longer exists") {
		t.Errorf("expected warning about missing file, got: %q", buf.String())
	}

	// Local file should be untouched.
	got, _ := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")
	if !strings.Contains(string(got), originalContent) {
		t.Errorf("local file should be untouched, got: %q", string(got))
	}
}

func TestRunUpdateFlow_MultipleFilesSameRepo(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	newCommit := "newcommit456"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, "old1", ".alca.node.toml", "version = 22\n")
	writeLocalFile(t, fs, testScanDir+"/.alca.python.toml", cloneURL, "old2", ".alca.python.toml", "version = 3.12\n")

	// Only one fetch for the repo.
	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "FETCH_HEAD"), []byte(newCommit+"\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir, newCommit, ".alca.node.toml"), []byte("version = 22\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir, newCommit, ".alca.python.toml"), []byte("version = 3.12\n"))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both files should be updated.
	for _, name := range []string{".alca.node.toml", ".alca.python.toml"} {
		got, _ := afero.ReadFile(fs, testScanDir+"/"+name)
		info, err := ParseSourceComment(got)
		if err != nil {
			t.Fatalf("%s: parsing source comment: %v", name, err)
		}
		if info.CommitHash != newCommit {
			t.Errorf("%s CommitHash = %q, want %q", name, info.CommitHash, newCommit)
		}
	}

	// Verify only one fetch occurred.
	fetchKey := gitFetchCmd(repoDir, cloneURL, "HEAD")
	if count := cmd.CallCount(fetchKey); count != 1 {
		t.Errorf("expected 1 fetch call, got %d", count)
	}
}

func TestRunUpdateFlow_MultipleRepos(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL1 := "https://github.com/myorg/presets"
	repoDir1 := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	cloneURL2 := "https://github.com/other/configs"
	repoDir2 := updateCacheDir + "/github.com/git-https/-/other/configs"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL1, "old1", ".alca.node.toml", "version = 22\n")
	writeLocalFile(t, fs, testScanDir+"/.alca.python.toml", cloneURL2, "old2", ".alca.python.toml", "version = 3.12\n")

	// Repo 1.
	cmd.ExpectSuccess(gitInitBareCmd(repoDir1), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir1, cloneURL1, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir1, "FETCH_HEAD"), []byte("commit1\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir1, "commit1", ".alca.node.toml"), []byte("version = 22\nupdated\n"))

	// Repo 2.
	cmd.ExpectSuccess(gitInitBareCmd(repoDir2), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir2, cloneURL2, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir2, "FETCH_HEAD"), []byte("commit2\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir2, "commit2", ".alca.python.toml"), []byte("version = 3.12\nupdated\n"))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both files should be updated with their respective new commits.
	got1, _ := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")
	info1, err := ParseSourceComment(got1)
	if err != nil {
		t.Fatalf(".alca.node.toml: parsing source comment: %v", err)
	}
	if info1.CommitHash != "commit1" {
		t.Errorf(".alca.node.toml CommitHash = %q, want %q", info1.CommitHash, "commit1")
	}

	got2, _ := afero.ReadFile(fs, testScanDir+"/.alca.python.toml")
	info2, err := ParseSourceComment(got2)
	if err != nil {
		t.Fatalf(".alca.python.toml: parsing source comment: %v", err)
	}
	if info2.CommitHash != "commit2" {
		t.Errorf(".alca.python.toml CommitHash = %q, want %q", info2.CommitHash, "commit2")
	}
}

func TestRunUpdateFlow_NetworkFailure_NoCache(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	originalContent := "image = \"node:20\"\n"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, "oldcommit", ".alca.node.toml", originalContent)

	// EnsureRepo fails (network error). Dir gets created but has no commits.
	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectFailure(gitFetchCmd(repoDir, cloneURL, "HEAD"), fmt.Errorf("network unreachable"))
	// rev-parse HEAD fails because the bare repo has no commits yet.
	cmd.ExpectFailure(gitRevParseCmd(repoDir, "HEAD"), fmt.Errorf("unknown revision HEAD"))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("expected nil error on network failure, got: %v", err)
	}

	// Warning should mention no cache available.
	if !strings.Contains(buf.String(), "No cache available, skipping") {
		t.Errorf("expected 'No cache available, skipping' warning, got: %q", buf.String())
	}

	// Local file should be untouched.
	got, _ := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")
	if !strings.Contains(string(got), originalContent) {
		t.Errorf("local file should be untouched after network failure, got: %q", string(got))
	}
}

func TestRunUpdateFlow_NetworkFailure_StaleCache(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	repoFilePath := ".alca.node.toml"
	staleCommit := "stale999"
	staleContent := "image = \"node:21\"\n"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, "oldcommit", repoFilePath, "image = \"node:20\"\n")

	// Pre-create cache directory (simulates a previous successful fetch).
	if err := fs.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// EnsureRepo: dir already exists, so no git init. Fetch fails (network error).
	cmd.ExpectFailure(gitFetchCmd(repoDir, cloneURL, "HEAD"), fmt.Errorf("network unreachable"))

	// Fallback: rev-parse HEAD succeeds with a stale commit.
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "HEAD"), []byte(staleCommit+"\n"))

	// CheckoutFile uses git show with the stale commit.
	cmd.ExpectSuccess(gitShowCmd(repoDir, staleCommit, repoFilePath), []byte(staleContent))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warning should mention using cached version.
	if !strings.Contains(buf.String(), "Using cached version") {
		t.Errorf("expected 'Using cached version' warning, got: %q", buf.String())
	}

	// File should be updated with stale cache content.
	got, err := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}

	wantComment := FormatSourceComment(cloneURL, staleCommit, repoFilePath)
	if !strings.HasPrefix(string(got), wantComment) {
		t.Errorf("file should start with stale commit source comment:\n got: %q\nwant prefix: %q", string(got), wantComment)
	}
	if !strings.Contains(string(got), staleContent) {
		t.Errorf("file should contain stale content %q, got: %q", staleContent, string(got))
	}
}

func TestRunUpdateFlow_CommitHashUpdated(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	repoFilePath := ".alca.node.toml"
	oldCommit := "aaa111"
	newCommit := "bbb222"
	newContent := "image = \"node:22\"\n"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, oldCommit, repoFilePath, "image = \"node:20\"\n")

	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "FETCH_HEAD"), []byte(newCommit+"\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir, newCommit, repoFilePath), []byte(newContent))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")

	// Old commit should NOT be present.
	info, err := ParseSourceComment(got)
	if err != nil {
		t.Fatalf("parsing source comment: %v", err)
	}
	if info.CommitHash == oldCommit {
		t.Errorf("CommitHash should not be old commit %q", oldCommit)
	}

	// New commit SHOULD be present in the source comment.
	expectedComment := FormatSourceComment(cloneURL, newCommit, repoFilePath)
	if !strings.HasPrefix(string(got), expectedComment) {
		t.Errorf("file should start with updated source comment:\n got: %q\nwant prefix: %q", string(got), expectedComment)
	}
}

func TestRunUpdateFlow_SubdirectoryFilePath(t *testing.T) {
	env, cmd, fs := newTestUpdateEnv()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()
	var buf bytes.Buffer

	cloneURL := "https://github.com/myorg/presets"
	repoDir := updateCacheDir + "/github.com/git-https/-/myorg/presets"
	repoFilePath := "backend/.alca.node.toml"
	newCommit := "newcommit"

	writeLocalFile(t, fs, testScanDir+"/.alca.node.toml", cloneURL, "oldcommit", repoFilePath, "[node]\n")

	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "FETCH_HEAD"), []byte(newCommit+"\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir, newCommit, repoFilePath), []byte("[node]\nupdated\n"))

	err := RunUpdateFlow(ctx, env, updateCacheDir, testScanDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := afero.ReadFile(fs, testScanDir+"/.alca.node.toml")
	expectedComment := FormatSourceComment(cloneURL, newCommit, repoFilePath)
	if !strings.HasPrefix(string(got), expectedComment) {
		t.Errorf("source comment should preserve subdirectory filepath:\n got: %q\nwant prefix: %q", string(got), expectedComment)
	}
}

func TestCachePathRoundTrip_GitSuffix(t *testing.T) {
	// A .git-suffixed URL must produce the same cache path through both:
	// 1. The preset flow: ParsePresetURL → CachePath
	// 2. The update flow: source comment → deriveCachePath
	//
	// These MUST be identical; otherwise the update flow creates a redundant clone.

	tests := []struct {
		name   string
		rawURL string
	}{
		{
			name:   "https with .git suffix",
			rawURL: "git+https://github.com/myorg/presets.git#abc123:backend/.alca.node.toml",
		},
		{
			name:   "https without .git suffix",
			rawURL: "git+https://github.com/myorg/presets#abc123:backend/.alca.node.toml",
		},
		{
			name:   "ssh with .git suffix",
			rawURL: "git+ssh://git@github.com/myorg/presets.git#abc123:.alca.node.toml",
		},
		{
			name:   "https with credentials and .git suffix",
			rawURL: "git+https://token@gitea.company.com/team/presets.git#def456:.alca.node.toml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Parse the original URL (preset flow).
			parsed, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("ParsePresetURL: %v", err)
			}
			presetCachePath := parsed.CachePath("")

			// Step 2: Simulate writing source comment with SourceBase (as preset flow does).
			repoFilePath := parsed.DirPath
			if repoFilePath == "" {
				repoFilePath = ".alca.node.toml"
			}
			comment := FormatSourceComment(parsed.SourceBase(), parsed.CommitHash, repoFilePath)

			// Step 3: Parse source comment back (as update flow does).
			info, err := ParseSourceComment([]byte(comment))
			if err != nil {
				t.Fatalf("ParseSourceComment: %v", err)
			}
			if info == nil {
				t.Fatal("expected SourceInfo, got nil")
			}

			// Step 4: Derive cache path from the source comment's RawURL (update flow).
			updateCachePath, err := deriveCachePath(info.RawURL)
			if err != nil {
				t.Fatalf("deriveCachePath: %v", err)
			}

			// Step 5: Both cache paths MUST be identical.
			if presetCachePath != updateCachePath {
				t.Errorf("cache path mismatch:\n  preset flow: %q\n  update flow: %q", presetCachePath, updateCachePath)
			}
		})
	}
}
