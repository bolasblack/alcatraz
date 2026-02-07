//go:build darwin

package pf

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/util"
)

func TestProjectFileName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/Users/alice/project",
			expected: "-Users-alice-project",
		},
		{
			name:     "nested path",
			path:     "/home/user/workspace/myapp",
			expected: "-home-user-workspace-myapp",
		},
		{
			name:     "root path",
			path:     "/",
			expected: "-",
		},
		{
			name:     "path with spaces encoded",
			path:     "/Users/alice/my project",
			expected: "-Users-alice-my project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newPfHelper().projectFileName(tt.path)
			if result != tt.expected {
				t.Errorf("projectFileName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestHasLANAccess(t *testing.T) {
	tests := []struct {
		name      string
		lanAccess []string
		expected  bool
	}{
		{
			name:      "empty slice",
			lanAccess: []string{},
			expected:  false,
		},
		{
			name:      "nil slice",
			lanAccess: nil,
			expected:  false,
		},
		{
			name:      "wildcard only",
			lanAccess: []string{"*"},
			expected:  true,
		},
		{
			name:      "wildcard with others",
			lanAccess: []string{"10.0.0.0/8", "*"},
			expected:  true,
		},
		{
			name:      "specific CIDR only",
			lanAccess: []string{"10.0.0.0/8", "192.168.0.0/16"},
			expected:  true, // any non-empty config means LAN access is configured
		},
		{
			name:      "single specific entry",
			lanAccess: []string{"192.168.1.100"},
			expected:  true, // any non-empty config means LAN access is configured
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasLANAccess(tt.lanAccess)
			if result != tt.expected {
				t.Errorf("hasLANAccess(%v) = %v, want %v", tt.lanAccess, result, tt.expected)
			}
		})
	}
}

func TestGenerateNATRules(t *testing.T) {
	tests := []struct {
		name       string
		subnet     string
		interfaces []string
		expected   string
	}{
		{
			name:       "single interface",
			subnet:     "192.168.138.0/23",
			interfaces: []string{"en0"},
			expected:   "nat on en0 from 192.168.138.0/23 to any -> (en0)\n",
		},
		{
			name:       "multiple interfaces",
			subnet:     "192.168.138.0/23",
			interfaces: []string{"en0", "en1", "en8"},
			expected: "nat on en0 from 192.168.138.0/23 to any -> (en0)\n" +
				"nat on en1 from 192.168.138.0/23 to any -> (en1)\n" +
				"nat on en8 from 192.168.138.0/23 to any -> (en8)\n",
		},
		{
			name:       "empty interfaces",
			subnet:     "192.168.138.0/23",
			interfaces: []string{},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newPfHelper().generateNATRules(tt.subnet, tt.interfaces)
			if result != tt.expected {
				t.Errorf("generateNATRules(%q, %v) = %q, want %q", tt.subnet, tt.interfaces, result, tt.expected)
			}
		})
	}
}

func TestParseLineValues(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		prefix   string
		expected []string
	}{
		{
			name:     "single value",
			output:   "Device: en0",
			prefix:   "Device:",
			expected: []string{"en0"},
		},
		{
			name:     "multiple values",
			output:   "Device: en0\nDevice: en1\nDevice: en8",
			prefix:   "Device:",
			expected: []string{"en0", "en1", "en8"},
		},
		{
			name:     "mixed lines",
			output:   "Hardware Port: Wi-Fi\nDevice: en0\nEthernet Address: aa:bb:cc:dd:ee:ff",
			prefix:   "Device:",
			expected: []string{"en0"},
		},
		{
			name:     "no matching lines",
			output:   "Hardware Port: Wi-Fi\nEthernet Address: aa:bb:cc:dd:ee:ff",
			prefix:   "Device:",
			expected: nil,
		},
		{
			name:     "empty output",
			output:   "",
			prefix:   "Device:",
			expected: nil,
		},
		{
			name:     "value with spaces",
			output:   "network.subnet4: 192.168.138.0/23",
			prefix:   "network.subnet4",
			expected: []string{"192.168.138.0/23"},
		},
		{
			name:     "empty value after colon",
			output:   "Device:",
			prefix:   "Device:",
			expected: nil,
		},
		{
			name:     "whitespace around value",
			output:   "  interface:   en0  ",
			prefix:   "interface:",
			expected: []string{"en0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLineValues(tt.output, tt.prefix)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("parseLineValues(%q, %q) = %v, want %v", tt.output, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestParseLineValue(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		prefix        string
		expectedValue string
		expectedFound bool
	}{
		{
			name:          "single value found",
			output:        "interface: en0",
			prefix:        "interface:",
			expectedValue: "en0",
			expectedFound: true,
		},
		{
			name:          "multiple values returns first",
			output:        "interface: en0\ninterface: en1",
			prefix:        "interface:",
			expectedValue: "en0",
			expectedFound: true,
		},
		{
			name:          "no match",
			output:        "gateway: 192.168.1.1",
			prefix:        "interface:",
			expectedValue: "",
			expectedFound: false,
		},
		{
			name:          "empty output",
			output:        "",
			prefix:        "interface:",
			expectedValue: "",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := parseLineValue(tt.output, tt.prefix)
			if value != tt.expectedValue || found != tt.expectedFound {
				t.Errorf("parseLineValue(%q, %q) = (%q, %v), want (%q, %v)",
					tt.output, tt.prefix, value, found, tt.expectedValue, tt.expectedFound)
			}
		})
	}
}

func TestParseRuleInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "single rule",
			content:  "nat on en0 from 192.168.138.0/23 to any -> (en0)\n",
			expected: []string{"en0"},
		},
		{
			name: "multiple rules",
			content: "nat on en0 from 192.168.138.0/23 to any -> (en0)\n" +
				"nat on en1 from 192.168.138.0/23 to any -> (en1)\n" +
				"nat on en8 from 192.168.138.0/23 to any -> (en8)\n",
			expected: []string{"en0", "en1", "en8"},
		},
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "comment lines",
			content:  "# This is a comment\nnat on en0 from 192.168.138.0/23 to any -> (en0)\n",
			expected: []string{"en0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newPfHelper().parseRuleInterfaces(tt.content)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("parseRuleInterfaces(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Command-Dependent Function Tests (using MockCommandRunner)
// =============================================================================

func TestGetOrbStackSubnet(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name        string
		output      string
		cmdErr      error
		expected    string
		expectError bool
	}{
		{
			name: "parses subnet correctly",
			output: `docker.enabled: true
k8s.enabled: false
network.subnet4: 192.168.138.0/23
`,
			expected:    "192.168.138.0/23",
			expectError: false,
		},
		{
			name:        "command fails",
			cmdErr:      fmt.Errorf("command not found"),
			expectError: true,
		},
		{
			name: "subnet not in output",
			output: `docker.enabled: true
k8s.enabled: false
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := util.NewMockCommandRunner()
			mock.Expect("orbctl config show", []byte(tt.output), tt.cmdErr)

			env := shared.NewTestNetworkEnv()
			env.Cmd = mock

			result, err := h.getOrbStackSubnet(env)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
			mock.AssertCalled(t, "orbctl config show")
		})
	}
}

func TestGetPhysicalInterfaces(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name        string
		output      string
		cmdErr      error
		expected    []string
		expectError bool
	}{
		{
			name: "parses multiple interfaces",
			output: `Hardware Port: Wi-Fi
Device: en0
Ethernet Address: aa:bb:cc:dd:ee:ff

Hardware Port: Thunderbolt Ethernet
Device: en1
Ethernet Address: 11:22:33:44:55:66
`,
			expected:    []string{"en0", "en1"},
			expectError: false,
		},
		{
			name:        "command fails",
			cmdErr:      fmt.Errorf("command not found"),
			expectError: true,
		},
		{
			name:        "no interfaces found",
			output:      "Hardware Port: Wi-Fi\n",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := util.NewMockCommandRunner()
			mock.Expect("networksetup -listallhardwareports", []byte(tt.output), tt.cmdErr)

			env := shared.NewTestNetworkEnv()
			env.Cmd = mock

			result, err := h.getPhysicalInterfaces(env)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !slices.Equal(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNeedsRuleUpdate_WithMockCommands(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name                string
		existingRules       string
		interfacesOutput    string
		expectedNeedsUpdate bool
		expectedNewIfaces   []string
	}{
		{
			name:          "new interface added",
			existingRules: "nat on en0 from 192.168.138.0/23 to any -> (en0)\n",
			interfacesOutput: `Hardware Port: Wi-Fi
Device: en0
Ethernet Address: aa:bb:cc:dd:ee:ff

Hardware Port: USB Ethernet
Device: en5
Ethernet Address: 11:22:33:44:55:66
`,
			expectedNeedsUpdate: true,
			expectedNewIfaces:   []string{"en5"},
		},
		{
			name:          "no new interfaces",
			existingRules: "nat on en0 from 192.168.138.0/23 to any -> (en0)\n",
			interfacesOutput: `Hardware Port: Wi-Fi
Device: en0
Ethernet Address: aa:bb:cc:dd:ee:ff
`,
			expectedNeedsUpdate: false,
			expectedNewIfaces:   nil,
		},
		{
			name:          "no existing file - needs update",
			existingRules: "", // empty means don't create file
			interfacesOutput: `Hardware Port: Wi-Fi
Device: en0
Ethernet Address: aa:bb:cc:dd:ee:ff
`,
			expectedNeedsUpdate: true,
			expectedNewIfaces:   []string{"en0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := shared.NewTestNetworkEnv()

			// Setup filesystem
			if tt.existingRules != "" {
				_ = env.Fs.MkdirAll(pfAnchorDir, 0755)
				_ = afero.WriteFile(env.Fs, filepath.Join(pfAnchorDir, sharedRuleFile),
					[]byte(tt.existingRules), 0644)
			}

			// Setup mock commands
			mock := util.NewMockCommandRunner()
			mock.ExpectSuccess("networksetup -listallhardwareports", []byte(tt.interfacesOutput))
			env.Cmd = mock

			needsUpdate, newIfaces, err := h.needsRuleUpdate(env)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if needsUpdate != tt.expectedNeedsUpdate {
				t.Errorf("needsUpdate = %v, want %v", needsUpdate, tt.expectedNeedsUpdate)
			}
			if !slices.Equal(newIfaces, tt.expectedNewIfaces) {
				t.Errorf("newIfaces = %v, want %v", newIfaces, tt.expectedNewIfaces)
			}
		})
	}
}

// =============================================================================
// Filesystem Function Tests (using MockFs)
// =============================================================================

// newTestEnv creates a test environment with in-memory filesystem.
func newTestEnv(t *testing.T) (*shared.NetworkEnv, afero.Fs) {
	t.Helper()
	memFs := afero.NewMemMapFs()
	env := &shared.NetworkEnv{Fs: memFs}
	return env, memFs
}

func TestFileExists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(afero.Fs) // Setup files in the base fs
		path     string
		expected bool
	}{
		{
			name: "file exists",
			setup: func(fs afero.Fs) {
				_ = afero.WriteFile(fs, "/etc/test.conf", []byte("content"), 0644)
			},
			path:     "/etc/test.conf",
			expected: true,
		},
		{
			name: "directory exists",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/etc/pf.anchors", 0755)
			},
			path:     "/etc/pf.anchors",
			expected: true,
		},
		{
			name:     "file does not exist",
			setup:    func(fs afero.Fs) {},
			path:     "/nonexistent/file",
			expected: false,
		},
		{
			name: "parent exists but file does not",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll("/etc", 0755)
			},
			path:     "/etc/missing.conf",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, memFs := newTestEnv(t)
			tt.setup(memFs)

			result := fileExists(env, tt.path)
			if result != tt.expected {
				t.Errorf("fileExists(env, %q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestReadExistingRuleInterfaces(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name               string
		setup              func(afero.Fs)
		expectedInterfaces []string
		expectedExists     bool
		expectError        bool
	}{
		{
			name: "file exists with rules",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll(pfAnchorDir, 0755)
				content := "nat on en0 from 192.168.138.0/23 to any -> (en0)\nnat on en1 from 192.168.138.0/23 to any -> (en1)\n"
				_ = afero.WriteFile(fs, filepath.Join(pfAnchorDir, sharedRuleFile), []byte(content), 0644)
			},
			expectedInterfaces: []string{"en0", "en1"},
			expectedExists:     true,
			expectError:        false,
		},
		{
			name:               "file does not exist",
			setup:              func(fs afero.Fs) {},
			expectedInterfaces: nil,
			expectedExists:     false,
			expectError:        false,
		},
		{
			name: "empty file",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll(pfAnchorDir, 0755)
				_ = afero.WriteFile(fs, filepath.Join(pfAnchorDir, sharedRuleFile), []byte(""), 0644)
			},
			expectedInterfaces: nil,
			expectedExists:     true,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, memFs := newTestEnv(t)
			tt.setup(memFs)

			interfaces, exists, err := h.readExistingRuleInterfaces(env)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if exists != tt.expectedExists {
				t.Errorf("exists = %v, want %v", exists, tt.expectedExists)
			}
			if !slices.Equal(interfaces, tt.expectedInterfaces) {
				t.Errorf("interfaces = %v, want %v", interfaces, tt.expectedInterfaces)
			}
		})
	}
}
