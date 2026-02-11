package runtime

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bolasblack/alcatraz/internal/util"
)

// TestShouldUseMutagen tests the decision logic for when to use Mutagen sync.
// Decision table from AGD-025:
// | Platform              | Condition    | Use Mutagen |
// |-----------------------|--------------|-------------|
// | Linux                 | Has excludes | Yes         |
// | Linux                 | No excludes  | No          |
// | macOS + Docker Desktop| Always       | Yes         |
// | macOS + OrbStack      | Has excludes | Yes         |
// | macOS + OrbStack      | No excludes  | No          |
func TestShouldUseMutagen(t *testing.T) {
	tests := []struct {
		name        string
		platform    RuntimePlatform
		hasExcludes bool
		expected    bool
	}{
		// Linux cases
		{
			name:        "Linux with excludes",
			platform:    PlatformLinux,
			hasExcludes: true,
			expected:    true,
		},
		{
			name:        "Linux without excludes",
			platform:    PlatformLinux,
			hasExcludes: false,
			expected:    false,
		},
		// macOS + Docker Desktop cases (always use Mutagen)
		{
			name:        "Docker Desktop with excludes",
			platform:    PlatformMacDockerDesktop,
			hasExcludes: true,
			expected:    true,
		},
		{
			name:        "Docker Desktop without excludes",
			platform:    PlatformMacDockerDesktop,
			hasExcludes: false,
			expected:    true, // Always use Mutagen on Docker Desktop
		},
		// macOS + OrbStack cases
		{
			name:        "OrbStack with excludes",
			platform:    PlatformMacOrbStack,
			hasExcludes: true,
			expected:    true,
		},
		{
			name:        "OrbStack without excludes",
			platform:    PlatformMacOrbStack,
			hasExcludes: false,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUseMutagen(tt.platform, tt.hasExcludes)
			if result != tt.expected {
				t.Errorf("ShouldUseMutagen(%v, %v) = %v, expected %v",
					tt.platform, tt.hasExcludes, result, tt.expected)
			}
		})
	}
}

// TestMutagenSyncBuildCreateArgs tests command construction for creating Mutagen sync sessions.
func TestMutagenSyncBuildCreateArgs(t *testing.T) {
	tests := []struct {
		name string
		sync MutagenSync
		want []string
	}{
		{
			name: "basic sync without ignores",
			sync: MutagenSync{
				Name:   "alca-project-workspace",
				Source: "/Users/me/project",
				Target: "docker://container-id/workspace",
			},
			want: []string{
				"sync", "create",
				"--name=alca-project-workspace",
				"/Users/me/project",
				"docker://container-id/workspace",
			},
		},
		{
			name: "sync with ignore patterns",
			sync: MutagenSync{
				Name:    "alca-project-workspace",
				Source:  "/Users/me/project",
				Target:  "docker://container-id/workspace",
				Ignores: []string{"**/.env.prod", "**/secrets/", "node_modules/"},
			},
			want: []string{
				"sync", "create",
				"--name=alca-project-workspace",
				"--ignore=**/.env.prod",
				"--ignore=**/secrets/",
				"--ignore=node_modules/",
				"/Users/me/project",
				"docker://container-id/workspace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.sync.buildCreateArgs()
			if !slices.Equal(args, tt.want) {
				t.Errorf("buildCreateArgs() = %v, want %v", args, tt.want)
			}
		})
	}
}

// TestMutagenSyncFlush_Success tests that Flush calls mutagen sync flush with the correct session name.
func TestMutagenSyncFlush_Success(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("mutagen sync flush alca-project-workspace", []byte(""))
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "alca-project-workspace"}
	err := sync.flushWithRetry(env, 1, 0)
	if err != nil {
		t.Fatalf("Flush() unexpected error: %v", err)
	}

	mock.AssertCalled(t, "mutagen sync flush alca-project-workspace")
}

// TestMutagenSyncFlush_NonRetryableError tests that Flush returns immediately on non-retryable errors.
func TestMutagenSyncFlush_NonRetryableError(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("mutagen sync flush test-session", errCommandNotFound)
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "test-session"}
	err := sync.flushWithRetry(env, 3, 0)
	if err == nil {
		t.Fatal("Flush() should return error when mutagen fails")
	}
	if !strings.Contains(err.Error(), "mutagen sync flush failed") {
		t.Errorf("Flush() error = %q, want it to contain 'mutagen sync flush failed'", err.Error())
	}
	// Should NOT retry on non-retryable error
	if mock.CallCount("mutagen sync flush test-session") != 1 {
		t.Errorf("expected 1 call, got %d", mock.CallCount("mutagen sync flush test-session"))
	}
}

