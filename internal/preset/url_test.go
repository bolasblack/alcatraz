package preset

import (
	"errors"
	"testing"
)

func TestParsePresetURL_FragmentVariations(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		commitHash string
		dirPath    string
	}{
		{
			name:       "both commit and dir",
			rawURL:     "git+https://github.com/myorg/presets.git#a1b2c3d:backend",
			commitHash: "a1b2c3d",
			dirPath:    "backend",
		},
		{
			name:       "commit only",
			rawURL:     "git+https://github.com/myorg/presets.git#a1b2c3d",
			commitHash: "a1b2c3d",
			dirPath:    "",
		},
		{
			name:       "dir only",
			rawURL:     "git+https://github.com/myorg/presets.git#:backend/folder",
			commitHash: "",
			dirPath:    "backend/folder",
		},
		{
			name:       "no fragment",
			rawURL:     "git+https://github.com/myorg/presets.git",
			commitHash: "",
			dirPath:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.CommitHash != tt.commitHash {
				t.Errorf("CommitHash = %q, want %q", u.CommitHash, tt.commitHash)
			}
			if u.DirPath != tt.dirPath {
				t.Errorf("DirPath = %q, want %q", u.DirPath, tt.dirPath)
			}
		})
	}
}

func TestParsePresetURL_MalformedFragments(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "double colon", rawURL: "git+https://host/repo#::", wantErr: true},
		{name: "colon then space", rawURL: "git+https://host/repo#: ", wantErr: true},
		{name: "non-hex commit hash", rawURL: "git+https://host/repo#xyz:dir", wantErr: true},
		{name: "commit with space", rawURL: "git+https://host/repo#abc 123:dir", wantErr: true},
		{name: "empty fragment", rawURL: "git+https://host/repo#", wantErr: false},
		{name: "valid commit only", rawURL: "git+https://host/repo#a1b2c3d", wantErr: false},
		{name: "valid dir only", rawURL: "git+https://host/repo#:backend", wantErr: false},
		{name: "valid both", rawURL: "git+https://host/repo#a1b2c3d:backend/sub", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePresetURL(tt.rawURL)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidPresetURL) {
					t.Fatalf("ParsePresetURL(%q) error = %v, want ErrInvalidPresetURL", tt.rawURL, err)
				}
			} else if err != nil {
				t.Fatalf("ParsePresetURL(%q) unexpected error = %v", tt.rawURL, err)
			}
		})
	}
}

func TestParsePresetURL_CachePathExamples(t *testing.T) {
	base := "/home/user/.alcatraz/cache-presets"

	tests := []struct {
		name     string
		rawURL   string
		wantPath string
	}{
		{
			name:     "https with .git suffix",
			rawURL:   "git+https://github.com/myorg/presets.git",
			wantPath: base + "/github.com/git-https/-/myorg/presets-git",
		},
		{
			name:     "https without .git suffix",
			rawURL:   "git+https://github.com/myorg/presets",
			wantPath: base + "/github.com/git-https/-/myorg/presets",
		},
		{
			name:     "ssh with user",
			rawURL:   "git+ssh://git@github.com/myorg/presets",
			wantPath: base + "/github.com/git-ssh/git/myorg/presets",
		},
		{
			name:     "https with token",
			rawURL:   "git+https://token@gitea.company.com/team/presets",
			wantPath: base + "/gitea.company.com/git-https/token/team/presets",
		},
		{
			name:     "https with user:pass",
			rawURL:   "git+https://user:pass@gitea.company.com/team/presets",
			wantPath: base + "/gitea.company.com/git-https/user-pass/team/presets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := u.CachePath(base)
			if got != tt.wantPath {
				t.Errorf("CachePath = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestParsePresetURL_GitSuffixStripping(t *testing.T) {
	withGit, err := ParsePresetURL("git+https://github.com/myorg/presets.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	withoutGit, err := ParsePresetURL("git+https://github.com/myorg/presets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if withGit.RepoPath != withoutGit.RepoPath {
		t.Errorf("RepoPath mismatch: %q vs %q", withGit.RepoPath, withoutGit.RepoPath)
	}
	if withGit.CloneURL != withoutGit.CloneURL {
		t.Errorf("CloneURL mismatch: %q vs %q", withGit.CloneURL, withoutGit.CloneURL)
	}
	if withGit.RepoPath != "myorg/presets" {
		t.Errorf("RepoPath = %q, want %q", withGit.RepoPath, "myorg/presets")
	}
}

func TestParsePresetURL_CredentialDetection(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		hasCred bool
		creds   string
	}{
		{
			name:    "no credentials",
			rawURL:  "git+https://github.com/myorg/presets",
			hasCred: false,
			creds:   "",
		},
		{
			name:    "token only",
			rawURL:  "git+https://token@gitea.company.com/team/presets",
			hasCred: true,
			creds:   "token",
		},
		{
			name:    "user:pass",
			rawURL:  "git+https://user:pass@gitea.company.com/team/presets",
			hasCred: true,
			creds:   "user:pass",
		},
		{
			name:    "ssh user",
			rawURL:  "git+ssh://git@github.com/myorg/presets",
			hasCred: true,
			creds:   "git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.HasCredentials() != tt.hasCred {
				t.Errorf("HasCredentials() = %v, want %v", u.HasCredentials(), tt.hasCred)
			}
			if u.Credentials != tt.creds {
				t.Errorf("Credentials = %q, want %q", u.Credentials, tt.creds)
			}
		})
	}
}

func TestParsePresetURL_ErrorCases(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{
			name:   "missing git+ prefix",
			rawURL: "https://github.com/myorg/presets",
		},
		{
			name:   "empty string",
			rawURL: "",
		},
		{
			name:   "git+ with no URL",
			rawURL: "git+",
		},
		{
			name:   "git+ with missing host",
			rawURL: "git+https:///path/only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePresetURL(tt.rawURL)
			if !errors.Is(err, ErrInvalidPresetURL) {
				t.Fatalf("ParsePresetURL(%q) error = %v, want ErrInvalidPresetURL", tt.rawURL, err)
			}
		})
	}
}

