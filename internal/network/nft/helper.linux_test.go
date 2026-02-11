package nft

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// NewLinuxHelper Factory Tests
// =============================================================================

func TestNewLinuxHelper_ReturnsNilWithoutLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: nil}
	h := NewLinuxHelper(cfg, runtime.PlatformLinux)
	assert.Nil(t, h, "should return nil when no LAN access is configured")
}

func TestNewLinuxHelper_ReturnsHelperWithLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"*"}}
	h := NewLinuxHelper(cfg, runtime.PlatformLinux)
	assert.NotNil(t, h, "should return non-nil helper when LAN access is configured")
}

// =============================================================================
// Setup Tests
// =============================================================================

func TestLinuxSetup_ReturnsNoError(t *testing.T) {
	env := shared.NewTestNetworkEnv()
	h := &nftLinuxHelper{}

	action, err := h.Setup(env, "/project", nil)
	require.NoError(t, err)
	assert.NotNil(t, action, "should return a non-nil PostCommitAction")
}

func TestLinuxSetup_IsNoOp(t *testing.T) {
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "/project", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.Setup(env, "/project", nil)
	require.NoError(t, err)

	assert.Empty(t, mockCmd.Calls, "Setup should not execute any commands")
}

// =============================================================================
// Teardown Tests
// =============================================================================

func TestLinuxTeardown_ReturnsNoError(t *testing.T) {
	env := shared.NewTestNetworkEnv()
	h := &nftLinuxHelper{}

	err := h.Teardown(env, "/project")
	assert.NoError(t, err)
}

func TestLinuxTeardown_IsNoOp(t *testing.T) {
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(afero.NewMemMapFs(), mockCmd, "/project", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_ = h.Teardown(env, "/project")
	assert.Empty(t, mockCmd.Calls, "Teardown should not execute any commands")
}

// =============================================================================
// HelperStatus Tests
// =============================================================================

func TestLinuxHelperStatus_NotInstalledWhenDirMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	status := h.HelperStatus(context.Background(), env)
	assert.False(t, status.Installed, "should not be installed when directory doesn't exist")
	assert.False(t, status.NeedsUpdate)
}

func TestLinuxHelperStatus_NotInstalledWhenDirExistsButNoInclude(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte("#!/usr/sbin/nft -f\n"), 0644))
	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	status := h.HelperStatus(context.Background(), env)
	assert.False(t, status.Installed, "should not be installed when include line is missing")
}

func TestLinuxHelperStatus_InstalledWhenDirAndIncludeExist(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	content := "#!/usr/sbin/nft -f\n" + alcatrazIncludeLineOnLinux + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))
	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	status := h.HelperStatus(context.Background(), env)
	assert.True(t, status.Installed, "should be installed when directory and include line both exist")
	assert.False(t, status.NeedsUpdate, "NeedsUpdate should always be false for Linux helper")
}

func TestLinuxHelperStatus_NotInstalledWhenDirExistsButConfMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	status := h.HelperStatus(context.Background(), env)
	assert.False(t, status.Installed, "should not be installed when nftables.conf doesn't exist")
}

// =============================================================================
// DetailedStatus Tests
// =============================================================================

func TestLinuxDetailedStatus_EmptyWhenDirMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	info := h.DetailedStatus(env)
	assert.Empty(t, info.RuleFiles, "should return empty when directory doesn't exist")
}

func TestLinuxDetailedStatus_ListsNftFiles(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))

	ruleContent1 := "table inet alca-abc { }"
	ruleContent2 := "table inet alca-def { }"
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "abc.nft"), []byte(ruleContent1), 0644))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "def.nft"), []byte(ruleContent2), 0644))

	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	info := h.DetailedStatus(env)
	assert.Len(t, info.RuleFiles, 2, "should list two .nft files")

	names := make(map[string]string)
	for _, rf := range info.RuleFiles {
		names[rf.Name] = rf.Content
	}
	assert.Equal(t, ruleContent1, names["abc.nft"])
	assert.Equal(t, ruleContent2, names["def.nft"])
}

func TestLinuxDetailedStatus_SkipsNonNftFiles(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "rules.nft"), []byte("content"), 0644))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "readme.txt"), []byte("text"), 0644))

	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	info := h.DetailedStatus(env)
	assert.Len(t, info.RuleFiles, 1, "should only list .nft files")
	assert.Equal(t, "rules.nft", info.RuleFiles[0].Name)
}

func TestLinuxDetailedStatus_SkipsDirectories(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Join(alcatrazNftDirOnLinux, "subdir.nft"), 0755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "rules.nft"), []byte("content"), 0644))

	env := shared.NewNetworkEnv(fs, util.NewMockCommandRunner(), "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	info := h.DetailedStatus(env)
	assert.Len(t, info.RuleFiles, 1, "should skip directories even if named .nft")
}

// =============================================================================
// InstallHelper Tests
// =============================================================================

