//go:build darwin

package pf

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/afero"
)

func TestParsePfConfAndRemoveOldAnchor(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		oldAnchorLine string
		expected      []string
	}{
		{
			name: "removes old wildcard anchor",
			content: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz/*"
anchor "com.apple/*"
`,
			oldAnchorLine: `nat-anchor "alcatraz/*"`,
			expected:      []string{`scrub-anchor "com.apple/*"`, `nat-anchor "com.apple/*"`, `anchor "com.apple/*"`, ""},
		},
		{
			name: "no old anchor to remove",
			content: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`,
			oldAnchorLine: `nat-anchor "alcatraz/*"`,
			expected:      []string{`scrub-anchor "com.apple/*"`, `nat-anchor "com.apple/*"`, `anchor "com.apple/*"`, ""},
		},
		{
			name:          "empty content",
			content:       "",
			oldAnchorLine: `nat-anchor "alcatraz/*"`,
			expected:      []string{""},
		},
		{
			name: "preserves whitespace lines",
			content: `nat-anchor "com.apple/*"
   nat-anchor "alcatraz/*"
anchor "com.apple/*"
`,
			oldAnchorLine: `nat-anchor "alcatraz/*"`,
			expected:      []string{`nat-anchor "com.apple/*"`, `anchor "com.apple/*"`, ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePfConfAndRemoveOldAnchor(tt.content, tt.oldAnchorLine)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("parsePfConfAndRemoveOldAnchor() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFindLastNatAnchorIndex(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected int
	}{
		{
			name:     "single nat-anchor",
			lines:    []string{`scrub-anchor "com.apple/*"`, `nat-anchor "com.apple/*"`, `anchor "com.apple/*"`},
			expected: 1,
		},
		{
			name:     "multiple nat-anchors returns last",
			lines:    []string{`nat-anchor "first"`, `anchor "middle"`, `nat-anchor "last"`},
			expected: 2,
		},
		{
			name:     "no nat-anchor",
			lines:    []string{`scrub-anchor "com.apple/*"`, `anchor "com.apple/*"`},
			expected: -1,
		},
		{
			name:     "empty lines",
			lines:    []string{},
			expected: -1,
		},
		{
			name:     "whitespace before nat-anchor",
			lines:    []string{`  nat-anchor "test"`},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLastNatAnchorIndex(tt.lines)
			if result != tt.expected {
				t.Errorf("findLastNatAnchorIndex(%v) = %d, want %d", tt.lines, result, tt.expected)
			}
		})
	}
}

func TestInsertAfterIndex(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		idx      int
		line     string
		expected []string
	}{
		{
			name:     "insert after first",
			lines:    []string{"a", "b", "c"},
			idx:      0,
			line:     "NEW",
			expected: []string{"a", "NEW", "b", "c"},
		},
		{
			name:     "insert after middle",
			lines:    []string{"a", "b", "c"},
			idx:      1,
			line:     "NEW",
			expected: []string{"a", "b", "NEW", "c"},
		},
		{
			name:     "insert after last",
			lines:    []string{"a", "b", "c"},
			idx:      2,
			line:     "NEW",
			expected: []string{"a", "b", "c", "NEW"},
		},
		{
			name:     "single element",
			lines:    []string{"a"},
			idx:      0,
			line:     "NEW",
			expected: []string{"a", "NEW"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertAfterIndex(tt.lines, tt.idx, tt.line)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("insertAfterIndex(%v, %d, %q) = %v, want %v", tt.lines, tt.idx, tt.line, result, tt.expected)
			}
		})
	}
}

func TestInsertBeforeFirstAnchor(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		anchorLine string
		expected   []string
	}{
		{
			name:       "inserts before anchor line",
			lines:      []string{`scrub-anchor "com.apple/*"`, `anchor "com.apple/*"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`scrub-anchor "com.apple/*"`, `nat-anchor "alcatraz"`, `anchor "com.apple/*"`, ""},
		},
		{
			name:       "no anchor line - append before trailing empty",
			lines:      []string{`scrub-anchor "com.apple/*"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`scrub-anchor "com.apple/*"`, `nat-anchor "alcatraz"`, ""},
		},
		{
			name:       "no anchor line and no trailing empty",
			lines:      []string{`scrub-anchor "com.apple/*"`},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`scrub-anchor "com.apple/*"`, `nat-anchor "alcatraz"`},
		},
		{
			name:       "empty input",
			lines:      []string{},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`nat-anchor "alcatraz"`},
		},
		{
			name:       "anchor with whitespace prefix",
			lines:      []string{`scrub`, `  anchor "test"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`scrub`, `nat-anchor "alcatraz"`, `  anchor "test"`, ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertBeforeFirstAnchor(tt.lines, tt.anchorLine)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("insertBeforeFirstAnchor(%v, %q) = %v, want %v", tt.lines, tt.anchorLine, result, tt.expected)
			}
		})
	}
}

func TestInsertAnchorLine(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		anchorLine string
		expected   []string
	}{
		{
			name:       "inserts after existing nat-anchor",
			lines:      []string{`nat-anchor "com.apple/*"`, `anchor "com.apple/*"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`nat-anchor "com.apple/*"`, `nat-anchor "alcatraz"`, `anchor "com.apple/*"`, ""},
		},
		{
			name:       "inserts after last of multiple nat-anchors",
			lines:      []string{`nat-anchor "first"`, `nat-anchor "second"`, `anchor "test"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`nat-anchor "first"`, `nat-anchor "second"`, `nat-anchor "alcatraz"`, `anchor "test"`, ""},
		},
		{
			name:       "no nat-anchor - inserts before anchor",
			lines:      []string{`scrub-anchor "test"`, `anchor "com.apple/*"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`scrub-anchor "test"`, `nat-anchor "alcatraz"`, `anchor "com.apple/*"`, ""},
		},
		{
			name:       "no nat-anchor and no anchor - appends",
			lines:      []string{`scrub-anchor "test"`, ""},
			anchorLine: `nat-anchor "alcatraz"`,
			expected:   []string{`scrub-anchor "test"`, `nat-anchor "alcatraz"`, ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertAnchorLine(tt.lines, tt.anchorLine)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("insertAnchorLine(%v, %q) = %v, want %v", tt.lines, tt.anchorLine, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Filesystem Function Tests (using MockFs via TransactFs)
// =============================================================================

func TestEnsurePfAnchor(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name            string
		initialContent  string
		expectedContent string
		expectError     bool
	}{
		{
			name: "adds anchor to file with existing nat-anchor",
			initialContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`,
			expectedContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
anchor "alcatraz"
`,
			expectError: false,
		},
		{
			name: "anchor already exists - no change",
			initialContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
anchor "alcatraz"
`,
			expectedContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
anchor "alcatraz"
`,
			expectError: false,
		},
		{
			name: "old wildcard anchor preserved - ensurePfAnchor only adds",
			initialContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz/*"
anchor "com.apple/*"
`,
			expectedContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
anchor "alcatraz"
`,
			expectError: false,
		},
		{
			name: "no nat-anchor - inserts before anchor",
			initialContent: `scrub-anchor "com.apple/*"
anchor "com.apple/*"
`,
			expectedContent: `scrub-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
anchor "alcatraz"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, memFs := newTestEnv(t)

			// Setup: write initial pf.conf
			_ = afero.WriteFile(memFs, pfConfPath, []byte(tt.initialContent), 0644)

			// Execute
			err := h.ensurePfAnchor(env)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify
			content, err := afero.ReadFile(env.Fs, pfConfPath)
			if err != nil {
				t.Fatalf("failed to read pf.conf: %v", err)
			}
			if string(content) != tt.expectedContent {
				t.Errorf("pf.conf content =\n%s\nwant:\n%s", string(content), tt.expectedContent)
			}
		})
	}
}

func TestRemovePfAnchor(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name            string
		initialContent  string
		expectedContent string
		expectError     bool
	}{
		{
			name: "removes anchor from file",
			initialContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
nat-anchor "alcatraz"
anchor "com.apple/*"
`,
			expectedContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`,
			expectError: false,
		},
		{
			name: "anchor not present - no change",
			initialContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`,
			expectedContent: `scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
anchor "com.apple/*"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, memFs := newTestEnv(t)

			// Setup
			_ = afero.WriteFile(memFs, pfConfPath, []byte(tt.initialContent), 0644)

			// Execute
			err := h.removePfAnchor(env)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify
			content, err := afero.ReadFile(env.Fs, pfConfPath)
			if err != nil {
				t.Fatalf("failed to read pf.conf: %v", err)
			}
			if string(content) != tt.expectedContent {
				t.Errorf("pf.conf content =\n%s\nwant:\n%s", string(content), tt.expectedContent)
			}
		})
	}
}

func TestWriteSharedRule(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name        string
		rules       string
		expectError bool
	}{
		{
			name:        "writes rules to shared file",
			rules:       "nat on en0 from 192.168.138.0/23 to any -> (en0)\n",
			expectError: false,
		},
		{
			name:        "writes empty content",
			rules:       "",
			expectError: false,
		},
		{
			name:        "writes multiple rules",
			rules:       "nat on en0 from 192.168.138.0/23 to any -> (en0)\nnat on en1 from 192.168.138.0/23 to any -> (en1)\n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := newTestEnv(t)

			// Execute
			err := h.writeSharedRule(env, tt.rules)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify
			sharedPath := filepath.Join(pfAnchorDir, sharedRuleFile)
			content, err := afero.ReadFile(env.Fs, sharedPath)
			if err != nil {
				t.Fatalf("failed to read shared rule file: %v", err)
			}
			if string(content) != tt.rules {
				t.Errorf("shared rule content = %q, want %q", string(content), tt.rules)
			}
		})
	}
}

func TestWriteProjectFile(t *testing.T) {
	h := newPfHelper()

	tests := []struct {
		name        string
		projectDir  string
		content     string
		expectError bool
	}{
		{
			name:        "writes project file",
			projectDir:  "/Users/alice/project",
			content:     "# Project rules\n",
			expectError: false,
		},
		{
			name:        "project path with spaces",
			projectDir:  "/Users/alice/my project",
			content:     "# Rules\n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := newTestEnv(t)

			// Execute
			err := h.writeProjectFile(env, tt.projectDir, tt.content)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify
			expectedPath := filepath.Join(pfAnchorDir, h.projectFileName(tt.projectDir))
			content, err := afero.ReadFile(env.Fs, expectedPath)
			if err != nil {
				t.Fatalf("failed to read project file: %v", err)
			}
			if string(content) != tt.content {
				t.Errorf("project file content = %q, want %q", string(content), tt.content)
			}
		})
	}
}

func TestFindLastAnchorIndex(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected int
	}{
		{
			name:     "single anchor",
			lines:    []string{`scrub-anchor "test"`, `anchor "com.apple/*"`, `nat-anchor "test"`},
			expected: 1,
		},
		{
			name:     "multiple anchors returns last",
			lines:    []string{`anchor "first"`, `nat-anchor "middle"`, `anchor "last"`},
			expected: 2,
		},
		{
			name:     "no anchor lines",
			lines:    []string{`scrub-anchor "com.apple/*"`, `nat-anchor "test"`},
			expected: -1,
		},
		{
			name:     "empty lines",
			lines:    []string{},
			expected: -1,
		},
		{
			name:     "whitespace before anchor",
			lines:    []string{`  anchor "test"`},
			expected: 0,
		},
		{
			name:     "anchor without space is not matched",
			lines:    []string{`anchor"nospace"`},
			expected: -1,
		},
		{
			name:     "nat-anchor is not matched",
			lines:    []string{`nat-anchor "test"`, `scrub-anchor "test"`},
			expected: -1,
		},
		{
			name:     "rdr-anchor is not matched",
			lines:    []string{`rdr-anchor "test"`},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLastAnchorIndex(tt.lines)
			if result != tt.expected {
				t.Errorf("findLastAnchorIndex(%v) = %d, want %d", tt.lines, result, tt.expected)
			}
		})
	}
}

func TestFindNewInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		current  []string
		existing []string
		expected []string
	}{
		{
			name:     "no new interfaces",
			current:  []string{"en0", "en1"},
			existing: []string{"en0", "en1"},
			expected: nil,
		},
		{
			name:     "one new interface",
			current:  []string{"en0", "en1", "en5"},
			existing: []string{"en0", "en1"},
			expected: []string{"en5"},
		},
		{
			name:     "multiple new interfaces",
			current:  []string{"en0", "en1", "en5", "en8"},
			existing: []string{"en0"},
			expected: []string{"en1", "en5", "en8"},
		},
		{
			name:     "all new interfaces",
			current:  []string{"en0", "en1"},
			existing: []string{},
			expected: []string{"en0", "en1"},
		},
		{
			name:     "empty current returns nil",
			current:  []string{},
			existing: []string{"en0", "en1"},
			expected: nil,
		},
		{
			name:     "nil existing",
			current:  []string{"en0"},
			existing: nil,
			expected: []string{"en0"},
		},
		{
			name:     "order preserved",
			current:  []string{"en8", "en0", "en5"},
			existing: []string{"en0"},
			expected: []string{"en8", "en5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findNewInterfaces(tt.current, tt.existing)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("findNewInterfaces(%v, %v) = %v, want %v", tt.current, tt.existing, result, tt.expected)
			}
		})
	}
}

func TestInsertFilterAnchorLine(t *testing.T) {
	tests := []struct {
		name             string
		lines            []string
		filterAnchorLine string
		expected         []string
	}{
		{
			name:             "inserts after last anchor",
			lines:            []string{`nat-anchor "test"`, `anchor "com.apple/*"`, ``},
			filterAnchorLine: `anchor "alcatraz"`,
			expected:         []string{`nat-anchor "test"`, `anchor "com.apple/*"`, `anchor "alcatraz"`, ``},
		},
		{
			name:             "no anchor - inserts after last nat-anchor",
			lines:            []string{`nat-anchor "first"`, `nat-anchor "second"`, ``},
			filterAnchorLine: `anchor "alcatraz"`,
			expected:         []string{`nat-anchor "first"`, `nat-anchor "second"`, `anchor "alcatraz"`, ``},
		},
		{
			name:             "no anchors at all - appends before trailing empty",
			lines:            []string{`scrub-anchor "test"`, ``},
			filterAnchorLine: `anchor "alcatraz"`,
			expected:         []string{`scrub-anchor "test"`, `anchor "alcatraz"`, ``},
		},
		{
			name:             "no trailing empty - appends at end",
			lines:            []string{`scrub-anchor "test"`},
			filterAnchorLine: `anchor "alcatraz"`,
			expected:         []string{`scrub-anchor "test"`, `anchor "alcatraz"`},
		},
		{
			name:             "empty input",
			lines:            []string{},
			filterAnchorLine: `anchor "alcatraz"`,
			expected:         []string{`anchor "alcatraz"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertFilterAnchorLine(tt.lines, tt.filterAnchorLine)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("insertFilterAnchorLine(%v, %q) = %v, want %v", tt.lines, tt.filterAnchorLine, result, tt.expected)
			}
		})
	}
}
