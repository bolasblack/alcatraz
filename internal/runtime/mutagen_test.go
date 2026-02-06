package runtime

import (
	"errors"
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

// TestMutagenSyncCreate tests command construction for creating Mutagen sync sessions.
func TestMutagenSyncBuildCreateArgs(t *testing.T) {
	tests := []struct {
		name      string
		sync      MutagenSync
		wantParts []string
	}{
		{
			name: "basic sync without ignores",
			sync: MutagenSync{
				Name:   "alca-project-workspace",
				Source: "/Users/me/project",
				Target: "docker://container-id/workspace",
			},
			wantParts: []string{
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
			wantParts: []string{
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
			argsStr := strings.Join(args, " ")

			for _, want := range tt.wantParts {
				if !strings.Contains(argsStr, want) {
					t.Errorf("buildCreateArgs() missing %q in args: %v", want, args)
				}
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
	expected := []string{"sync", "terminate", "alca-project-workspace"}

	if len(args) != len(expected) {
		t.Fatalf("buildTerminateArgs() returned %v, expected %v", args, expected)
	}

	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("buildTerminateArgs()[%d] = %q, expected %q", i, arg, expected[i])
		}
	}
}

// TestListMutagenSyncsBuildArgs tests command construction for listing Mutagen sync sessions.
func TestListMutagenSyncsBuildArgs(t *testing.T) {
	args := buildListSyncsArgs()
	expected := []string{"sync", "list", "--template={{.Name}}"}

	if len(args) != len(expected) {
		t.Fatalf("buildListSyncsArgs() returned %v, expected %v", args, expected)
	}

	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("buildListSyncsArgs()[%d] = %q, expected %q", i, arg, expected[i])
		}
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

			if len(result) != len(tt.expected) {
				t.Fatalf("parseMutagenListOutput() returned %v, expected %v", result, tt.expected)
			}

			for i, name := range result {
				if name != tt.expected[i] {
					t.Errorf("parseMutagenListOutput()[%d] = %q, expected %q", i, name, tt.expected[i])
				}
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
			result := MutagenSessionName(tt.projectID, tt.mountIndex)
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
