package preset

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"
)

const gitURLPrefix = "git+"

// IsPresetURL reports whether rawURL has the "git+" prefix expected by preset URLs.
func IsPresetURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, gitURLPrefix)
}

// PresetURL represents a parsed git preset URL.
//
// URL format: git+<clone-url>[#[commit-hash]:[dir-path]]
type PresetURL struct {
	// CloneURL is the full clone URL without "git+" prefix and with ".git" stripped.
	CloneURL string
	// Host is the hostname (and port if present) from the URL.
	Host string
	// Protocol is the original scheme including "git+" prefix (e.g. "git+https").
	Protocol string
	// Credentials is the user info from the URL (username, username:password, or empty).
	Credentials string
	// RepoPath is the path portion of the URL with ".git" stripped and leading slash removed.
	RepoPath string
	// rawRepoPath preserves the original path before ".git" stripping for cache path generation.
	rawRepoPath string
	// CommitHash is the commit hash from the fragment (may be empty).
	CommitHash string
	// DirPath is the directory path from the fragment (may be empty, defaults to repo root).
	DirPath string
	// RawURL is the original full URL as provided.
	RawURL string
}

// ParsePresetURL parses a git preset URL string into a PresetURL.
//
// The expected format is: git+<clone-url>[#[commit-hash]:[dir-path]]
func ParsePresetURL(rawURL string) (*PresetURL, error) {
	if !strings.HasPrefix(rawURL, gitURLPrefix) {
		return nil, fmt.Errorf("preset URL must start with \"git+\" prefix, got: %s: %w", rawURL, ErrInvalidPresetURL)
	}

	// Strip "git+" prefix for standard URL parsing.
	stripped := rawURL[len(gitURLPrefix):]

	parsed, err := url.Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("invalid URL (%w): %w", err, ErrInvalidPresetURL)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid URL: missing host in %s: %w", rawURL, ErrInvalidPresetURL)
	}

	// Split fragment on first ":" → commit hash + dir path.
	var commitHash, dirPath string
	if parsed.Fragment != "" {
		if idx := strings.Index(parsed.Fragment, ":"); idx >= 0 {
			commitHash = parsed.Fragment[:idx]
			dirPath = parsed.Fragment[idx+1:]
		} else {
			commitHash = parsed.Fragment
		}
	}

	// Validate fragment components.
	if commitHash != "" && !isHexString(commitHash) {
		return nil, fmt.Errorf("invalid commit hash in fragment: %q (must be hexadecimal): %w", commitHash, ErrInvalidPresetURL)
	}
	if dirPath != "" && (strings.Contains(dirPath, ":") || strings.TrimSpace(dirPath) == "") {
		return nil, fmt.Errorf("invalid directory path in fragment: %q: %w", dirPath, ErrInvalidPresetURL)
	}

	// Extract credentials from user info.
	var credentials string
	if parsed.User != nil {
		credentials = parsed.User.String()
	}

	// Preserve raw repo path (with .git suffix if present) before stripping for cache path.
	rawRepoPath := strings.TrimPrefix(parsed.Path, "/")

	// Strip ".git" suffix from path for normalization.
	repoPath := strings.TrimSuffix(parsed.Path, ".git")
	repoPath = strings.TrimPrefix(repoPath, "/")

	// Build CloneURL: scheme + "://" + [userinfo@] + host + path (without .git).
	var cloneURL strings.Builder
	cloneURL.WriteString(parsed.Scheme)
	cloneURL.WriteString("://")
	if parsed.User != nil {
		cloneURL.WriteString(parsed.User.String())
		cloneURL.WriteString("@")
	}
	cloneURL.WriteString(parsed.Host)
	cloneURL.WriteString("/")
	cloneURL.WriteString(repoPath)

	return &PresetURL{
		CloneURL:    cloneURL.String(),
		Host:        parsed.Host,
		Protocol:    gitURLPrefix + parsed.Scheme,
		Credentials: credentials,
		RepoPath:    repoPath,
		rawRepoPath: rawRepoPath,
		CommitHash:  commitHash,
		DirPath:     dirPath,
		RawURL:      rawURL,
	}, nil
}

// CachePath returns the full cache directory path for this preset URL.
//
// Format: <baseCacheDir>/<host>/<protocol>/<credentials>/<repo-path>/
func (u *PresetURL) CachePath(baseCacheDir string) string {
	// Replace invalid path chars in protocol with "-".
	protocol := sanitizePathComponent(u.Protocol)

	// Credentials: use "-" as placeholder when empty.
	creds := "-"
	if u.Credentials != "" {
		creds = sanitizePathComponent(u.Credentials)
	}

	// Use rawRepoPath: convert only dots to dashes while preserving slashes.
	repoPath := strings.ReplaceAll(u.rawRepoPath, ".", "-")

	return filepath.Join(baseCacheDir, u.Host, protocol, creds, repoPath)
}

// SourceBase returns the clone URL base preserving the original path (including .git suffix),
// without the "git+" prefix and without the fragment. This is used in source comments so that
// the update flow can derive the same cache path as the original preset flow.
func (u *PresetURL) SourceBase() string {
	base := u.RawURL[len(gitURLPrefix):]
	if idx := strings.Index(base, "#"); idx >= 0 {
		base = base[:idx]
	}
	return base
}

// HasCredentials returns true if user info (user:pass@ or token@) is present in the URL.
func (u *PresetURL) HasCredentials() bool {
	return u.Credentials != ""
}

// isHexString returns true if s consists only of hexadecimal characters.
func isHexString(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return false
		}
	}
	return true
}

// sanitizePathComponent replaces characters invalid in file paths with "-".
func sanitizePathComponent(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '+':
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
