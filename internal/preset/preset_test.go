package preset

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

const (
	flowCacheDir  = "/home/user/.alcatraz/cache-presets"
	flowRawURL    = "git+https://github.com/myorg/presets.git#a1b2c3d:backend"
	flowCachePath = "github.com/git-https/-/myorg/presets-git"
	flowRepoDir   = flowCacheDir + "/" + flowCachePath
	flowCommit    = "a1b2c3d"
	flowDirPath   = "backend"
)

// setupFlow creates a PresetEnv, MockCommandRunner, and output buffer for testing.
func setupFlow() (*PresetEnv, *util.MockCommandRunner, *bytes.Buffer) {
	fs := afero.NewMemMapFs()
	cmd := util.NewMockCommandRunner()
	env := NewPresetEnv(fs, cmd)
	var buf bytes.Buffer
	return env, cmd, &buf
}

// expectEnsureRepo pre-creates the cache directory and sets up mock expectations
// for EnsureRepo with a specific commit already present in cache.
func expectEnsureRepo(env *PresetEnv, cmd *util.MockCommandRunner, commitHash string) {
	// Pre-create cache dir so EnsureRepo skips git init --bare.
	_ = env.Fs.MkdirAll(flowRepoDir, 0o755)
	cmd.ExpectSuccess(gitCatFileCmd(flowRepoDir, commitHash), []byte("commit\n"))
}

// selectAll returns a PromptFileSelection that selects all files.
func selectAll(files []string) ([]string, error) {
	return files, nil
}

// selectNone returns a PromptFileSelection that selects nothing (user cancels).
func selectNone(_ []string) ([]string, error) {
	return nil, nil
}

// alwaysOverwrite returns a PromptOverwrite that always says yes.
func alwaysOverwrite(_ string) (bool, error) {
	return true, nil
}

// neverOverwrite returns a PromptOverwrite that always says no.
func neverOverwrite(_ string) (bool, error) {
	return false, nil
}

func TestRunPresetFlow_HappyPath(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	// EnsureRepo: commit already present
	expectEnsureRepo(env, cmd, flowCommit)

	// ListFiles
	lsOutput := ".alca.node.toml\n.alca.python.toml\n"
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(lsOutput))

	// CheckoutFile for both files
	nodeContent := []byte("[node]\nversion = \"20\"\n")
	pythonContent := []byte("[python]\nversion = \"3.12\"\n")
	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, "backend/.alca.node.toml"), nodeContent)
	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, "backend/.alca.python.toml"), pythonContent)

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectAll, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify files were written with source comments
	got, err := afero.ReadFile(env.Fs, "/project/.alca.node.toml")
	if err != nil {
		t.Fatalf("reading node file: %v", err)
	}
	if _, err := ParseSourceComment(got); err != nil {
		t.Errorf("node file: failed to parse source comment: %v", err)
	}
	if !strings.Contains(string(got), "[node]") {
		t.Error("node file missing original content")
	}

	got, err = afero.ReadFile(env.Fs, "/project/.alca.python.toml")
	if err != nil {
		t.Fatalf("reading python file: %v", err)
	}
	if _, err := ParseSourceComment(got); err != nil {
		t.Errorf("python file: failed to parse source comment: %v", err)
	}

	// Verify output messages
	output := buf.String()
	if !strings.Contains(output, "Downloaded .alca.node.toml") {
		t.Errorf("output missing download message for node, got: %s", output)
	}
	if !strings.Contains(output, "Downloaded .alca.python.toml") {
		t.Errorf("output missing download message for python, got: %s", output)
	}
}

func TestRunPresetFlow_CredentialWarning(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	rawURL := "git+https://token@gitea.company.com/team/presets"
	cloneURL := "https://token@gitea.company.com/team/presets"
	cachePath := "gitea.company.com/git-https/token/team/presets"
	repoDir := flowCacheDir + "/" + cachePath
	commit := "abc123"

	cmd.ExpectSuccess(gitInitBareCmd(repoDir), nil)
	cmd.ExpectSuccess(gitFetchCmd(repoDir, cloneURL, "HEAD"), nil)
	cmd.ExpectSuccess(gitRevParseCmd(repoDir, "FETCH_HEAD"), []byte(commit+"\n"))
	cmd.ExpectSuccess(gitLsTreeCmd(repoDir, commit+":"), []byte(".alca.node.toml\n"))
	cmd.ExpectSuccess(gitShowCmd(repoDir, commit, ".alca.node.toml"), []byte("[node]\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, rawURL, "/project", selectAll, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Warning: The URL contains credentials") {
		t.Errorf("expected credential warning, got: %s", output)
	}
}

func TestRunPresetFlow_NoCredentialWarning(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(".alca.node.toml\n"))
	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, "backend/.alca.node.toml"), []byte("[node]\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectAll, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(buf.String(), "Warning") {
		t.Errorf("unexpected warning for URL without credentials: %s", buf.String())
	}
}

func TestRunPresetFlow_FileSelection(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath),
		[]byte(".alca.node.toml\n.alca.python.toml\n.alca.go.toml\n"))

	// Only select node
	selectOne := func(files []string) ([]string, error) {
		if len(files) != 3 {
			t.Errorf("expected 3 files offered, got %d", len(files))
		}
		return []string{".alca.node.toml"}, nil
	}

	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, "backend/.alca.node.toml"), []byte("[node]\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectOne, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only node should be downloaded
	if !strings.Contains(buf.String(), "Downloaded .alca.node.toml") {
		t.Error("expected node download message")
	}
	if strings.Contains(buf.String(), "Downloaded .alca.python.toml") {
		t.Error("unexpected python download message")
	}
}

