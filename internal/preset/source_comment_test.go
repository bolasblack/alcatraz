package preset

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/afero"
)

func TestFormatSourceComment(t *testing.T) {
	got := FormatSourceComment("https://github.com/myorg/presets.git", "a1b2c3d", ".alca.node.toml")
	want := "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#a1b2c3d:.alca.node.toml\n"
	if got != want {
		t.Errorf("FormatSourceComment:\n got: %q\nwant: %q", got, want)
	}
}

func TestParseSourceComment_Valid(t *testing.T) {
	content := []byte("# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#a1b2c3d:.alca.node.toml\nimage = \"node:20\"\n")

	info, err := ParseSourceComment(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected SourceInfo, got nil")
	}
	if info.CloneURL != "https://github.com/myorg/presets.git" {
		t.Errorf("CloneURL = %q, want %q", info.CloneURL, "https://github.com/myorg/presets.git")
	}
	if info.CommitHash != "a1b2c3d" {
		t.Errorf("CommitHash = %q, want %q", info.CommitHash, "a1b2c3d")
	}
	if info.FilePath != ".alca.node.toml" {
		t.Errorf("FilePath = %q, want %q", info.FilePath, ".alca.node.toml")
	}
	if info.RawURL != "git+https://github.com/myorg/presets.git#a1b2c3d:.alca.node.toml" {
		t.Errorf("RawURL = %q", info.RawURL)
	}
}

func TestParseSourceComment_NoSourceComment(t *testing.T) {
	content := []byte("# Just a regular comment\nimage = \"node:20\"\n")

	info, err := ParseSourceComment(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestParseSourceComment_InLeadingCommentBlock(t *testing.T) {
	content := []byte("# This is a regular comment\n# Another comment\n# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#abc123:.alca.node.toml\nimage = \"node:20\"\n")

	info, err := ParseSourceComment(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected SourceInfo, got nil")
	}
	if info.CommitHash != "abc123" {
		t.Errorf("CommitHash = %q, want %q", info.CommitHash, "abc123")
	}
}

func TestParseSourceComment_IgnoredAfterNonCommentLine(t *testing.T) {
	content := []byte("image = \"node:20\"\n# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#abc123:.alca.node.toml\nworkdir = \"/app\"\n")

	info, err := ParseSourceComment(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil (source comment after non-comment line), got %+v", info)
	}
}

func TestParseSourceComment_Malformed(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "missing git+ prefix",
			content: "# Alcatraz Preset Source: https://github.com/myorg/presets.git#abc:file\n",
		},
		{
			name:    "missing fragment",
			content: "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git\n",
		},
		{
			name:    "missing filepath in fragment",
			content: "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#abc123\n",
		},
		{
			name:    "empty commit hash",
			content: "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#:.alca.node.toml\n",
		},
		{
			name:    "empty filepath",
			content: "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#abc123:\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSourceComment([]byte(tt.content))
			if !errors.Is(err, ErrInvalidSourceComment) {
				t.Fatalf("expected ErrInvalidSourceComment, got %v", err)
			}
		})
	}
}

