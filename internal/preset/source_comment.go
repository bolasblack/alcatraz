package preset

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/afero"
)

const sourceCommentPrefix = "# Alcatraz Preset Source: "

// SourceInfo holds parsed information from a source comment in a preset file.
type SourceInfo struct {
	RawURL     string // the full git+... URL as stored in the comment (with #commit:filepath)
	CloneURL   string // the clone URL portion (without git+ prefix, without fragment)
	CommitHash string // the commit hash
	FilePath   string // the in-repo filepath
}

// FormatSourceComment builds the source comment line.
// cloneURL is WITHOUT the git+ prefix (caller provides the clean clone URL).
func FormatSourceComment(cloneURL, commitHash, filePath string) string {
	return fmt.Sprintf("%sgit+%s#%s:%s\n", sourceCommentPrefix, cloneURL, commitHash, filePath)
}

// ParseSourceComment parses a source comment from file content.
// It scans consecutive # lines from the top of the file looking for the source comment.
// Returns nil, nil if no source comment is found (not an error).
// Returns an error only for malformed source comments.
func ParseSourceComment(content []byte) (*SourceInfo, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {
			break
		}
		if !strings.HasPrefix(line, sourceCommentPrefix) {
			continue
		}
		rawURL := strings.TrimPrefix(line, sourceCommentPrefix)
		return parseRawURL(rawURL)
	}
	return nil, nil
}

func parseRawURL(rawURL string) (*SourceInfo, error) {
	if !strings.HasPrefix(rawURL, gitURLPrefix) {
		return nil, fmt.Errorf("source comment URL must start with git+: %q: %w", rawURL, ErrInvalidSourceComment)
	}

	withoutPrefix := rawURL[len(gitURLPrefix):]

	hashIdx := strings.LastIndex(withoutPrefix, "#")
	if hashIdx < 0 {
		return nil, fmt.Errorf("source comment URL missing fragment (#commit:filepath): %q: %w", rawURL, ErrInvalidSourceComment)
	}

	cloneURL := withoutPrefix[:hashIdx]
	fragment := withoutPrefix[hashIdx+1:]

	colonIdx := strings.Index(fragment, ":")
	if colonIdx < 0 {
		return nil, fmt.Errorf("source comment fragment missing filepath (commit:filepath): %q: %w", rawURL, ErrInvalidSourceComment)
	}

	commitHash := fragment[:colonIdx]
	filePath := fragment[colonIdx+1:]

	if commitHash == "" {
		return nil, fmt.Errorf("source comment has empty commit hash: %q: %w", rawURL, ErrInvalidSourceComment)
	}
	if filePath == "" {
		return nil, fmt.Errorf("source comment has empty filepath: %q: %w", rawURL, ErrInvalidSourceComment)
	}

	return &SourceInfo{
		RawURL:     rawURL,
		CloneURL:   cloneURL,
		CommitHash: commitHash,
		FilePath:   filePath,
	}, nil
}

// WriteSourceComment writes file content with source comment as line 1.
func WriteSourceComment(fs afero.Fs, filePath string, content []byte, cloneURL, commitHash, repoFilePath string) error {
	comment := FormatSourceComment(cloneURL, commitHash, repoFilePath)
	data := append([]byte(comment), content...)
	return afero.WriteFile(fs, filePath, data, 0644)
}

// UpdateSourceComment updates the commit hash in an existing source comment.
func UpdateSourceComment(fs afero.Fs, filePath string, newCommitHash string) error {
	content, err := afero.ReadFile(fs, filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	info, err := ParseSourceComment(content)
	if err != nil {
		return fmt.Errorf("parsing source comment: %w", err)
	}
	if info == nil {
		return fmt.Errorf("no source comment found in %s: %w", filePath, ErrNoSourceComment)
	}

	oldLine := sourceCommentPrefix + info.RawURL
	newComment := FormatSourceComment(info.CloneURL, newCommitHash, info.FilePath)
	// newComment has trailing newline; oldLine does not — match consistently
	newContent := strings.Replace(string(content), oldLine+"\n", newComment, 1)

	return afero.WriteFile(fs, filePath, []byte(newContent), 0644)
}

// StripSourceComment removes the source comment line from content.
func StripSourceComment(content []byte) []byte {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lines []string
	skipped := false
	inLeadingComments := true

	for scanner.Scan() {
		line := scanner.Text()
		if inLeadingComments && strings.HasPrefix(line, "#") {
			if !skipped && strings.HasPrefix(line, sourceCommentPrefix) {
				skipped = true
				continue
			}
			lines = append(lines, line)
			continue
		}
		inLeadingComments = false
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil
	}

	return []byte(strings.Join(lines, "\n") + "\n")
}
