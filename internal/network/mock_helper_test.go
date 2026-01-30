package network

import "github.com/bolasblack/alcatraz/internal/util"

// MockNetworkHelper implements NetworkHelper for testing.
// Provides test configuration fields and call recording.
type MockNetworkHelper struct {
	// ========== Test Configuration (set before calling) ==========

	// ReturnStatus is returned by HelperStatus()
	ReturnStatus HelperStatus

	// ReturnDetailedStatus is returned by DetailedStatus()
	ReturnDetailedStatus DetailedStatusInfo

	// ReturnSetupError is returned by Setup()
	ReturnSetupError error

	// ReturnSetupAction is returned by Setup() (default: empty action)
	ReturnSetupAction *PostCommitAction

	// ReturnInstallError is returned by InstallHelper()
	ReturnInstallError error

	// ReturnInstallAction is returned by InstallHelper()
	ReturnInstallAction *PostCommitAction

	// ReturnUninstallError is returned by UninstallHelper()
	ReturnUninstallError error

	// ReturnTeardownError is returned by Teardown()
	ReturnTeardownError error

	// ========== Call Recording (check after calling) ==========

	// SetupCalls records all Setup() invocations
	SetupCalls []SetupCall

	// TeardownCalls records all Teardown() invocations
	TeardownCalls []TeardownCall

	// InstallHelperCalled is true if InstallHelper() was called
	InstallHelperCalled bool

	// UninstallHelperCalled is true if UninstallHelper() was called
	UninstallHelperCalled bool

	// HelperStatusCalled is true if HelperStatus() was called
	HelperStatusCalled bool

	// DetailedStatusCalled is true if DetailedStatus() was called
	DetailedStatusCalled bool
}

// SetupCall records a call to Setup()
type SetupCall struct {
	ProjectDir string
}

// TeardownCall records a call to Teardown()
type TeardownCall struct {
	ProjectDir string
}

func (m *MockNetworkHelper) HelperStatus(env *util.Env) HelperStatus {
	m.HelperStatusCalled = true
	return m.ReturnStatus
}

func (m *MockNetworkHelper) DetailedStatus(env *util.Env) DetailedStatusInfo {
	m.DetailedStatusCalled = true
	return m.ReturnDetailedStatus
}

func (m *MockNetworkHelper) Setup(env *util.Env, projectDir string, progress ProgressFunc) (*PostCommitAction, error) {
	m.SetupCalls = append(m.SetupCalls, SetupCall{ProjectDir: projectDir})

	if m.ReturnSetupError != nil {
		return nil, m.ReturnSetupError
	}

	action := m.ReturnSetupAction
	if action == nil {
		action = &PostCommitAction{} // default: no post-commit action
	}
	return action, nil
}

func (m *MockNetworkHelper) Teardown(env *util.Env, projectDir string) error {
	m.TeardownCalls = append(m.TeardownCalls, TeardownCall{ProjectDir: projectDir})
	return m.ReturnTeardownError
}

func (m *MockNetworkHelper) InstallHelper(env *util.Env, progress ProgressFunc) (*PostCommitAction, error) {
	m.InstallHelperCalled = true

	if m.ReturnInstallError != nil {
		return nil, m.ReturnInstallError
	}

	action := m.ReturnInstallAction
	if action == nil {
		action = &PostCommitAction{}
	}
	return action, nil
}

func (m *MockNetworkHelper) UninstallHelper(env *util.Env, progress ProgressFunc) (*PostCommitAction, error) {
	m.UninstallHelperCalled = true

	if m.ReturnUninstallError != nil {
		return nil, m.ReturnUninstallError
	}
	return &PostCommitAction{}, nil
}