// TestMutagenSyncFlush_RetriesOnNotReady tests that Flush retries when session is not yet connected.
func TestMutagenSyncFlush_RetriesOnNotReady(t *testing.T) {
	notReadyErr := errors.New("exit status 1")
	notReadyOutput := []byte("Error: unable to flush session: session is not currently able to synchronize")

	mock := util.NewMockCommandRunner()
	// First two attempts fail with retryable error, third succeeds
	mock.ExpectSequence("mutagen sync flush test-session", notReadyOutput, notReadyErr)
	mock.ExpectSequence("mutagen sync flush test-session", notReadyOutput, notReadyErr)
	mock.ExpectSequence("mutagen sync flush test-session", []byte(""), nil)
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "test-session"}
	err := sync.flushWithRetry(env, 5, 0)
	if err != nil {
		t.Fatalf("Flush() should succeed after retries, got: %v", err)
	}
	if mock.CallCount("mutagen sync flush test-session") != 3 {
		t.Errorf("expected 3 calls, got %d", mock.CallCount("mutagen sync flush test-session"))
	}
}

// TestMutagenSyncFlush_ExhaustsRetries tests that Flush fails after max retries.
func TestMutagenSyncFlush_ExhaustsRetries(t *testing.T) {
	notReadyErr := errors.New("exit status 1")
	notReadyOutput := []byte("Error: unable to flush session: session is not currently able to synchronize")

	mock := util.NewMockCommandRunner()
	mock.Expect("mutagen sync flush test-session", notReadyOutput, notReadyErr)
	mock.Expect("mutagen sync flush test-session", notReadyOutput, notReadyErr)
	mock.Expect("mutagen sync flush test-session", notReadyOutput, notReadyErr)
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "test-session"}
	err := sync.flushWithRetry(env, 3, 0)
	if err == nil {
		t.Fatal("Flush() should fail after exhausting retries")
	}
}

// TestIsFlushRetryable tests the retryable error detection.
func TestIsFlushRetryable(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{"Error: unable to flush session: session is not currently able to synchronize", true},
		{"Error: no matching sessions", false},
		{"command not found", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isFlushRetryable(tt.output); got != tt.want {
			t.Errorf("isFlushRetryable(%q) = %v, want %v", tt.output, got, tt.want)
		}
	}
}

// TestMutagenSyncBuildTerminateArgs tests command construction for terminating Mutagen sync sessions.
func TestMutagenSyncBuildTerminateArgs(t *testing.T) {
	sync := MutagenSync{
		Name: "alca-project-workspace",
	}

	args := sync.buildTerminateArgs()
	want := []string{"sync", "terminate", "alca-project-workspace"}

	if !slices.Equal(args, want) {
		t.Errorf("buildTerminateArgs() = %v, want %v", args, want)
	}
}

// TestListMutagenSyncsBuildArgs tests command construction for listing Mutagen sync sessions.
func TestListMutagenSyncsBuildArgs(t *testing.T) {
	args := buildListSyncsArgs()
	want := []string{"sync", "list", `--template={{range .}}{{.Name}}{{"\n"}}{{end}}`}

	if !slices.Equal(args, want) {
		t.Errorf("buildListSyncsArgs() = %v, want %v", args, want)
	}
}

// TestBuildListSessionJSONArgs tests command construction for listing session JSON.
func TestBuildListSessionJSONArgs(t *testing.T) {
	args := buildListSessionJSONArgs("alca-project-0")
	want := []string{"sync", "list", "alca-project-0", "--template={{json .}}"}

	if !slices.Equal(args, want) {
		t.Errorf("buildListSessionJSONArgs() = %v, want %v", args, want)
	}
}

