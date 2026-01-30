package network

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// Command-Dependent Function Tests (using MockCommandRunner)
// =============================================================================

func TestIsLaunchDaemonLoaded(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name     string
		cmdErr   error
		expected bool
	}{
		{
			name:     "daemon is loaded",
			cmdErr:   nil,
			expected: true,
		},
		{
			name:     "daemon not loaded",
			cmdErr:   fmt.Errorf("Could not find service"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := util.NewMockCommandRunner()
			mock.Expect("launchctl print system/"+launchDaemonLabel, nil, tt.cmdErr)

			env := util.NewTestEnv()
			env.Cmd = mock

			result := h.isLaunchDaemonLoaded(env)

			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
			mock.AssertCalled(t, "launchctl print system/"+launchDaemonLabel)
		})
	}
}

func TestIsHelperInstalled(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name            string
		daemonLoaded    bool
		anchorDirExists bool
		expected        bool
	}{
		{
			name:            "both daemon loaded and anchor dir exist",
			daemonLoaded:    true,
			anchorDirExists: true,
			expected:        true,
		},
		{
			name:            "daemon not loaded",
			daemonLoaded:    false,
			anchorDirExists: true,
			expected:        false,
		},
		{
			name:            "anchor dir missing",
			daemonLoaded:    true,
			anchorDirExists: false,
			expected:        false,
		},
		{
			name:            "both missing",
			daemonLoaded:    false,
			anchorDirExists: false,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := util.NewMockCommandRunner()
			if tt.daemonLoaded {
				mock.ExpectSuccess("launchctl print system/"+launchDaemonLabel, nil)
			} else {
				mock.ExpectFailure("launchctl print system/"+launchDaemonLabel, fmt.Errorf("not found"))
			}

			env := util.NewTestEnv()
			env.Cmd = mock

			if tt.anchorDirExists {
				_ = env.Fs.MkdirAll(pfAnchorDir, 0755)
			}

			result := h.isHelperInstalled(env)
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFlushPfRules(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name        string
		cmdErr      error
		expectError bool
	}{
		{
			name:        "flush succeeds",
			cmdErr:      nil,
			expectError: false,
		},
		{
			name:        "flush fails",
			cmdErr:      fmt.Errorf("pfctl failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := util.NewMockCommandRunner()
			mock.Expect("sudo pfctl -a "+pfAnchorName+" -F all", nil, tt.cmdErr)

			env := util.NewTestEnv()
			env.Cmd = mock

			err := h.flushPfRules(env, nil)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			mock.AssertCalled(t, "sudo pfctl -a "+pfAnchorName+" -F all")
		})
	}
}

// =============================================================================
// Filesystem Function Tests (using MockFs)
// =============================================================================

func TestIsHelperNeedsUpdate(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name     string
		setup    func(afero.Fs)
		expected bool
	}{
		{
			name: "plist content differs",
			setup: func(fs afero.Fs) {
				// Write plist with different content
				_ = afero.WriteFile(fs, launchDaemonPath, []byte("old plist content"), 0644)
				// Write valid pf.conf with new anchor
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, pfConfPath, []byte(content), 0644)
			},
			expected: true,
		},
		{
			name: "pf.conf has old wildcard anchor",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, launchDaemonPath, []byte(launchDaemonPlist), 0644)
				// Write pf.conf with old wildcard format
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz/*"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, pfConfPath, []byte(content), 0644)
			},
			expected: true,
		},
		{
			name: "pf.conf missing alcatraz anchor",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, launchDaemonPath, []byte(launchDaemonPlist), 0644)
				// Write pf.conf without alcatraz anchor
				content := `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, pfConfPath, []byte(content), 0644)
			},
			expected: true,
		},
		{
			name: "everything up to date",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, launchDaemonPath, []byte(launchDaemonPlist), 0644)
				// Write pf.conf with new anchor format
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, pfConfPath, []byte(content), 0644)
			},
			expected: false,
		},
		{
			name: "plist missing - not considered needing update",
			setup: func(fs afero.Fs) {
				// No plist file (helper not installed)
				// Write valid pf.conf
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, pfConfPath, []byte(content), 0644)
			},
			expected: false, // Use isHelperInstalled to check if installed
		},
		{
			name: "pf.conf missing - not considered needing update",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, launchDaemonPath, []byte(launchDaemonPlist), 0644)
				// No pf.conf
			},
			expected: false, // pf.conf read error doesn't trigger update
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			env := &util.Env{Fs: memFs}
			tt.setup(memFs)

			result := h.isHelperNeedsUpdate(env)
			if result != tt.expected {
				t.Errorf("isHelperNeedsUpdate() = %v, want %v", result, tt.expected)
			}
		})
	}
}