func TestLinuxInstallHelper_CreatesDirectory(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.InstallHelper(env, nil)
	require.NoError(t, err)

	exists, _ := afero.DirExists(fs, alcatrazNftDirOnLinux)
	assert.True(t, exists, "InstallHelper should create the nftables directory")
}

func TestLinuxInstallHelper_AddsIncludeLine(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte("#!/usr/sbin/nft -f\n"), 0644))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.InstallHelper(env, nil)
	require.NoError(t, err)

	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)
	assert.Contains(t, string(content), alcatrazIncludeLineOnLinux,
		"InstallHelper should add include line to nftables.conf")
}

func TestLinuxInstallHelper_SkipsIncludeLineIfAlreadyPresent(t *testing.T) {
	fs := afero.NewMemMapFs()
	existing := "#!/usr/sbin/nft -f\n" + alcatrazIncludeLineOnLinux + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(existing), 0644))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.InstallHelper(env, nil)
	require.NoError(t, err)

	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	s := string(content)
	first := strings.Index(s, alcatrazIncludeLineOnLinux)
	second := strings.Index(s[first+1:], alcatrazIncludeLineOnLinux)
	assert.Equal(t, -1, second, "include line should not be duplicated")
}

func TestLinuxInstallHelper_ReturnsPostCommitAction(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.InstallHelper(env, nil)
	require.NoError(t, err)
	assert.NotNil(t, action, "should return PostCommitAction")
	assert.NotNil(t, action.Run, "PostCommitAction.Run should not be nil")
}

func TestLinuxInstallHelper_PostCommitReloadsNftables(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.InstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	require.NoError(t, err)

	mockCmd.AssertCalled(t, "systemctl enable nftables.service")
	mockCmd.AssertCalled(t, "nft -f "+nftablesConfPathOnLinux)
}

func TestLinuxInstallHelper_PostCommitReturnsErrorOnNftFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectFailure("nft -f "+nftablesConfPathOnLinux, assert.AnError)
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.InstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	assert.Error(t, err, "PostCommitAction should propagate nft reload error")
	assert.Contains(t, err.Error(), "failed to reload nftables")
}

func TestLinuxInstallHelper_AcceptsNilProgress(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.InstallHelper(env, nil)
	assert.NoError(t, err)
}

// =============================================================================
// UninstallHelper Tests
// =============================================================================

func TestLinuxUninstallHelper_RemovesIncludeLine(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\n" + alcatrazIncludeLineOnLinux + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.UninstallHelper(env, nil)
	require.NoError(t, err)

	result, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)
	assert.NotContains(t, string(result), alcatrazIncludeLineOnLinux,
		"UninstallHelper should remove include line")
}

func TestLinuxUninstallHelper_RemovesRuleFiles(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "abc.nft"), []byte("rules"), 0644))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(alcatrazNftDirOnLinux, "def.nft"), []byte("rules"), 0644))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.UninstallHelper(env, nil)
	require.NoError(t, err)

	files, _ := afero.ReadDir(fs, alcatrazNftDirOnLinux)
	assert.Empty(t, files, "UninstallHelper should remove all files in alcatraz dir")
}

func TestLinuxUninstallHelper_ReturnsPostCommitAction(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.UninstallHelper(env, nil)
	require.NoError(t, err)
	assert.NotNil(t, action)
	assert.NotNil(t, action.Run)
}

func TestLinuxUninstallHelper_PostCommitDeletesTablesAndDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("nft list tables", []byte("table inet alca-abc123\ntable inet alca-def456\ntable inet myother\n"))
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.UninstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	require.NoError(t, err)

	mockCmd.AssertCalled(t, "nft delete table inet alca-abc123")
	mockCmd.AssertCalled(t, "nft delete table inet alca-def456")
	mockCmd.AssertNotCalled(t, "nft delete table inet myother")

	exists, _ := afero.DirExists(fs, alcatrazNftDirOnLinux)
	assert.False(t, exists, "PostCommitAction should remove the alcatraz nft directory")
}

func TestLinuxUninstallHelper_PostCommitHandlesNoTables(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("nft list tables", []byte(""))
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.UninstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	assert.NoError(t, err, "should handle empty table listing gracefully")
}

func TestLinuxUninstallHelper_PostCommitHandlesListTablesError(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(alcatrazNftDirOnLinux, 0755))
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectFailure("nft list tables", assert.AnError)
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	action, err := h.UninstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	assert.NoError(t, err, "should not fail when nft list tables errors")

	exists, _ := afero.DirExists(fs, alcatrazNftDirOnLinux)
	assert.False(t, exists, "directory should still be removed even when listing tables fails")
}

func TestLinuxUninstallHelper_AcceptsNilProgress(t *testing.T) {
	fs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(fs, mockCmd, "", runtime.PlatformLinux)
	h := &nftLinuxHelper{}

	_, err := h.UninstallHelper(env, nil)
	assert.NoError(t, err, "should not panic with nil progress")
}
