package vmhelper

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// DI Contract Tests
// =============================================================================

func TestNewVMHelperEnv_StoresInjectedDeps(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()

	env := NewVMHelperEnv(mockFs, mockCmd)

	assert.Equal(t, mockFs, env.Fs, "NewVMHelperEnv must store the exact Fs instance")
	assert.Equal(t, mockCmd, env.Cmd, "NewVMHelperEnv must store the exact Cmd instance")
}

// =============================================================================
// WriteEntryScript Tests
// =============================================================================

func TestWriteEntryScript_CreatesDirectories(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := WriteEntryScript(env)
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	nftDirPath := filepath.Join(home, shared.NftDirRel)
	helperDirPath := filepath.Join(home, HelperDir)

	exists, _ := afero.DirExists(mockFs, nftDirPath)
	assert.True(t, exists, "WriteEntryScript must create nft directory")

	exists, _ = afero.DirExists(mockFs, helperDirPath)
	assert.True(t, exists, "WriteEntryScript must create helper directory")
}

func TestWriteEntryScript_WritesEntryScript(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := WriteEntryScript(env)
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	scriptPath := filepath.Join(home, HelperDir, entryFileName)

	content, err := afero.ReadFile(mockFs, scriptPath)
	require.NoError(t, err)
	assert.Equal(t, entryScript, string(content), "WriteEntryScript must write the embedded entry script")
}

// =============================================================================
// InstallHelper Tests
// =============================================================================

func TestInstallHelper_RunsDockerCommands(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("true\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := InstallHelper(context.Background(), env, runtime.PlatformMacOrbStack, nil)
	require.NoError(t, err)

	// Should remove existing container
	mockCmd.AssertCalled(t, "docker rm -f "+ContainerName)

	// Should run docker run
	// These arg checks are intentional: --privileged, --pid=host, --net=host,
	// and volume mounts are security-critical flags that must be present for
	// the network helper container to function correctly.
	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "docker" && len(call.Args) > 0 && call.Args[0] == "run" {
			found = true
			// Verify security-critical flags
			args := call.Key
			assert.Contains(t, args, "--privileged")
			assert.Contains(t, args, "--pid=host")
			assert.Contains(t, args, "--net=host")
			assert.Contains(t, args, "--restart=always")
			assert.Contains(t, args, ContainerName)
			assert.Contains(t, args, "ALCA_PLATFORM=orbstack")
			break
		}
	}
	assert.True(t, found, "InstallHelper must call docker run")
}

