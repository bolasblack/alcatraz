package nft

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// NewDarwinHelper Tests
// =============================================================================

func TestNewDarwinHelper_ReturnsNilWhenNoLANAccess(t *testing.T) {
	cfg := config.Network{LANAccess: nil}
	helper := NewDarwinHelper(cfg, runtime.PlatformMacOrbStack)
	assert.Nil(t, helper, "NewDarwinHelper should return nil when LANAccess is nil")
}

func TestNewDarwinHelper_ReturnsNilWhenLANAccessEmpty(t *testing.T) {
	cfg := config.Network{LANAccess: []string{}}
	helper := NewDarwinHelper(cfg, runtime.PlatformMacOrbStack)
	assert.Nil(t, helper, "NewDarwinHelper should return nil when LANAccess is empty")
}

func TestNewDarwinHelper_ReturnsHelperWhenLANAccessPresent(t *testing.T) {
	cfg := config.Network{LANAccess: []string{"*"}}
	helper := NewDarwinHelper(cfg, runtime.PlatformMacOrbStack)
	assert.NotNil(t, helper, "NewDarwinHelper should return non-nil when LANAccess is configured")
}

func TestNewDarwinHelper_UsesPlatformForRuleset(t *testing.T) {
	// Verify the platform is reflected in behavior by creating a firewall
	// with Docker Desktop env and checking the generated ruleset uses
	// the Docker Desktop priority (filter - 1) rather than OrbStack (filter - 2).
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "/project", "", runtime.PlatformMacDockerDesktop)
	firewall := New(env)

	_, err := firewall.ApplyRules("container1", "172.17.0.2", nil)
	require.NoError(t, err)

	dir, _ := nftDirOnDarwin()
	content, err := afero.ReadFile(mockFs, dir+"/"+nftFileName("/project"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "priority filter - 1",
		"Docker Desktop platform should use priority filter - 1")
}

// =============================================================================
// Darwin Setup Tests
// =============================================================================

func TestDarwinSetup_ReturnsNoError(t *testing.T) {
	helper := newTestDarwinHelper(t)
	env := newTestDarwinNetworkEnv()

	action, err := helper.Setup(env, "/projects/test", nil)
	assert.NoError(t, err, "Setup should not error")
	assert.NotNil(t, action, "Setup should return a PostCommitAction")
}

func TestDarwinSetup_IsNoOp(t *testing.T) {
	helper := newTestDarwinHelper(t)
	env := newTestDarwinNetworkEnv()

	action, _ := helper.Setup(env, "/projects/test", nil)

	// PostCommitAction.Run should be nil (no-op)
	assert.Nil(t, action.Run, "Setup PostCommitAction.Run should be nil (no-op on darwin)")
}

// =============================================================================
// Darwin Teardown Tests
// =============================================================================

func TestDarwinTeardown_ReturnsNoError(t *testing.T) {
	helper := newTestDarwinHelper(t)
	env := newTestDarwinNetworkEnv()

	err := helper.Teardown(env, "/projects/test")
	assert.NoError(t, err, "Teardown should not error (no-op on darwin)")
}

// =============================================================================
// Darwin HelperStatus Tests
// =============================================================================

func TestDarwinHelperStatus_InstalledWhenContainerRunning(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, []byte("true\n"))
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	status := helper.HelperStatus(context.Background(), env)

	assert.True(t, status.Installed, "HelperStatus should report Installed when container is running")
}

func TestDarwinHelperStatus_NotInstalledWhenContainerNotRunning(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, []byte("false\n"))
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	status := helper.HelperStatus(context.Background(), env)

	assert.False(t, status.Installed, "HelperStatus should report not Installed when container is stopped")
}

func TestDarwinHelperStatus_NotInstalledWhenDockerFails(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectFailure("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, assert.AnError)
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	status := helper.HelperStatus(context.Background(), env)

	assert.False(t, status.Installed, "HelperStatus should report not Installed when docker inspect fails")
}

func TestDarwinHelperStatus_NeedsUpdateWhenScriptDiffers(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, []byte("true\n"))
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	// Write an old entry script so NeedsUpdate returns true
	home, _ := os.UserHomeDir()
	scriptPath := filepath.Join(home, vmhelper.HelperDir, "entry.sh")
	_ = mockFs.MkdirAll(filepath.Dir(scriptPath), 0755)
	_ = afero.WriteFile(mockFs, scriptPath, []byte("old script content"), 0755)

	helper := newTestDarwinHelper(t)
	status := helper.HelperStatus(context.Background(), env)

	assert.True(t, status.Installed, "HelperStatus should report Installed")
	assert.True(t, status.NeedsUpdate, "HelperStatus should report NeedsUpdate when entry script differs")
}