func TestParsePresetURL_NestedGitLabGroups(t *testing.T) {
	u, err := ParsePresetURL("git+https://gitlab.com/group/subgroup/configs#:alcatraz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.RepoPath != "group/subgroup/configs" {
		t.Errorf("RepoPath = %q, want %q", u.RepoPath, "group/subgroup/configs")
	}
	if u.DirPath != "alcatraz" {
		t.Errorf("DirPath = %q, want %q", u.DirPath, "alcatraz")
	}
	if u.Host != "gitlab.com" {
		t.Errorf("Host = %q, want %q", u.Host, "gitlab.com")
	}
}

func TestParsePresetURL_PortInURL(t *testing.T) {
	u, err := ParsePresetURL("git+https://git.example.com:8443/team/presets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.Host != "git.example.com:8443" {
		t.Errorf("Host = %q, want %q", u.Host, "git.example.com:8443")
	}
	if u.RepoPath != "team/presets" {
		t.Errorf("RepoPath = %q, want %q", u.RepoPath, "team/presets")
	}
}

func TestParsePresetURL_CloneURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantURL string
	}{
		{
			name:    "https strips .git",
			rawURL:  "git+https://github.com/myorg/presets.git",
			wantURL: "https://github.com/myorg/presets",
		},
		{
			name:    "ssh with user",
			rawURL:  "git+ssh://git@github.com/myorg/presets.git",
			wantURL: "ssh://git@github.com/myorg/presets",
		},
		{
			name:    "fragment not included in CloneURL",
			rawURL:  "git+https://github.com/myorg/presets.git#abc123:dir",
			wantURL: "https://github.com/myorg/presets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.CloneURL != tt.wantURL {
				t.Errorf("CloneURL = %q, want %q", u.CloneURL, tt.wantURL)
			}
		})
	}
}

func TestParsePresetURL_Protocol(t *testing.T) {
	tests := []struct {
		name         string
		rawURL       string
		wantProtocol string
	}{
		{
			name:         "https",
			rawURL:       "git+https://github.com/myorg/presets",
			wantProtocol: "git+https",
		},
		{
			name:         "ssh",
			rawURL:       "git+ssh://git@github.com/myorg/presets",
			wantProtocol: "git+ssh",
		},
		{
			name:         "http",
			rawURL:       "git+http://github.com/myorg/presets",
			wantProtocol: "git+http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.Protocol != tt.wantProtocol {
				t.Errorf("Protocol = %q, want %q", u.Protocol, tt.wantProtocol)
			}
		})
	}
}

func TestPresetURL_SourceBase(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		wantBase string
	}{
		{
			name:     "https with .git and fragment",
			rawURL:   "git+https://github.com/myorg/presets.git#abc123:dir",
			wantBase: "https://github.com/myorg/presets.git",
		},
		{
			name:     "https without .git and fragment",
			rawURL:   "git+https://github.com/myorg/presets#abc123:dir",
			wantBase: "https://github.com/myorg/presets",
		},
		{
			name:     "no fragment",
			rawURL:   "git+https://github.com/myorg/presets.git",
			wantBase: "https://github.com/myorg/presets.git",
		},
		{
			name:     "ssh with credentials",
			rawURL:   "git+ssh://git@github.com/myorg/presets.git#abc:file",
			wantBase: "ssh://git@github.com/myorg/presets.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParsePresetURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := u.SourceBase(); got != tt.wantBase {
				t.Errorf("SourceBase() = %q, want %q", got, tt.wantBase)
			}
		})
	}
}

func TestParsePresetURL_RawURLPreserved(t *testing.T) {
	raw := "git+https://user:pass@gitea.company.com/team/presets.git#abc:dir"
	u, err := ParsePresetURL(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.RawURL != raw {
		t.Errorf("RawURL = %q, want %q", u.RawURL, raw)
	}
}