func TestParseSourceComment_EmptyContent(t *testing.T) {
	info, err := ParseSourceComment([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestWriteSourceComment(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := []byte("image = \"node:20\"\n")

	err := WriteSourceComment(fs, "/test/.alca.node.toml", content, "https://github.com/myorg/presets.git", "a1b2c3d", ".alca.node.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := afero.ReadFile(fs, "/test/.alca.node.toml")
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	want := "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#a1b2c3d:.alca.node.toml\nimage = \"node:20\"\n"
	if string(got) != want {
		t.Errorf("file content:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestUpdateSourceComment(t *testing.T) {
	fs := afero.NewMemMapFs()
	original := "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#oldcommit:.alca.node.toml\nimage = \"node:20\"\n"
	if err := afero.WriteFile(fs, "/test/.alca.node.toml", []byte(original), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := UpdateSourceComment(fs, "/test/.alca.node.toml", "newcommit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := afero.ReadFile(fs, "/test/.alca.node.toml")
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	want := "# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#newcommit:.alca.node.toml\nimage = \"node:20\"\n"
	if string(got) != want {
		t.Errorf("file content:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestUpdateSourceComment_NoSourceComment(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "image = \"node:20\"\n"
	if err := afero.WriteFile(fs, "/test/.alca.node.toml", []byte(content), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := UpdateSourceComment(fs, "/test/.alca.node.toml", "newcommit")
	if !errors.Is(err, ErrNoSourceComment) {
		t.Fatalf("expected ErrNoSourceComment, got %v", err)
	}
}

func TestRoundTrip_WriteThenParse(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := []byte("image = \"node:20\"\nworkdir = \"/app\"\n")
	cloneURL := "https://github.com/myorg/presets.git"
	commitHash := "abc123def456"
	repoFilePath := "backend/.alca.node.toml"

	err := WriteSourceComment(fs, "/project/.alca.node.toml", content, cloneURL, commitHash, repoFilePath)
	if err != nil {
		t.Fatalf("WriteSourceComment: %v", err)
	}

	got, err := afero.ReadFile(fs, "/project/.alca.node.toml")
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	info, err := ParseSourceComment(got)
	if err != nil {
		t.Fatalf("ParseSourceComment: %v", err)
	}
	if info == nil {
		t.Fatal("expected SourceInfo, got nil")
	}
	if info.CloneURL != cloneURL {
		t.Errorf("CloneURL = %q, want %q", info.CloneURL, cloneURL)
	}
	if info.CommitHash != commitHash {
		t.Errorf("CommitHash = %q, want %q", info.CommitHash, commitHash)
	}
	if info.FilePath != repoFilePath {
		t.Errorf("FilePath = %q, want %q", info.FilePath, repoFilePath)
	}
}

func TestStripSourceComment(t *testing.T) {
	content := []byte("# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#a1b2c3d:.alca.node.toml\nimage = \"node:20\"\n")

	got := StripSourceComment(content)
	want := []byte("image = \"node:20\"\n")
	if !bytes.Equal(got, want) {
		t.Errorf("StripSourceComment:\n got: %q\nwant: %q", string(got), string(want))
	}
}

func TestStripSourceComment_WithOtherComments(t *testing.T) {
	content := []byte("# Regular comment\n# Alcatraz Preset Source: git+https://github.com/myorg/presets.git#a1b2c3d:.alca.node.toml\n# Another comment\nimage = \"node:20\"\n")

	got := StripSourceComment(content)
	want := []byte("# Regular comment\n# Another comment\nimage = \"node:20\"\n")
	if !bytes.Equal(got, want) {
		t.Errorf("StripSourceComment:\n got: %q\nwant: %q", string(got), string(want))
	}
}

func TestStripSourceComment_NoSourceComment(t *testing.T) {
	content := []byte("# Regular comment\nimage = \"node:20\"\n")

	got := StripSourceComment(content)
	if !bytes.Equal(got, content) {
		t.Errorf("StripSourceComment should return content unchanged:\n got: %q\nwant: %q", string(got), string(content))
	}
}

func TestParseSourceComment_WithSSHURL(t *testing.T) {
	content := []byte("# Alcatraz Preset Source: git+ssh://git@github.com/myorg/private-presets.git#abc123:backend/.alca.node.toml\n")

	info, err := ParseSourceComment(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected SourceInfo, got nil")
	}
	if info.CloneURL != "ssh://git@github.com/myorg/private-presets.git" {
		t.Errorf("CloneURL = %q, want %q", info.CloneURL, "ssh://git@github.com/myorg/private-presets.git")
	}
	if info.CommitHash != "abc123" {
		t.Errorf("CommitHash = %q, want %q", info.CommitHash, "abc123")
	}
	if info.FilePath != "backend/.alca.node.toml" {
		t.Errorf("FilePath = %q, want %q", info.FilePath, "backend/.alca.node.toml")
	}
}

func TestParseSourceComment_WithCredentialsInURL(t *testing.T) {
	content := []byte("# Alcatraz Preset Source: git+https://token@gitea.company.com/team/presets#abc123:.alca.node.toml\n")

	info, err := ParseSourceComment(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected SourceInfo, got nil")
	}
	if info.CloneURL != "https://token@gitea.company.com/team/presets" {
		t.Errorf("CloneURL = %q, want %q", info.CloneURL, "https://token@gitea.company.com/team/presets")
	}
}

func TestRealWorldTomlContent(t *testing.T) {
	toml := `# Alcatraz Preset Source: git+https://github.com/myorg/alca-presets.git#e004664f:backend/.alca.node.toml
# Node.js development preset

image = "node:20-bookworm"
workdir = "/workspace"

[mounts]
node_modules = { type = "volume", dst = "/workspace/node_modules" }

[env]
NODE_ENV = "development"
NPM_CONFIG_LOGLEVEL = "warn"

[ports]
dev = "3000:3000"
debug = "9229:9229"
`

	info, err := ParseSourceComment([]byte(toml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected SourceInfo, got nil")
	}
	if info.CloneURL != "https://github.com/myorg/alca-presets.git" {
		t.Errorf("CloneURL = %q", info.CloneURL)
	}
	if info.CommitHash != "e004664f" {
		t.Errorf("CommitHash = %q", info.CommitHash)
	}
	if info.FilePath != "backend/.alca.node.toml" {
		t.Errorf("FilePath = %q", info.FilePath)
	}

	stripped := StripSourceComment([]byte(toml))
	reparsed, err := ParseSourceComment(stripped)
	if err != nil {
		t.Fatalf("unexpected error after strip: %v", err)
	}
	if reparsed != nil {
		t.Error("expected nil after stripping source comment")
	}
}
