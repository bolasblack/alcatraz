package network

import "github.com/bolasblack/alcatraz/internal/network/shared"

// MockFirewall implements Firewall for testing.
// Provides test configuration fields and call recording.
type MockFirewall struct {
	// ========== Test Configuration (set before calling) ==========

	// ReturnApplyError is returned by ApplyRules()
	ReturnApplyError error

	// ReturnCleanupError is returned by Cleanup()
	ReturnCleanupError error

	// ========== Call Recording (check after calling) ==========

	// ApplyRulesCalls records all ApplyRules() invocations
	ApplyRulesCalls []ApplyRulesCall

	// CleanupCalls records all Cleanup() invocations
	CleanupCalls []CleanupCall
}

// ApplyRulesCall records a call to ApplyRules()
type ApplyRulesCall struct {
	ContainerID string
	ContainerIP string
	Rules       []shared.LANAccessRule
}

// CleanupCall records a call to Cleanup()
type CleanupCall struct {
	ContainerID string
}

// Compile-time interface assertion.
var _ Firewall = (*MockFirewall)(nil)

func (m *MockFirewall) ApplyRules(containerID string, containerIP string, rules []LANAccessRule) (*PostCommitAction, error) {
	m.ApplyRulesCalls = append(m.ApplyRulesCalls, ApplyRulesCall{
		ContainerID: containerID,
		ContainerIP: containerIP,
		Rules:       rules,
	})
	return &PostCommitAction{}, m.ReturnApplyError
}

func (m *MockFirewall) Cleanup(containerID string) (*PostCommitAction, error) {
	m.CleanupCalls = append(m.CleanupCalls, CleanupCall{
		ContainerID: containerID,
	})
	return &PostCommitAction{}, m.ReturnCleanupError
}

func (m *MockFirewall) CleanupStaleFiles() (int, error) {
	return 0, nil
}
