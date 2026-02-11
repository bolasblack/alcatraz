package nft

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// Tests for macOS rule generation (AGD-030)
// =============================================================================

func TestChainPriority(t *testing.T) {
	tests := []struct {
		name     string
		runtime  runtime.RuntimePlatform
		expected string
	}{
		{
			name:     "OrbStack uses filter - 2",
			runtime:  runtime.PlatformMacOrbStack,
			expected: "filter - 2",
		},
		{
			name:     "Docker Desktop uses filter - 1",
			runtime:  runtime.PlatformMacDockerDesktop,
			expected: "filter - 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chainPriority(tt.runtime)
			if got != tt.expected {
				t.Errorf("chainPriority(%q) = %q, want %q", tt.runtime, got, tt.expected)
			}
		})
	}
}

func TestGenerateRulesetWithPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		expected string
	}{
		{
			name:     "OrbStack priority",
			priority: "filter - 2",
			expected: "priority filter - 2",
		},
		{
			name:     "Docker Desktop priority",
			priority: "filter - 1",
			expected: "priority filter - 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleset := generateRuleset("alca-test", "172.17.0.2", nil, tt.priority, "/test/project", "")
			if !strings.Contains(ruleset, tt.expected) {
				t.Errorf("ruleset should contain %q\nGot:\n%s", tt.expected, ruleset)
			}
		})
	}
}

// =============================================================================
// DI Contract Tests for macOS ApplyRules/Cleanup (per-container)
// =============================================================================

func TestApplyRulesOnDarwin_WritesPerContainerFile(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/Users/alice/myproject", runtime.PlatformMacOrbStack)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 80, Protocol: shared.ProtoTCP},
	}

	_, err := firewall.ApplyRules("container123", "172.17.0.2", rules)
	if err != nil {
		t.Fatalf("ApplyRules failed: %v", err)
	}

	// Verify the rule file uses project-path-based naming
	dir, _ := nftDirOnDarwin()
	expectedFile := dir + "/" + nftFileName("/Users/alice/myproject")
	exists, err := afero.Exists(mockFs, expectedFile)
	if err != nil {
		t.Fatalf("Error checking file existence: %v", err)
	}
	if !exists {
		t.Errorf("ApplyRules (mac) must write per-container file %s", expectedFile)
	}
}

func TestApplyRulesOnDarwin_TriggersReload(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, []byte("true\n"))
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/Users/alice/myproject", runtime.PlatformMacOrbStack)
	firewall := New(env)

	action, _ := firewall.ApplyRules("container123", "172.17.0.2", nil)

	// Run post-commit action to trigger reload
	if action != nil && action.Run != nil {
		_ = action.Run(context.Background(), nil)
	}

	// Verify docker exec was called for reload
	mockCmd.AssertCalled(t, "docker exec "+vmhelper.ContainerName+" sh -c kill -HUP 1")
}

func TestApplyRulesOnDarwin_ReloadFailsWhenHelperNotInstalled(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	// docker inspect fails → container not found
	mockCmd.ExpectFailure("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, assert.AnError)
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/Users/alice/myproject", runtime.PlatformMacOrbStack)
	firewall := New(env)

	action, err := firewall.ApplyRules("container123", "172.17.0.2", nil)
	if err != nil {
		t.Fatalf("ApplyRules should not fail (file write phase): %v", err)
	}

	// Post-commit reload should fail with descriptive error
	if action != nil && action.Run != nil {
		err = action.Run(context.Background(), nil)
		if err == nil {
			t.Fatal("reloadVMHelper should fail when helper is not installed")
		}
		if !strings.Contains(err.Error(), "not installed") {
			t.Errorf("error should mention 'not installed', got: %v", err)
		}
	}
}

func TestApplyRulesOnDarwin_WritesPerContainerRuleset(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/Users/alice/myproject", runtime.PlatformMacOrbStack)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{IP: "192.168.1.100", Port: 8080, Protocol: shared.ProtoTCP},
	}

	_, err := firewall.ApplyRules("container123", "172.17.0.2", rules)
	if err != nil {
		t.Fatalf("ApplyRules failed: %v", err)
	}

	dir, _ := nftDirOnDarwin()
	content, err := afero.ReadFile(mockFs, dir+"/"+nftFileName("/Users/alice/myproject"))
	if err != nil {
		t.Fatalf("Failed to read rule file: %v", err)
	}

	contentStr := string(content)
	// Verify it uses per-container table name
	expectedTable := "table inet " + tableName("container123") + " {"
	if !strings.Contains(contentStr, expectedTable) {
		t.Errorf("macOS rule file should use per-container table\nGot:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "192.168.1.100 tcp dport 8080 accept") {
		t.Errorf("macOS rule file should contain allow rule\nGot:\n%s", contentStr)
	}
	// Verify OrbStack priority
	if !strings.Contains(contentStr, "priority filter - 2") {
		t.Errorf("OrbStack ruleset should use priority filter - 2\nGot:\n%s", contentStr)
	}
}

func TestCleanupOnDarwin_RemovesPerContainerFile(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, []byte("true\n"))
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/Users/alice/myproject", runtime.PlatformMacOrbStack)
	firewall := New(env)

	// Create the per-project rule file
	dir, _ := nftDirOnDarwin()
	_ = mockFs.MkdirAll(dir, 0755)
	rulePath := dir + "/" + nftFileName("/Users/alice/myproject")
	_ = afero.WriteFile(mockFs, rulePath, []byte("test"), 0644)

	action, err := firewall.Cleanup("container123")
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify file was removed
	exists, _ := afero.Exists(mockFs, rulePath)
	if exists {
		t.Error("Cleanup (mac) must remove the per-container rule file")
	}

	// Run post-commit action to trigger reload
	if action != nil && action.Run != nil {
		_ = action.Run(context.Background(), nil)
	}

	// Verify reload was triggered
	mockCmd.AssertCalled(t, "docker exec "+vmhelper.ContainerName+" sh -c kill -HUP 1")
}

func TestApplyRulesOnDarwin_SkipsWhenAllLAN(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/Users/alice/myproject", runtime.PlatformMacOrbStack)
	firewall := New(env)

	rules := []shared.LANAccessRule{
		{AllLAN: true},
	}

	_, err := firewall.ApplyRules("container123", "172.17.0.2", rules)
	if err != nil {
		t.Errorf("ApplyRules with AllLAN should not error, got: %v", err)
	}

	if len(mockCmd.Calls) > 0 {
		t.Errorf("ApplyRules with AllLAN should not call any commands, got: %v", mockCmd.CallKeys())
	}
}