// TestParseMutagenListOutput tests parsing of mutagen sync list output.
func TestParseMutagenListOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		namePrefix string
		expected   []string
	}{
		{
			name:       "empty output",
			output:     "",
			namePrefix: "alca-",
			expected:   []string{},
		},
		{
			name:       "single session",
			output:     "alca-project-workspace\n",
			namePrefix: "alca-",
			expected:   []string{"alca-project-workspace"},
		},
		{
			name:       "multiple sessions",
			output:     "alca-proj1-workspace\nalca-proj2-workspace\nother-session\n",
			namePrefix: "alca-",
			expected:   []string{"alca-proj1-workspace", "alca-proj2-workspace"},
		},
		{
			name:       "no matching sessions",
			output:     "other-session1\nother-session2\n",
			namePrefix: "alca-",
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMutagenListOutput(tt.output, tt.namePrefix)

			if !slices.Equal(result, tt.expected) {
				t.Errorf("parseMutagenListOutput() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestMutagenSyncSessionName tests generation of unique session names.
func TestMutagenSyncSessionName(t *testing.T) {
	tests := []struct {
		projectID  string
		mountIndex int
		expected   string
	}{
		{"abc123", 0, "alca-abc123-0"},
		{"def456", 1, "alca-def456-1"},
		{"xyz789", 2, "alca-xyz789-2"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := util.MutagenSessionName(tt.projectID, tt.mountIndex)
			if result != tt.expected {
				t.Errorf("MutagenSessionName(%q, %d) = %q, expected %q",
					tt.projectID, tt.mountIndex, result, tt.expected)
			}
		})
	}
}

// TestMutagenSyncTarget tests generation of Mutagen target URLs.
func TestMutagenSyncTarget(t *testing.T) {
	tests := []struct {
		containerID string
		path        string
		expected    string
	}{
		{"abc123def", "/workspace", "docker://abc123def/workspace"},
		{"container-id", "/app/data", "docker://container-id/app/data"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := MutagenTarget(tt.containerID, tt.path)
			if result != tt.expected {
				t.Errorf("MutagenTarget(%q, %q) = %q, expected %q",
					tt.containerID, tt.path, result, tt.expected)
			}
		})
	}
}

// TestMutagenSyncCreate_Success tests Create via mock command runner.
func TestMutagenSyncCreate_Success(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("mutagen sync create --name=test-session --ignore=.git/ /src docker://cid/workspace", []byte(""))
	env := newMockEnv(mock)

	sync := MutagenSync{
		Name:    "test-session",
		Source:  "/src",
		Target:  "docker://cid/workspace",
		Ignores: []string{".git/"},
	}
	err := sync.Create(env)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	mock.AssertCalled(t, "mutagen sync create --name=test-session --ignore=.git/ /src docker://cid/workspace")
}

// TestMutagenSyncCreate_Failure tests Create error wrapping.
func TestMutagenSyncCreate_Failure(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.Expect("mutagen sync create --name=test-session /src docker://cid/workspace",
		[]byte("permission denied"), errors.New("exit status 1"))
	env := newMockEnv(mock)

	sync := MutagenSync{
		Name:   "test-session",
		Source: "/src",
		Target: "docker://cid/workspace",
	}
	err := sync.Create(env)
	if err == nil {
		t.Fatal("Create() should return error")
	}
	if !strings.Contains(err.Error(), "mutagen sync create failed") {
		t.Errorf("Create() error = %q, want it to contain 'mutagen sync create failed'", err.Error())
	}
}

// TestMutagenSyncTerminate_Success tests Terminate via mock command runner.
func TestMutagenSyncTerminate_Success(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("mutagen sync terminate test-session", []byte(""))
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "test-session"}
	err := sync.Terminate(env)
	if err != nil {
		t.Fatalf("Terminate() unexpected error: %v", err)
	}
	mock.AssertCalled(t, "mutagen sync terminate test-session")
}

// TestMutagenSyncTerminate_NoMatchingSessions tests that "no matching sessions" is not an error.
func TestMutagenSyncTerminate_NoMatchingSessions(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.Expect("mutagen sync terminate test-session",
		[]byte("Error: no matching sessions"), errors.New("exit status 1"))
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "test-session"}
	err := sync.Terminate(env)
	if err != nil {
		t.Fatalf("Terminate() should not error for 'no matching sessions', got: %v", err)
	}
}

// TestMutagenSyncTerminate_RealError tests that non-"no matching sessions" errors are returned.
func TestMutagenSyncTerminate_RealError(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.Expect("mutagen sync terminate test-session",
		[]byte("daemon not running"), errors.New("exit status 1"))
	env := newMockEnv(mock)

	sync := MutagenSync{Name: "test-session"}
	err := sync.Terminate(env)
	if err == nil {
		t.Fatal("Terminate() should return error for non-matching-sessions failure")
	}
	if !strings.Contains(err.Error(), "mutagen sync terminate failed") {
		t.Errorf("Terminate() error = %q, want 'mutagen sync terminate failed'", err.Error())
	}
}

// TestListMutagenSyncs_Success tests listing sessions via command runner.
func TestListMutagenSyncs_Success(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess(`mutagen sync list --template={{range .}}{{.Name}}{{"\n"}}{{end}}`,
		[]byte("alca-proj-0\nalca-proj-1\nother-session\n"))
	env := newMockEnv(mock)

	result, err := ListMutagenSyncs(env, "alca-proj-")
	if err != nil {
		t.Fatalf("ListMutagenSyncs() unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 sessions, got %d: %v", len(result), result)
	}
}

// TestListMutagenSyncs_CommandError returns empty slice on error.
func TestListMutagenSyncs_CommandError(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure(`mutagen sync list --template={{range .}}{{.Name}}{{"\n"}}{{end}}`, errCommandNotFound)
	env := newMockEnv(mock)

	result, err := ListMutagenSyncs(env, "alca-")
	if err != nil {
		t.Fatalf("ListMutagenSyncs() should not return error, got: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result on error, got %v", result)
	}
}

