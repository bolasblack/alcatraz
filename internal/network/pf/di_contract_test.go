//go:build darwin

package pf

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// DI Contract Tests for PF Firewall
//
// These tests verify the Dependency Injection contract:
// 1. New() stores the provided NetworkEnv (doesn't create new deps)
// 2. Operations use the injected Cmd (not create their own)
// =============================================================================

// TestNew_StoresInjectedEnv verifies that New stores the exact NetworkEnv
// instance provided, not a copy.
func TestNew_StoresInjectedEnv(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", false)

	firewall := New(env)

	// Cast to *PF to access internal env field
	pf, ok := firewall.(*PF)
	if !ok {
		t.Fatalf("New() should return *PF, got %T", firewall)
	}

	if pf.env != env {
		t.Error("New() must store the exact NetworkEnv instance provided")
	}
}

// TestApplyRules_UsesInjectedCmd verifies that ApplyRules uses the
// injected CommandRunner, not a newly created one.
func TestApplyRules_UsesInjectedCmd(t *testing.T) {
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(nil, mockCmd, "", false)
	firewall := New(env)

	// Set up expectation for pfctl command
	// The command pattern is: sh -c "echo \"<ruleset>\" | pfctl -a <anchor> -f -"
	mockCmd.AllowUnexpected()

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
	}

	// Call ApplyRules - we expect it to use mockCmd
	_ = firewall.ApplyRules("container123", "172.20.0.5", rules)

	// Verify mockCmd was called (not a new CommandRunner)
	if len(mockCmd.Calls) == 0 {
		t.Fatal("ApplyRules must use the injected Cmd - no calls recorded on mockCmd")
	}

	// Verify it called 'sh' with the piped command
	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "sh" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ApplyRules should call 'sh' via injected Cmd, got calls: %v", mockCmd.CallKeys())
	}
}

// TestApplyRules_CmdReceivesCorrectArgs verifies that the injected Cmd
// receives the expected arguments for pfctl.
func TestApplyRules_CmdReceivesCorrectArgs(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(nil, mockCmd, "", false)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "10.0.0.1", Port: 443, Protocol: shared.ProtoTCP},
	}

	_ = firewall.ApplyRules("abc123def456", "172.20.0.5", rules)

	// Verify the command includes the anchor name
	if len(mockCmd.Calls) == 0 {
		t.Fatal("Expected at least one command call")
	}

	call := mockCmd.Calls[0]
	if call.Name != "sh" || len(call.Args) < 2 {
		t.Fatalf("Expected 'sh -c <script>', got: %s %v", call.Name, call.Args)
	}

	script := call.Args[1]
	// Should contain the anchor name (truncated container ID)
	if !strings.Contains(script, "alcatraz/abc123def456") {
		t.Errorf("Script should contain anchor name 'alcatraz/abc123def456', got: %s", script)
	}
	// Should contain pfctl command
	if !strings.Contains(script, "pfctl") {
		t.Errorf("Script should contain 'pfctl', got: %s", script)
	}
}

// TestCleanup_UsesInjectedCmd verifies that Cleanup uses the injected
// CommandRunner for pfctl flush operations.
func TestCleanup_UsesInjectedCmd(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(nil, mockCmd, "", false)
	firewall := New(env)

	_ = firewall.Cleanup("container123")

	// Verify mockCmd was called
	if len(mockCmd.Calls) == 0 {
		t.Fatal("Cleanup must use the injected Cmd - no calls recorded on mockCmd")
	}

	// Verify it called pfctl directly
	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "pfctl" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Cleanup should call 'pfctl' via injected Cmd, got calls: %v", mockCmd.CallKeys())
	}
}

// TestCleanup_CmdReceivesFlushArgs verifies that Cleanup passes the correct
// flush arguments to pfctl via the injected Cmd.
func TestCleanup_CmdReceivesFlushArgs(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(nil, mockCmd, "", false)
	firewall := New(env)

	_ = firewall.Cleanup("abc123def456")

	// Find the pfctl call
	var pfctlCall *util.CommandCall
	for i := range mockCmd.Calls {
		if mockCmd.Calls[i].Name == "pfctl" {
			pfctlCall = &mockCmd.Calls[i]
			break
		}
	}

	if pfctlCall == nil {
		t.Fatal("Expected pfctl call")
	}

	// Verify arguments: -a <anchor> -F all
	args := strings.Join(pfctlCall.Args, " ")
	if !strings.Contains(args, "-a") {
		t.Errorf("pfctl args should contain '-a', got: %v", pfctlCall.Args)
	}
	if !strings.Contains(args, "alcatraz/abc123def456") {
		t.Errorf("pfctl args should contain anchor name, got: %v", pfctlCall.Args)
	}
	if !strings.Contains(args, "-F all") {
		t.Errorf("pfctl args should contain '-F all' for flush, got: %v", pfctlCall.Args)
	}
}

// TestApplyRules_ReturnsErrorFromInjectedCmd verifies that errors from
// the injected Cmd are properly propagated.
func TestApplyRules_ReturnsErrorFromInjectedCmd(t *testing.T) {
	expectedErr := errors.New("pfctl command failed")
	mockCmd := util.NewMockCommandRunner()
	// Configure mock to return an error for any sh command
	mockCmd.ExpectFailure("sh -c echo \"# Block RFC1918 and other private ranges\\nblock drop quick from 172.20.0.5 to 10.0.0.0/8\\nblock drop quick from 172.20.0.5 to 172.16.0.0/12\\nblock drop quick from 172.20.0.5 to 192.168.0.0/16\\nblock drop quick from 172.20.0.5 to 169.254.0.0/16\\nblock drop quick from 172.20.0.5 to 127.0.0.0/8\\n\" | pfctl -a alcatraz/abc123def456 -f -", expectedErr)
	// Also set default error for any command
	mockCmd.AllowUnexpected() // We'll check the error separately

	// Create a mock that returns errors
	mockCmdWithErr := util.NewMockCommandRunner()
	mockCmdWithErr.Expect("sh -c echo \"# Block RFC1918 and other private ranges\\nblock drop quick from 172.20.0.5 to 10.0.0.0/8\\nblock drop quick from 172.20.0.5 to 172.16.0.0/12\\nblock drop quick from 172.20.0.5 to 192.168.0.0/16\\nblock drop quick from 172.20.0.5 to 169.254.0.0/16\\nblock drop quick from 172.20.0.5 to 127.0.0.0/8\\n\" | pfctl -a alcatraz/abc123def456 -f -", nil, expectedErr)

	env := shared.NewNetworkEnv(nil, mockCmdWithErr, "", false)
	firewall := New(env)

	err := firewall.ApplyRules("abc123def456", "172.20.0.5", nil)

	// Should return an error (the exact error is wrapped, so check it's not nil)
	if err == nil {
		t.Error("ApplyRules should propagate errors from the injected Cmd")
	}
}

// TestApplyRules_SkipsWhenAllLAN verifies that when AllLAN is set,
// no commands are executed via the injected Cmd.
func TestApplyRules_SkipsWhenAllLAN(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(nil, mockCmd, "", false)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{AllLAN: true},
	}

	err := firewall.ApplyRules("container123", "172.20.0.5", rules)

	if err != nil {
		t.Errorf("ApplyRules with AllLAN should not error, got: %v", err)
	}

	// No commands should be called
	if len(mockCmd.Calls) > 0 {
		t.Errorf("ApplyRules with AllLAN should not call any commands, got: %v", mockCmd.CallKeys())
	}
}
