package nft

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// DI Contract Tests for NFTables Firewall
//
// These tests verify the Dependency Injection contract:
// 1. New() stores the provided NetworkEnv (doesn't create new deps)
// 2. Operations use the injected Fs for file operations
// 3. Operations use the injected Cmd for nft commands
// =============================================================================

// TestApplyRules_UsesInjectedFs verifies that ApplyRules uses the
// injected Fs for writing rule files, not the real filesystem.
func TestApplyRules_UsesInjectedFs(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
	}

	_, err := firewall.ApplyRules("container123", "172.17.0.2", rules, nil)
	if err != nil {
		t.Fatalf("ApplyRules failed: %v", err)
	}

	// Verify the rule file was written to the mockFs
	rulePath := "/etc/nftables.d/alcatraz/" + nftFileName("/test/project")
	exists, err := afero.Exists(mockFs, rulePath)
	if err != nil {
		t.Fatalf("Error checking file existence: %v", err)
	}
	if !exists {
		t.Errorf("ApplyRules must write to injected Fs - file %s not found in mockFs", rulePath)
	}
}

// TestApplyRules_UsesInjectedCmd verifies that ApplyRules uses the
// injected CommandRunner for nft commands, not a newly created one.
func TestApplyRules_UsesInjectedCmd(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", "")
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
	}

	action, _ := firewall.ApplyRules("container123", "172.17.0.2", rules, nil)

	// Run post-commit action to trigger the nft command
	if action != nil && action.Run != nil {
		_ = action.Run(context.Background(), nil)
	}

	// Verify mockCmd was called (not a new CommandRunner)
	if len(mockCmd.Calls) == 0 {
		t.Fatal("ApplyRules PostCommitAction must use the injected Cmd - no calls recorded on mockCmd")
	}
}

// TestApplyRules_CmdReceivesCorrectArgs verifies that the injected Cmd
// receives the expected arguments for nft -f.
func TestApplyRules_CmdReceivesCorrectArgs(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "10.0.0.1", Port: 443, Protocol: shared.ProtoTCP},
	}

	action, _ := firewall.ApplyRules("abc123", "172.17.0.2", rules, nil)

	// Run post-commit action to trigger the nft command
	if action != nil && action.Run != nil {
		_ = action.Run(context.Background(), nil)
	}

	// Find the nft call
	var nftCall *util.CommandCall
	for i := range mockCmd.Calls {
		if mockCmd.Calls[i].Name == "sudo nft" {
			nftCall = &mockCmd.Calls[i]
			break
		}
	}

	if nftCall == nil {
		t.Fatal("Expected nft call")
	}

	// Verify arguments: -f <rulepath>
	if len(nftCall.Args) < 2 {
		t.Fatalf("Expected 'nft -f <path>', got: nft %v", nftCall.Args)
	}

	if nftCall.Args[0] != "-f" {
		t.Errorf("First arg should be '-f', got: %s", nftCall.Args[0])
	}

	// Should point to the project-path-based rule file
	rulePath := nftCall.Args[1]
	expectedFileName := nftFileName("/test/project")
	if !strings.Contains(rulePath, expectedFileName) {
		t.Errorf("Rule path should contain project-path filename %s, got: %s", expectedFileName, rulePath)
	}
}