func TestDarwinHelperStatus_NoUpdateNeededWhenNotInstalled(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectFailure("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, assert.AnError)
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	status := helper.HelperStatus(context.Background(), env)

	assert.False(t, status.NeedsUpdate, "HelperStatus should not report NeedsUpdate when not installed")
}

// =============================================================================
// Darwin DetailedStatus Tests
// =============================================================================

func TestDarwinDetailedStatus_ReturnsEmptyWhenNoFiles(t *testing.T) {
	env := newTestDarwinNetworkEnv()
	helper := newTestDarwinHelper(t)

	info := helper.DetailedStatus(env)
	assert.Empty(t, info.RuleFiles, "DetailedStatus should return empty RuleFiles when directory doesn't exist")
}

func TestDarwinDetailedStatus_ListsNftFiles(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	nftDir, err := nftDirOnDarwin()
	require.NoError(t, err)
	_ = mockFs.MkdirAll(nftDir, 0755)
	_ = afero.WriteFile(mockFs, filepath.Join(nftDir, "alca-abc123.nft"), []byte("table inet alca-abc123 {}"), 0644)
	_ = afero.WriteFile(mockFs, filepath.Join(nftDir, "alca-def456.nft"), []byte("table inet alca-def456 {}"), 0644)

	helper := newTestDarwinHelper(t)
	info := helper.DetailedStatus(env)

	assert.Len(t, info.RuleFiles, 2, "DetailedStatus should list all .nft files")
}

func TestDarwinDetailedStatus_SkipsNonNftFiles(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	nftDir, err := nftDirOnDarwin()
	require.NoError(t, err)
	_ = mockFs.MkdirAll(nftDir, 0755)
	_ = afero.WriteFile(mockFs, filepath.Join(nftDir, "alca-abc123.nft"), []byte("rules"), 0644)
	_ = afero.WriteFile(mockFs, filepath.Join(nftDir, "readme.txt"), []byte("not a rule"), 0644)

	helper := newTestDarwinHelper(t)
	info := helper.DetailedStatus(env)

	assert.Len(t, info.RuleFiles, 1, "DetailedStatus should skip non-.nft files")
	assert.Equal(t, "alca-abc123.nft", info.RuleFiles[0].Name)
}

func TestDarwinDetailedStatus_SkipsDirectories(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	nftDir, err := nftDirOnDarwin()
	require.NoError(t, err)
	_ = mockFs.MkdirAll(filepath.Join(nftDir, "subdir.nft"), 0755)

	helper := newTestDarwinHelper(t)
	info := helper.DetailedStatus(env)

	assert.Empty(t, info.RuleFiles, "DetailedStatus should skip directories even if they end in .nft")
}

func TestDarwinDetailedStatus_ReadsFileContent(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	nftDir, err := nftDirOnDarwin()
	require.NoError(t, err)
	_ = mockFs.MkdirAll(nftDir, 0755)
	expectedContent := "table inet alca-test { chain forward { } }"
	_ = afero.WriteFile(mockFs, filepath.Join(nftDir, "alca-test.nft"), []byte(expectedContent), 0644)

	helper := newTestDarwinHelper(t)
	info := helper.DetailedStatus(env)

	require.Len(t, info.RuleFiles, 1)
	assert.Equal(t, expectedContent, info.RuleFiles[0].Content, "DetailedStatus should read file content")
}

// =============================================================================
// Darwin InstallHelper Tests
// =============================================================================

func TestDarwinInstallHelper_CreatesNftDirectory(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	_, err := helper.InstallHelper(env, nil)
	require.NoError(t, err)

	nftDir, _ := nftDirOnDarwin()
	exists, _ := afero.DirExists(mockFs, nftDir)
	assert.True(t, exists, "InstallHelper must create the nft directory")
}

func TestDarwinInstallHelper_ReturnsPostCommitAction(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	action, err := helper.InstallHelper(env, nil)

	require.NoError(t, err)
	assert.NotNil(t, action, "InstallHelper should return a PostCommitAction")
	assert.NotNil(t, action.Run, "InstallHelper PostCommitAction.Run should not be nil")
}

func TestDarwinInstallHelper_PostCommitCallsNetworkHelperInstall(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+vmhelper.ContainerName, []byte("true\n"))
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	action, err := helper.InstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	require.NoError(t, err)

	mockCmd.AssertCalled(t, "docker rm -f "+vmhelper.ContainerName)
}

func TestDarwinInstallHelper_AcceptsNilProgress(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	_, err := helper.InstallHelper(env, nil)
	assert.NoError(t, err, "InstallHelper should handle nil progress func")
}

func TestDarwinInstallHelper_CallsProgressFunc(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	called := false
	progress := func(format string, args ...any) {
		called = true
	}

	helper := newTestDarwinHelper(t)
	_, _ = helper.InstallHelper(env, progress)
	assert.True(t, called, "InstallHelper should call the progress function")
}

// =============================================================================
// Darwin UninstallHelper Tests
// =============================================================================

func TestDarwinUninstallHelper_ReturnsPostCommitAction(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	action, err := helper.UninstallHelper(env, nil)

	require.NoError(t, err)
	assert.NotNil(t, action, "UninstallHelper should return a PostCommitAction")
	assert.NotNil(t, action.Run, "UninstallHelper PostCommitAction.Run should not be nil")
}

func TestDarwinUninstallHelper_PostCommitCallsNetworkHelperUninstall(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	action, err := helper.UninstallHelper(env, nil)
	require.NoError(t, err)

	err = action.Run(context.Background(), nil)
	require.NoError(t, err)

	mockCmd.AssertCalled(t, "docker rm -f "+vmhelper.ContainerName)
}

func TestDarwinUninstallHelper_AcceptsNilProgress(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := shared.NewNetworkEnv(mockFs, mockCmd, "", "", runtime.PlatformMacOrbStack)

	helper := newTestDarwinHelper(t)
	_, err := helper.UninstallHelper(env, nil)
	assert.NoError(t, err, "UninstallHelper should handle nil progress func")
}

// =============================================================================
// Test Helpers
// =============================================================================

func newTestDarwinHelper(t *testing.T) shared.NetworkHelper {
	t.Helper()
	cfg := config.Network{LANAccess: []string{"*"}}
	helper := NewDarwinHelper(cfg, runtime.PlatformMacOrbStack)
	require.NotNil(t, helper)
	return helper
}

func newTestDarwinNetworkEnv() *shared.NetworkEnv {
	return shared.NewNetworkEnv(
		afero.NewMemMapFs(),
		util.NewMockCommandRunner(),
		"", "",

		runtime.PlatformMacOrbStack)

}
