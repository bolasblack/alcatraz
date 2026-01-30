package util

import (
	"fmt"
	"strings"
	"testing"
)

// MockCommandRunner implements CommandRunner for testing.
// Records all command invocations and returns pre-configured results.
type MockCommandRunner struct {
	// commands maps "name arg1 arg2 ..." to MockResult.
	commands map[string]MockResult

	// defaultError is returned for unexpected commands.
	defaultError error

	// Calls records all command invocations in order.
	Calls []CommandCall
}

// MockResult holds the pre-configured output and error for a command.
type MockResult struct {
	Output []byte
	Err    error
}

// CommandCall records a single command invocation.
type CommandCall struct {
	Name string
	Args []string
	Key  string // "name arg1 arg2 ..."
}

// NewMockCommandRunner creates a mock that fails on unexpected commands.
func NewMockCommandRunner() *MockCommandRunner {
	return &MockCommandRunner{
		commands:     make(map[string]MockResult),
		defaultError: fmt.Errorf("unexpected command"),
	}
}

// Expect registers a command and its expected result.
// cmd format: "name arg1 arg2 ..." (space-separated).
func (m *MockCommandRunner) Expect(cmd string, output []byte, err error) *MockCommandRunner {
	m.commands[cmd] = MockResult{Output: output, Err: err}
	return m
}

// ExpectSuccess is shorthand for Expect(cmd, output, nil).
func (m *MockCommandRunner) ExpectSuccess(cmd string, output []byte) *MockCommandRunner {
	return m.Expect(cmd, output, nil)
}

// ExpectFailure is shorthand for Expect(cmd, nil, err).
func (m *MockCommandRunner) ExpectFailure(cmd string, err error) *MockCommandRunner {
	return m.Expect(cmd, nil, err)
}

// AllowUnexpected makes unexpected commands return empty output and nil error.
func (m *MockCommandRunner) AllowUnexpected() *MockCommandRunner {
	m.defaultError = nil
	return m
}

// Run implements CommandRunner.
func (m *MockCommandRunner) Run(name string, args ...string) ([]byte, error) {
	key := name
	if len(args) > 0 {
		key = name + " " + strings.Join(args, " ")
	}

	m.Calls = append(m.Calls, CommandCall{
		Name: name,
		Args: args,
		Key:  key,
	})

	if result, ok := m.commands[key]; ok {
		return result.Output, result.Err
	}

	if m.defaultError != nil {
		return nil, fmt.Errorf("%w: %s", m.defaultError, key)
	}
	return nil, nil
}

// RunQuiet implements CommandRunner.
func (m *MockCommandRunner) RunQuiet(name string, args ...string) (string, error) {
	output, err := m.Run(name, args...)
	if err != nil {
		return string(output), err
	}
	return "", nil
}

// SudoRun implements CommandRunner.
// Records with key "sudo name arg1 arg2 ...".
func (m *MockCommandRunner) SudoRun(name string, args ...string) error {
	key := "sudo " + name
	if len(args) > 0 {
		key = "sudo " + name + " " + strings.Join(args, " ")
	}

	m.Calls = append(m.Calls, CommandCall{
		Name: "sudo " + name,
		Args: args,
		Key:  key,
	})

	if result, ok := m.commands[key]; ok {
		return result.Err
	}

	if m.defaultError != nil {
		return fmt.Errorf("%w: %s", m.defaultError, key)
	}
	return nil
}

// SudoRunQuiet implements CommandRunner.
// Records with key "sudo name arg1 arg2 ...".
func (m *MockCommandRunner) SudoRunQuiet(name string, args ...string) (string, error) {
	key := "sudo " + name
	if len(args) > 0 {
		key = "sudo " + name + " " + strings.Join(args, " ")
	}

	m.Calls = append(m.Calls, CommandCall{
		Name: "sudo " + name,
		Args: args,
		Key:  key,
	})

	if result, ok := m.commands[key]; ok {
		if result.Err != nil {
			return string(result.Output), result.Err
		}
		return "", nil
	}

	if m.defaultError != nil {
		return "", fmt.Errorf("%w: %s", m.defaultError, key)
	}
	return "", nil
}

// SudoRunScript implements CommandRunner.
// Records with key "sudo sh -c <script>" (truncated to first line for readability).
func (m *MockCommandRunner) SudoRunScript(script string) error {
	key := "sudo sh script"

	m.Calls = append(m.Calls, CommandCall{
		Name: "sudo sh",
		Args: []string{script},
		Key:  key,
	})

	if result, ok := m.commands[key]; ok {
		return result.Err
	}

	if m.defaultError != nil {
		return fmt.Errorf("%w: %s", m.defaultError, key)
	}
	return nil
}

// Called returns true if the command was called at least once.
func (m *MockCommandRunner) Called(cmd string) bool {
	for _, call := range m.Calls {
		if call.Key == cmd {
			return true
		}
	}
	return false
}

// CallCount returns how many times the command was called.
func (m *MockCommandRunner) CallCount(cmd string) int {
	count := 0
	for _, call := range m.Calls {
		if call.Key == cmd {
			count++
		}
	}
	return count
}

// AssertCalled fails the test if the command was not called.
func (m *MockCommandRunner) AssertCalled(t *testing.T, cmd string) {
	t.Helper()
	if !m.Called(cmd) {
		t.Errorf("expected command to be called: %s", cmd)
		t.Errorf("actual calls: %v", m.CallKeys())
	}
}

// AssertNotCalled fails the test if the command was called.
func (m *MockCommandRunner) AssertNotCalled(t *testing.T, cmd string) {
	t.Helper()
	if m.Called(cmd) {
		t.Errorf("expected command NOT to be called: %s", cmd)
	}
}

// CallKeys returns all called command keys for debugging.
func (m *MockCommandRunner) CallKeys() []string {
	keys := make([]string, len(m.Calls))
	for i, call := range m.Calls {
		keys[i] = call.Key
	}
	return keys
}
