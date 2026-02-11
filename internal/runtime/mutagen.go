// Package runtime provides container runtime abstraction for Alcatraz.
// This file implements Mutagen file sync integration for mount exclude support.
// See AGD-025 for Mutagen integration design decisions.
package runtime

import (
	"fmt"
	"strings"
	"time"
)

// MutagenSync manages a Mutagen sync session.
// Each session syncs files from a host source to a container target,
// with optional ignore patterns for excluding files.
type MutagenSync struct {
	Name    string   // Session name (unique per project+mount)
	Source  string   // Host path
	Target  string   // Container path (format: docker://container-id/path)
	Ignores []string // Patterns to ignore (gitignore-like syntax)
}

// Create creates a new Mutagen sync session.
// CLI command: mutagen sync create --name=<name> [--ignore=<pattern>]... <source> <target>
func (m *MutagenSync) Create(env *RuntimeEnv) error {
	args := m.buildCreateArgs()
	output, err := env.Cmd.RunQuiet("mutagen", args...)
	if err != nil {
		return fmt.Errorf("mutagen sync create failed: %w: %s", err, string(output))
	}
	return nil
}

// Flush waits for a Mutagen sync session to complete its current sync cycle.
// Retries if the session is not yet connected (e.g. just created).
// CLI command: mutagen sync flush <name>
func (m *MutagenSync) Flush(env *RuntimeEnv) error {
	return m.flushWithRetry(env, flushMaxRetries, flushRetryInterval)
}

const (
	flushMaxRetries    = 30
	flushRetryInterval = time.Second
)

func (m *MutagenSync) flushWithRetry(env *RuntimeEnv, maxRetries int, interval time.Duration) error {
	args := []string{"sync", "flush", m.Name}
	for attempt := range maxRetries {
		output, err := env.Cmd.RunQuiet("mutagen", args...)
		if err == nil {
			return nil
		}
		if attempt == maxRetries-1 || !isFlushRetryable(string(output)) {
			return fmt.Errorf("mutagen sync flush failed: %w: %s", err, string(output))
		}
		time.Sleep(interval)
	}
	return nil // unreachable
}

// isFlushRetryable returns true if the flush error indicates the session
// is still connecting and a retry may succeed.
func isFlushRetryable(output string) bool {
	return strings.Contains(output, "not currently able to synchronize")
}

// Terminate terminates a Mutagen sync session.
// CLI command: mutagen sync terminate <name>
func (m *MutagenSync) Terminate(env *RuntimeEnv) error {
	args := m.buildTerminateArgs()
	output, err := env.Cmd.RunQuiet("mutagen", args...)
	if err != nil {
		if strings.Contains(string(output), "no matching sessions") {
			return nil
		}
		return fmt.Errorf("mutagen sync terminate failed: %w: %s", err, string(output))
	}
	return nil
}

// buildCreateArgs constructs the arguments for mutagen sync create.
func (m *MutagenSync) buildCreateArgs() []string {
	args := []string{"sync", "create", "--name=" + m.Name}

	// Add ignore patterns
	for _, pattern := range m.Ignores {
		args = append(args, "--ignore="+pattern)
	}

	// Add source and target
	args = append(args, m.Source, m.Target)

	return args
}

// buildTerminateArgs constructs the arguments for mutagen sync terminate.
func (m *MutagenSync) buildTerminateArgs() []string {
	return []string{"sync", "terminate", m.Name}
}

// ListMutagenSyncs lists all Mutagen sync sessions matching a name prefix.
// CLI command: mutagen sync list --template='{{.Name}}'
func ListMutagenSyncs(env *RuntimeEnv, namePrefix string) ([]string, error) {
	args := buildListSyncsArgs()
	output, err := env.Cmd.RunQuiet("mutagen", args...)
	if err != nil {
		return []string{}, nil
	}
	return parseMutagenListOutput(string(output), namePrefix), nil
}

// buildListSyncsArgs constructs the arguments for mutagen sync list.
// mutagen passes []Session to the template, so we must use range.
func buildListSyncsArgs() []string {
	return []string{"sync", "list", `--template={{range .}}{{.Name}}{{"\n"}}{{end}}`}
}

// parseMutagenListOutput parses the output of mutagen sync list and filters by prefix.
func parseMutagenListOutput(output string, namePrefix string) []string {
	var result []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" && strings.HasPrefix(name, namePrefix) {
			result = append(result, name)
		}
	}
	return result
}

// ListSessionJSON returns raw JSON output for a Mutagen sync session.
// CLI command: mutagen sync list <sessionName> --template='{{json .}}'
func ListSessionJSON(env *RuntimeEnv, sessionName string) ([]byte, error) {
	args := buildListSessionJSONArgs(sessionName)
	output, err := env.Cmd.RunQuiet("mutagen", args...)
	if err != nil {
		return nil, fmt.Errorf("mutagen sync list failed: %w: %s", err, string(output))
	}
	return output, nil
}

// buildListSessionJSONArgs constructs the arguments for mutagen sync list JSON output.
func buildListSessionJSONArgs(sessionName string) []string {
	return []string{"sync", "list", sessionName, "--template={{json .}}"}
}

// MutagenSessionName generates a unique session name for a project mount.
// Format: alca-<projectID>-<mountIndex>
func MutagenSessionName(projectID string, mountIndex int) string {
	return fmt.Sprintf("alca-%s-%d", projectID, mountIndex)
}

// MutagenTarget generates a Mutagen target URL for a container path.
// Format: docker://<containerID>/<path>
func MutagenTarget(containerID string, path string) string {
	return fmt.Sprintf("docker://%s%s", containerID, path)
}

// TerminateProjectSyncs terminates all Mutagen sync sessions for a project.
// Used during container cleanup (down command).
func TerminateProjectSyncs(env *RuntimeEnv, projectID string) error {
	prefix := fmt.Sprintf("alca-%s-", projectID)
	sessions, err := ListMutagenSyncs(env, prefix)
	if err != nil {
		return err
	}

	var lastErr error
	for _, name := range sessions {
		sync := MutagenSync{Name: name}
		if err := sync.Terminate(env); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
