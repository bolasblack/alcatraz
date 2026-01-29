package network

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

func TestIsHelperNeedsUpdate(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(afero.Fs)
		expected bool
	}{
		{
			name: "plist content differs",
			setup: func(fs afero.Fs) {
				// Write plist with different content
				_ = afero.WriteFile(fs, LaunchDaemonPath, []byte("old plist content"), 0644)
				// Write valid pf.conf with new anchor
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, PfConfPath, []byte(content), 0644)
			},
			expected: true,
		},
		{
			name: "pf.conf has old wildcard anchor",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, LaunchDaemonPath, []byte(launchDaemonPlist), 0644)
				// Write pf.conf with old wildcard format
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz/*"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, PfConfPath, []byte(content), 0644)
			},
			expected: true,
		},
		{
			name: "pf.conf missing alcatraz anchor",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, LaunchDaemonPath, []byte(launchDaemonPlist), 0644)
				// Write pf.conf without alcatraz anchor
				content := `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, PfConfPath, []byte(content), 0644)
			},
			expected: true,
		},
		{
			name: "everything up to date",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, LaunchDaemonPath, []byte(launchDaemonPlist), 0644)
				// Write pf.conf with new anchor format
				content := `scrub-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
`
				_ = afero.WriteFile(fs, PfConfPath, []byte(content), 0644)
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
				_ = afero.WriteFile(fs, PfConfPath, []byte(content), 0644)
			},
			expected: false, // Use IsHelperInstalled to check if installed
		},
		{
			name: "pf.conf missing - not considered needing update",
			setup: func(fs afero.Fs) {
				// Write correct plist
				_ = afero.WriteFile(fs, LaunchDaemonPath, []byte(launchDaemonPlist), 0644)
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

			result := IsHelperNeedsUpdate(env)
			if result != tt.expected {
				t.Errorf("IsHelperNeedsUpdate() = %v, want %v", result, tt.expected)
			}
		})
	}
}