// TestListSessionJSON_Success tests JSON output retrieval.
func TestListSessionJSON_Success(t *testing.T) {
	jsonOutput := []byte(`[{"conflicts":[]}]`)
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("mutagen sync list alca-proj-0 --template={{json .}}", jsonOutput)
	env := newMockEnv(mock)

	result, err := ListSessionJSON(env, "alca-proj-0")
	if err != nil {
		t.Fatalf("ListSessionJSON() unexpected error: %v", err)
	}
	if string(result) != string(jsonOutput) {
		t.Errorf("ListSessionJSON() = %q, want %q", string(result), string(jsonOutput))
	}
}

// TestListSessionJSON_Error tests error wrapping.
func TestListSessionJSON_Error(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.Expect("mutagen sync list bad-session --template={{json .}}",
		[]byte("no matching sessions"), errors.New("exit status 1"))
	env := newMockEnv(mock)

	_, err := ListSessionJSON(env, "bad-session")
	if err == nil {
		t.Fatal("ListSessionJSON() should return error")
	}
	if !strings.Contains(err.Error(), "mutagen sync list failed") {
		t.Errorf("ListSessionJSON() error = %q, want 'mutagen sync list failed'", err.Error())
	}
}

// TestTerminateProjectSyncs_AllSucceed tests successful termination of all sessions.
func TestTerminateProjectSyncs_AllSucceed(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess(`mutagen sync list --template={{range .}}{{.Name}}{{"\n"}}{{end}}`,
		[]byte("alca-proj-0\nalca-proj-1\n"))
	mock.ExpectSuccess("mutagen sync terminate alca-proj-0", []byte(""))
	mock.ExpectSuccess("mutagen sync terminate alca-proj-1", []byte(""))
	env := newMockEnv(mock)

	err := TerminateProjectSyncs(env, "proj")
	if err != nil {
		t.Fatalf("TerminateProjectSyncs() unexpected error: %v", err)
	}
	mock.AssertCalled(t, "mutagen sync terminate alca-proj-0")
	mock.AssertCalled(t, "mutagen sync terminate alca-proj-1")
}

// TestTerminateProjectSyncs_PartialFailure tests that all sessions are attempted
// and the last error is returned.
func TestTerminateProjectSyncs_PartialFailure(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess(`mutagen sync list --template={{range .}}{{.Name}}{{"\n"}}{{end}}`,
		[]byte("alca-proj-0\nalca-proj-1\n"))
	mock.Expect("mutagen sync terminate alca-proj-0",
		[]byte("daemon error"), errors.New("exit status 1"))
	mock.ExpectSuccess("mutagen sync terminate alca-proj-1", []byte(""))
	env := newMockEnv(mock)

	err := TerminateProjectSyncs(env, "proj")
	// Should return the error from proj-0 (the last error encountered)
	if err == nil {
		t.Fatal("TerminateProjectSyncs() should return error on partial failure")
	}
	// Both sessions should still be attempted
	mock.AssertCalled(t, "mutagen sync terminate alca-proj-0")
	mock.AssertCalled(t, "mutagen sync terminate alca-proj-1")
}

// TestTerminateProjectSyncs_NoSessions tests termination with no matching sessions.
func TestTerminateProjectSyncs_NoSessions(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess(`mutagen sync list --template={{range .}}{{.Name}}{{"\n"}}{{end}}`, []byte("other-session\n"))
	env := newMockEnv(mock)

	err := TerminateProjectSyncs(env, "proj")
	if err != nil {
		t.Fatalf("TerminateProjectSyncs() with no sessions should not error, got: %v", err)
	}
}

// TestParseMutagenListOutput_WhitespaceLines tests that whitespace-only lines are ignored.
func TestParseMutagenListOutput_WhitespaceLines(t *testing.T) {
	output := "  \n\talca-proj-0\n   \nalca-proj-1\n\n"
	result := parseMutagenListOutput(output, "alca-")
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(result), result)
	}
}

// Integration tests (skipped in CI without mutagen binary)

func TestMutagenSyncCreateIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test would require mutagen binary and a running container
	t.Skip("integration test requires mutagen binary and running container")

	sync := MutagenSync{
		Name:    "test-session",
		Source:  "/tmp/test-source",
		Target:  "docker://test-container/workspace",
		Ignores: []string{".git/"},
	}

	runtimeEnv := NewRuntimeEnv(util.NewCommandRunner())
	err := sync.Create(runtimeEnv)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Cleanup
	defer func() {
		_ = sync.Terminate(runtimeEnv)
	}()
}
