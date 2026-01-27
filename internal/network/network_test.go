package network

import (
	"slices"
	"testing"
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
			result := ProjectFileName(tt.path)
			if result != tt.expected {
				t.Errorf("ProjectFileName(%q) = %q, want %q", tt.path, result, tt.expected)
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
			expected:  false,
		},
		{
			name:      "single specific entry",
			lanAccess: []string{"192.168.1.100"},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasLANAccess(tt.lanAccess)
			if result != tt.expected {
				t.Errorf("HasLANAccess(%v) = %v, want %v", tt.lanAccess, result, tt.expected)
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
			result := GenerateNATRules(tt.subnet, tt.interfaces)
			if result != tt.expected {
				t.Errorf("GenerateNATRules(%q, %v) = %q, want %q", tt.subnet, tt.interfaces, result, tt.expected)
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
			result := ParseRuleInterfaces(tt.content)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("ParseRuleInterfaces(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}
