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
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "", false)
	firewall := New(env)

	// All commands go through SudoRunQuiet, so Name is "sudo" with actual command in Args.
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

	// Verify it called 'sudo sh' for the pfctl load command
	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "sudo sh" && strings.Contains(strings.Join(call.Args, " "), "pfctl") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ApplyRules should call 'sudo sh -c ... pfctl ...' via injected Cmd, got calls: %v", mockCmd.CallKeys())
	}
}

// TestApplyRules_CmdReceivesCorrectArgs verifies that the injected Cmd
// receives the expected arguments for pfctl.
func TestApplyRules_CmdReceivesCorrectArgs(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "", false)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "10.0.0.1", Port: 443, Protocol: shared.ProtoTCP},
	}

	_ = firewall.ApplyRules("abc123def456", "172.20.0.5", rules)

	// Verify the load command includes the anchor name and pfctl
	if len(mockCmd.Calls) == 0 {
		t.Fatal("Expected at least one command call")
	}

	// Find the sudo sh call that loads rules via pfctl
	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "sudo sh" {
			args := strings.Join(call.Args, " ")
			if strings.Contains(args, "pfctl -a "+pfAnchorName) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("Expected 'sudo sh -c ... pfctl -a %s ...' call, got: %v", pfAnchorName, mockCmd.CallKeys())
	}
}

// TestCleanup_UsesInjectedCmd verifies that Cleanup uses the injected
// CommandRunner for rule removal and anchor reload.
func TestCleanup_UsesInjectedCmd(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "", false)
	firewall := New(env)

	_ = firewall.Cleanup("container123")

	// Verify mockCmd was called
	if len(mockCmd.Calls) == 0 {
		t.Fatal("Cleanup must use the injected Cmd - no calls recorded on mockCmd")
	}

	// Verify it called sudo rm (for rule file removal) and sudo sh (for anchor reload)
	hasRm := false
	hasSh := false
	for _, call := range mockCmd.Calls {
		if call.Name == "sudo rm" {
			hasRm = true
		}
		if call.Name == "sudo sh" {
			hasSh = true
		}
	}
	if !hasRm {
		t.Errorf("Cleanup should call 'sudo rm' via injected Cmd, got calls: %v", mockCmd.CallKeys())
	}
	if !hasSh {
		t.Errorf("Cleanup should call 'sudo sh' via injected Cmd, got calls: %v", mockCmd.CallKeys())
	}
}

// TestCleanup_CmdReceivesCorrectArgs verifies that Cleanup removes rule files
// and reloads the anchor via the injected Cmd.
func TestCleanup_CmdReceivesCorrectArgs(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "", false)
	firewall := New(env)

	_ = firewall.Cleanup("abc123def456")

	// Find the sudo sh call that reloads the anchor
	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "sudo sh" {
			args := strings.Join(call.Args, " ")
			if strings.Contains(args, "pfctl") && strings.Contains(args, pfAnchorName) {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected 'sudo sh -c ... pfctl ... %s ...' call, got: %v", pfAnchorName, mockCmd.CallKeys())
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

	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmdWithErr, "", false)
	firewall := New(env)

	err := firewall.ApplyRules("abc123def456", "172.20.0.5", nil)

	// Should return an error (the exact error is wrapped, so check it's not nil)
	if err == nil {
		t.Error("ApplyRules should propagate errors from the injected Cmd")
	}
}

// TestApplyRules_NoFilterRulesWhenAllLAN verifies that when AllLAN is set,
// no filter rules are generated but cleanup commands still run.
func TestApplyRules_NoFilterRulesWhenAllLAN(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "", false)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{AllLAN: true},
	}

	err := firewall.ApplyRules("container123", "172.20.0.5", rules)

	if err != nil {
		t.Errorf("ApplyRules with AllLAN should not error, got: %v", err)
	}

	// Commands are expected (cleanup old files, reload anchor),
	// but no filter rule file should be written (only rm -f calls).
	for _, call := range mockCmd.Calls {
		if call.Name == "mv" {
			t.Errorf("ApplyRules with AllLAN should not write rule files, got mv call: %v", call.Args)
		}
	}
}