func TestRunPresetFlow_OverwritePrompt_Overwrite(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	// Pre-create existing file
	if err := afero.WriteFile(env.Fs, "/project/.alca.node.toml", []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(".alca.node.toml\n"))
	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, "backend/.alca.node.toml"), []byte("[node]\nnew = true\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectAll, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := afero.ReadFile(env.Fs, "/project/.alca.node.toml")
	if strings.Contains(string(got), "old content") {
		t.Error("file should have been overwritten")
	}
	if !strings.Contains(string(got), "new = true") {
		t.Error("file should contain new content")
	}
}

func TestRunPresetFlow_OverwritePrompt_Skip(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	// Pre-create existing file
	if err := afero.WriteFile(env.Fs, "/project/.alca.node.toml", []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(".alca.node.toml\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectAll, neverOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := afero.ReadFile(env.Fs, "/project/.alca.node.toml")
	if !strings.Contains(string(got), "old content") {
		t.Error("file should NOT have been overwritten")
	}
	if strings.Contains(buf.String(), "Downloaded") {
		t.Error("no download message expected when skipping")
	}
}

func TestRunPresetFlow_NoFilesFound(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	expectEnsureRepo(env, cmd, flowCommit)
	// ls-tree returns no matching files
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte("README.md\n.gitignore\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectAll, alwaysOverwrite, buf)
	if !errors.Is(err, ErrNoPresetFiles) {
		t.Fatalf("expected ErrNoPresetFiles, got %v", err)
	}
}

func TestRunPresetFlow_NoFilesFound_RepoRoot(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	rawURL := "git+https://github.com/myorg/presets.git#a1b2c3d"
	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"), []byte("README.md\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, rawURL, "/project", selectAll, alwaysOverwrite, buf)
	if !errors.Is(err, ErrNoPresetFiles) {
		t.Fatalf("expected ErrNoPresetFiles, got %v", err)
	}
}

func TestRunPresetFlow_EmptySelection(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(".alca.node.toml\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectNone, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No files should have been written
	exists, _ := afero.Exists(env.Fs, "/project/.alca.node.toml")
	if exists {
		t.Error("no file should be written when selection is empty")
	}
}

func TestRunPresetFlow_DirPathJoining(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(".alca.node.toml\n"))
	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, "backend/.alca.node.toml"), []byte("[node]\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectAll, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the source comment contains the full in-repo path (dirPath + filename)
	got, _ := afero.ReadFile(env.Fs, "/project/.alca.node.toml")
	info, err := ParseSourceComment(got)
	if err != nil {
		t.Fatalf("parsing source comment: %v", err)
	}
	if info.FilePath != "backend/.alca.node.toml" {
		t.Errorf("FilePath = %q, want %q", info.FilePath, "backend/.alca.node.toml")
	}
}

func TestRunPresetFlow_NoDirPath(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	rawURL := "git+https://github.com/myorg/presets.git#a1b2c3d"
	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"), []byte(".alca.node.toml\n"))
	// Without DirPath, repo file path is just the filename
	cmd.ExpectSuccess(gitShowCmd(flowRepoDir, flowCommit, ".alca.node.toml"), []byte("[node]\n"))

	err := RunPresetFlow(ctx, env, flowCacheDir, rawURL, "/project", selectAll, alwaysOverwrite, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := afero.ReadFile(env.Fs, "/project/.alca.node.toml")
	// Source comment should have just the filename (no dir prefix)
	info, err := ParseSourceComment(got)
	if err != nil {
		t.Fatalf("parsing source comment: %v", err)
	}
	if info.FilePath != ".alca.node.toml" {
		t.Errorf("FilePath = %q, want %q", info.FilePath, ".alca.node.toml")
	}
}

func TestRunPresetFlow_InvalidURL(t *testing.T) {
	env, _, buf := setupFlow()
	ctx := context.Background()

	err := RunPresetFlow(ctx, env, flowCacheDir, "https://not-git-plus.com/repo", "/project", selectAll, alwaysOverwrite, buf)
	if !errors.Is(err, ErrInvalidPresetURL) {
		t.Fatalf("expected ErrInvalidPresetURL, got %v", err)
	}
}

func TestRunPresetFlow_SelectionError(t *testing.T) {
	env, cmd, buf := setupFlow()
	defer cmd.AssertAllExpectationsMet(t)
	ctx := context.Background()

	expectEnsureRepo(env, cmd, flowCommit)
	cmd.ExpectSuccess(gitLsTreeCmd(flowRepoDir, flowCommit+":"+flowDirPath), []byte(".alca.node.toml\n"))

	errUserInterrupted := errors.New("user interrupted")
	selectErr := func(_ []string) ([]string, error) {
		return nil, errUserInterrupted
	}

	err := RunPresetFlow(ctx, env, flowCacheDir, flowRawURL, "/project", selectErr, alwaysOverwrite, buf)
	if !errors.Is(err, errUserInterrupted) {
		t.Fatalf("expected errUserInterrupted, got %v", err)
	}
}