func TestInstallHelper_SetsPlatformEnv(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	// ECI probe succeeds (no ECI)
	mockCmd.ExpectSuccess("docker run --rm --privileged --pid=host alpine:latest true", nil)
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("true\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := InstallHelper(context.Background(), env, runtime.PlatformMacDockerDesktop, nil)
	require.NoError(t, err)

	found := false
	for _, call := range mockCmd.Calls {
		if call.Name == "docker" && len(call.Args) > 1 && call.Args[0] == "run" && call.Args[1] == "-d" {
			assert.Contains(t, call.Key, "ALCA_PLATFORM=docker-desktop")
			found = true
			break
		}
	}
	assert.True(t, found, "InstallHelper must pass platform env to docker run")
}

func TestInstallHelper_DetectsECI_DockerDesktop(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	// ECI probe fails
	mockCmd.ExpectFailure("docker run --rm --privileged --pid=host alpine:latest true", assert.AnError)
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := InstallHelper(context.Background(), env, runtime.PlatformMacDockerDesktop, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Enhanced Container Isolation (ECI)")
	assert.Contains(t, err.Error(), "Disable ECI")
}

func TestInstallHelper_SkipsECICheck_OrbStack(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("true\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := InstallHelper(context.Background(), env, runtime.PlatformMacOrbStack, nil)
	require.NoError(t, err)

	// ECI probe should NOT have been called
	mockCmd.AssertNotCalled(t, "docker run --rm --privileged --pid=host alpine:latest true")
}

func TestInstallHelper_ECIPassesOnDockerDesktop(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	// ECI probe succeeds (no ECI)
	mockCmd.ExpectSuccess("docker run --rm --privileged --pid=host alpine:latest true", nil)
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("true\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := InstallHelper(context.Background(), env, runtime.PlatformMacDockerDesktop, nil)
	require.NoError(t, err)
}

func TestInstallHelper_FailsWhenContainerNotRunning(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	// docker inspect returns "false"
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("false\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := InstallHelper(context.Background(), env, runtime.PlatformMacOrbStack, nil)
	assert.Error(t, err, "InstallHelper should fail when container is not running after start")
	assert.Contains(t, err.Error(), "not running")
}

// =============================================================================
// UninstallHelper Tests
// =============================================================================

func TestUninstallHelper_RemovesContainer(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := UninstallHelper(context.Background(), env, nil)
	require.NoError(t, err)

	mockCmd.AssertCalled(t, "docker rm -f "+ContainerName)
}

func TestUninstallHelper_PropagatesError(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectFailure("docker rm -f "+ContainerName, assert.AnError)
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := UninstallHelper(context.Background(), env, nil)
	assert.Error(t, err)
}

// =============================================================================
// Reload Tests
// =============================================================================

func TestReload_SendsSIGHUP(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := Reload(context.Background(), env)
	require.NoError(t, err)

	mockCmd.AssertCalled(t, "docker exec "+ContainerName+" sh -c kill -HUP 1")
}

func TestReload_PropagatesError(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectFailure("docker exec "+ContainerName+" sh -c kill -HUP 1", assert.AnError)
	env := NewVMHelperEnv(mockFs, mockCmd)

	err := Reload(context.Background(), env)
	assert.Error(t, err)
}

// =============================================================================
// IsInstalled Tests
// =============================================================================

func TestIsInstalled_ReturnsTrueWhenRunning(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("true\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	installed, err := IsInstalled(context.Background(), env)
	require.NoError(t, err)
	assert.True(t, installed)
}

func TestIsInstalled_ReturnsFalseWhenStopped(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectSuccess("docker inspect --format {{.State.Running}} "+ContainerName, []byte("false\n"))
	env := NewVMHelperEnv(mockFs, mockCmd)

	installed, err := IsInstalled(context.Background(), env)
	require.NoError(t, err)
	assert.False(t, installed)
}

func TestIsInstalled_ReturnsFalseWhenNotFound(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	mockCmd.ExpectFailure("docker inspect --format {{.State.Running}} "+ContainerName, assert.AnError)
	env := NewVMHelperEnv(mockFs, mockCmd)

	installed, err := IsInstalled(context.Background(), env)
	require.NoError(t, err)
	assert.False(t, installed, "IsInstalled should return false when container doesn't exist")
}

// =============================================================================
// NeedsUpdate Tests
// =============================================================================

func TestNeedsUpdate_ReturnsTrueWhenFileNotExists(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := NewVMHelperEnv(mockFs, mockCmd)

	needs, err := NeedsUpdate(env)
	require.NoError(t, err)
	assert.True(t, needs, "NeedsUpdate should return true when entry.sh doesn't exist")
}

func TestNeedsUpdate_ReturnsFalseWhenUpToDate(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := NewVMHelperEnv(mockFs, mockCmd)

	home, _ := os.UserHomeDir()
	scriptPath := filepath.Join(home, HelperDir, entryFileName)
	_ = mockFs.MkdirAll(filepath.Dir(scriptPath), 0755)
	_ = afero.WriteFile(mockFs, scriptPath, []byte(entryScript), 0755)

	needs, err := NeedsUpdate(env)
	require.NoError(t, err)
	assert.False(t, needs, "NeedsUpdate should return false when entry.sh matches embedded")
}

func TestNeedsUpdate_ReturnsTrueWhenDifferent(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()
	env := NewVMHelperEnv(mockFs, mockCmd)

	home, _ := os.UserHomeDir()
	scriptPath := filepath.Join(home, HelperDir, entryFileName)
	_ = mockFs.MkdirAll(filepath.Dir(scriptPath), 0755)
	_ = afero.WriteFile(mockFs, scriptPath, []byte("old content"), 0755)

	needs, err := NeedsUpdate(env)
	require.NoError(t, err)
	assert.True(t, needs, "NeedsUpdate should return true when entry.sh differs from embedded")
}

// =============================================================================
// Embed Tests
// =============================================================================

func TestEntryScriptEmbedded(t *testing.T) {
	assert.NotEmpty(t, entryScript, "entry.sh must be embedded")
	assert.Contains(t, entryScript, "#!/bin/sh", "entry.sh must start with shebang")
	assert.Contains(t, entryScript, "alcatraz-network-helper", "entry.sh must reference the helper name")
}