// TestApplyRules_FsReceivesCorrectContent verifies that the injected Fs
// receives the rule file with correct content.
func TestApplyRules_FsReceivesCorrectContent(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP},
	}

	_, err := firewall.ApplyRules("testcontainer", "172.17.0.2", rules, nil)
	if err != nil {
		t.Fatalf("ApplyRules failed: %v", err)
	}

	// Read the file content from mockFs
	content, err := afero.ReadFile(mockFs, "/etc/nftables.d/alcatraz/"+nftFileName("/test/project"))
	if err != nil {
		t.Fatalf("Failed to read rule file from mockFs: %v", err)
	}

	// Verify content contains expected nftables elements
	contentStr := string(content)
	if !strings.Contains(contentStr, "table inet alca-testcontaine") {
		t.Errorf("Rule file should contain table declaration, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "192.168.1.100") {
		t.Errorf("Rule file should contain allow rule IP, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "tcp dport 8080") {
		t.Errorf("Rule file should contain port rule, got:\n%s", contentStr)
	}
}

// TestCleanup_UsesInjectedFs verifies that Cleanup uses the injected Fs
// for removing the rule file.
func TestCleanup_UsesInjectedFs(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	// First create a rule file using the project-path-based name
	rulePath := "/etc/nftables.d/alcatraz/" + nftFileName("/test/project")
	_ = mockFs.MkdirAll("/etc/nftables.d/alcatraz", 0755)
	_ = afero.WriteFile(mockFs, rulePath, []byte("test"), 0644)

	// Verify file exists
	exists, _ := afero.Exists(mockFs, rulePath)
	if !exists {
		t.Fatal("Setup failed: rule file not created")
	}

	_, _ = firewall.Cleanup("container123")

	// Verify the file was removed via the injected Fs
	exists, _ = afero.Exists(mockFs, rulePath)
	if exists {
		t.Error("Cleanup must use injected Fs to remove rule file")
	}
}

// TestCleanup_UsesInjectedCmd verifies that Cleanup uses the injected Cmd
// for nft delete table command.
func TestCleanup_UsesInjectedCmd(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", "")
	firewall := New(env)

	action, _ := firewall.Cleanup("container123")

	// Run post-commit action to trigger the nft command
	if action != nil && action.Run != nil {
		_ = action.Run(context.Background(), nil)
	}

	// Verify mockCmd was called (not a new CommandRunner)
	if len(mockCmd.Calls) == 0 {
		t.Fatal("Cleanup PostCommitAction must use the injected Cmd - no calls recorded on mockCmd")
	}
}

// TestCleanup_CmdReceivesDeleteArgs verifies that Cleanup passes the correct
// delete table arguments to nft via the injected Cmd.
func TestCleanup_CmdReceivesDeleteArgs(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", "")
	firewall := New(env)

	action, _ := firewall.Cleanup("abc123def456")

	// Run post-commit action to trigger the nft command
	if action != nil && action.Run != nil {
		_ = action.Run(context.Background(), nil)
	}

	// Find the nft call
	var nftCall *util.CommandCall
	for i := range mockCmd.Calls {
		if mockCmd.Calls[i].Name == "sudo nft" {
			nftCall = &mockCmd.Calls[i]
			break
		}
	}

	if nftCall == nil {
		t.Fatal("Expected nft call")
	}

	// Verify arguments: delete table inet alca-<short-id>
	args := strings.Join(nftCall.Args, " ")
	if !strings.Contains(args, "delete") {
		t.Errorf("nft args should contain 'delete', got: %v", nftCall.Args)
	}
	if !strings.Contains(args, "table") {
		t.Errorf("nft args should contain 'table', got: %v", nftCall.Args)
	}
	if !strings.Contains(args, "inet") {
		t.Errorf("nft args should contain 'inet', got: %v", nftCall.Args)
	}
	if !strings.Contains(args, "alca-abc123def456") {
		t.Errorf("nft args should contain table name, got: %v", nftCall.Args)
	}
}

// =============================================================================
// Task 2: Cleanup deletes BOTH tables (inet isolation + ip proxy)
// =============================================================================

// TestCleanup_DeletesBothInetAndProxyTables verifies that cleanupOnLinux
// deletes both the inet isolation table AND the ip proxy table.
func TestCleanup_DeletesBothInetAndProxyTables(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	defer mockCmd.AssertAllExpectationsMet(t)

	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	containerID := "abc123def456"
	expectedInetTable := "alca-abc123def456"
	expectedProxyTable := "alca-proxy-abc123def456"

	// Expect both delete commands
	mockCmd.ExpectSuccess("sudo nft delete table inet "+expectedInetTable, nil)
	mockCmd.ExpectSuccess("sudo nft delete table ip "+expectedProxyTable, nil)

	action, err := firewall.Cleanup(containerID)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Run post-commit action to trigger the nft commands
	if action != nil && action.Run != nil {
		err = action.Run(context.Background(), nil)
		if err != nil {
			t.Fatalf("PostCommitAction.Run() error = %v", err)
		}
	}
}

// =============================================================================
// Task 3: tryDeleteTablesFromContent tests
// =============================================================================

// TestTryDeleteTablesFromContent_DeletesBothTables verifies that tryDeleteTablesFromContent
// deletes both the inet isolation table and the ip proxy table from content.
func TestTryDeleteTablesFromContent_DeletesBothTables(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	defer mockCmd.AssertAllExpectationsMet(t)

	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	n := New(env).(*NFTables)

	content := "#!/usr/sbin/nft -f\n# Alcatraz container rules for table: alca-abc123def456\n\ntable inet alca-abc123def456 {}\n"

	// Expect delete of both inet and ip proxy tables
	mockCmd.ExpectSuccess("sudo nft delete table inet alca-abc123def456", nil)
	mockCmd.ExpectSuccess("sudo nft delete table ip alca-proxy-abc123def456", nil)

	n.tryDeleteTablesFromContent(context.Background(), content)
}

// TestTryDeleteTablesFromContent_NoTableName verifies that tryDeleteTablesFromContent
// handles content without a table name comment gracefully — no commands, no panics.
func TestTryDeleteTablesFromContent_NoTableName(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	defer mockCmd.AssertAllExpectationsMet(t)

	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	n := New(env).(*NFTables)

	content := "#!/usr/sbin/nft -f\nsome random content\n"

	// Should not panic, should not call any commands
	n.tryDeleteTablesFromContent(context.Background(), content)
}

// TestTryDeleteTablesFromContent_NonAlcaPrefix verifies that proxy table derivation
// still works correctly for non-standard table names (without "alca-" prefix).
func TestTryDeleteTablesFromContent_NonAlcaPrefix(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	defer mockCmd.AssertAllExpectationsMet(t)

	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	n := New(env).(*NFTables)

	// Table name without "alca-" prefix — inet table should be deleted,
	// but proxy table derivation should be skipped (no "alca-" prefix).
	content := "#!/usr/sbin/nft -f\n# Alcatraz container rules for table: custom-table-name\n\ntable inet custom-table-name {}\n"

	// Only inet table deletion expected — proxy derivation skipped due to missing "alca-" prefix
	mockCmd.ExpectSuccess("sudo nft delete table inet custom-table-name", nil)

	n.tryDeleteTablesFromContent(context.Background(), content)
}

// =============================================================================
// Task 4: DI Contract test with non-nil ProxyConfig
// =============================================================================

// TestApplyRules_WithProxy_WritesProxyDNATRules verifies that passing a non-nil
// ProxyConfig to ApplyRules results in the written file containing proxy DNAT rules.
func TestApplyRules_WithProxy_WritesProxyDNATRules(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	proxy := &shared.ProxyConfig{Host: "10.0.0.1", Port: 1080}

	_, err := firewall.ApplyRules("container123", "172.17.0.2", nil, proxy)
	if err != nil {
		t.Fatalf("ApplyRules failed: %v", err)
	}

	// Read the file content from mockFs
	content, err := afero.ReadFile(mockFs, "/etc/nftables.d/alcatraz/"+nftFileName("/test/project"))
	if err != nil {
		t.Fatalf("Failed to read rule file from mockFs: %v", err)
	}

	contentStr := string(content)

	// Verify proxy DNAT rules are present
	if !strings.Contains(contentStr, "table ip alca-proxy-") {
		t.Errorf("Rule file should contain proxy table declaration, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "dnat to 10.0.0.1:1080") {
		t.Errorf("Rule file should contain DNAT rule, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "type nat hook prerouting priority dstnat") {
		t.Errorf("Rule file should contain NAT prerouting chain, got:\n%s", contentStr)
	}
	// Verify routing loop prevention
	if !strings.Contains(contentStr, "ip saddr 172.17.0.2 ip daddr 10.0.0.1 tcp dport 1080 accept") {
		t.Errorf("Rule file should contain routing loop prevention, got:\n%s", contentStr)
	}
}

// TestApplyRules_ReturnsErrorFromInjectedCmd verifies that errors from
// the injected Cmd are properly propagated.
func TestApplyRules_ReturnsErrorFromInjectedCmd(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	expectedErr := errors.New("nft command failed")
	mockCmd := util.NewMockCommandRunner()
	// Return error for nft -f command
	mockCmd.ExpectFailure("sudo nft -f /etc/nftables.d/alcatraz/"+nftFileName("/test/project"), expectedErr)

	env := shared.NewNetworkEnv(mockFs, mockCmd, "/test/project", "", "")
	firewall := New(env)

	action, err := firewall.ApplyRules("container123", "172.17.0.2", nil, nil)
	if err != nil {
		t.Fatalf("ApplyRules file write phase should not error: %v", err)
	}

	// Error should come from the post-commit action
	if action != nil && action.Run != nil {
		err = action.Run(context.Background(), nil)
	}
	if err == nil {
		t.Error("ApplyRules PostCommitAction should propagate errors from the injected Cmd")
	}
}

// TestApplyRules_SkipsWhenAllLAN verifies that when AllLAN is set,
// no commands are executed and no files are written.
func TestApplyRules_SkipsWhenAllLAN(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", "")
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{AllLAN: true},
	}

	_, err := firewall.ApplyRules("container123", "172.17.0.2", rules, nil)

	if err != nil {
		t.Errorf("ApplyRules with AllLAN should not error, got: %v", err)
	}

	// No commands should be called
	if len(mockCmd.Calls) > 0 {
		t.Errorf("ApplyRules with AllLAN should not call any commands, got: %v", mockCmd.CallKeys())
	}

	// No files should be written
	exists, _ := afero.Exists(mockFs, "/etc/nftables.d/alcatraz/"+nftFileName(""))
	if exists {
		t.Error("ApplyRules with AllLAN should not write any files")
	}
}

// TestApplyRules_CreatesDirViaInjectedFs verifies that ApplyRules uses
// the injected Fs to create the nftables.d directory.
func TestApplyRules_CreatesDirViaInjectedFs(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", "")
	firewall := New(env)

	// Directory doesn't exist yet
	exists, _ := afero.DirExists(mockFs, "/etc/nftables.d/alcatraz")
	if exists {
		t.Fatal("Setup error: directory should not exist initially")
	}

	_, _ = firewall.ApplyRules("container123", "172.17.0.2", nil, nil)

	// Directory should now exist on mockFs
	exists, _ = afero.DirExists(mockFs, "/etc/nftables.d/alcatraz")
	if !exists {
		t.Error("ApplyRules must create directory via injected Fs")
	}
}
