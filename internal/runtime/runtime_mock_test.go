package runtime

import (
	"errors"
	"runtime"
	"testing"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

// Helper to create RuntimeEnv with mock command runner.
func newMockEnv(mock *util.MockCommandRunner) *RuntimeEnv {
	return &RuntimeEnv{Cmd: mock}
}

// Test errors for mocking command failures.
var (
	errCommandNotFound  = errors.New("command not found")
	errDaemonNotRunning = errors.New("cannot connect to daemon")
)

// =============================================================================
// Available() Tests
// =============================================================================

func TestDockerAvailable_Success(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker version --format {{.Server.Version}}", []byte("24.0.0"))
	env := newMockEnv(mock)

	docker := NewDocker()
	if !docker.Available(env) {
		t.Error("Docker.Available() should return true when docker version succeeds")
	}

	mock.AssertCalled(t, "docker version --format {{.Server.Version}}")
}

func TestDockerAvailable_NotInstalled(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("docker version --format {{.Server.Version}}", errCommandNotFound)
	env := newMockEnv(mock)

	docker := NewDocker()
	if docker.Available(env) {
		t.Error("Docker.Available() should return false when docker not found")
	}
}

func TestDockerAvailable_DaemonNotRunning(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("docker version --format {{.Server.Version}}", errDaemonNotRunning)
	env := newMockEnv(mock)

	docker := NewDocker()
	if docker.Available(env) {
		t.Error("Docker.Available() should return false when daemon not running")
	}
}

func TestPodmanAvailable_Success(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("podman version --format {{.Version}}", []byte("4.5.0"))
	env := newMockEnv(mock)

	podman := NewPodman()
	if !podman.Available(env) {
		t.Error("Podman.Available() should return true when podman version succeeds")
	}

	mock.AssertCalled(t, "podman version --format {{.Version}}")
}

func TestPodmanAvailable_NotInstalled(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("podman version --format {{.Version}}", errCommandNotFound)
	env := newMockEnv(mock)

	podman := NewPodman()
	if podman.Available(env) {
		t.Error("Podman.Available() should return false when podman not found")
	}
}

// =============================================================================
// IsOrbStack() Tests
// =============================================================================

func TestIsOrbStack_True(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker info --format {{.OperatingSystem}}", []byte("OrbStack"))
	env := newMockEnv(mock)

	result, err := IsOrbStack(env)
	if err != nil {
		t.Fatalf("IsOrbStack() unexpected error: %v", err)
	}
	if !result {
		t.Error("IsOrbStack() should return true for OrbStack")
	}
}

func TestIsOrbStack_DockerDesktop(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker info --format {{.OperatingSystem}}", []byte("Docker Desktop"))
	env := newMockEnv(mock)

	result, err := IsOrbStack(env)
	if err != nil {
		t.Fatalf("IsOrbStack() unexpected error: %v", err)
	}
	if result {
		t.Error("IsOrbStack() should return false for Docker Desktop")
	}
}

func TestIsOrbStack_LinuxNative(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker info --format {{.OperatingSystem}}", []byte("Ubuntu 22.04.3 LTS"))
	env := newMockEnv(mock)

	result, err := IsOrbStack(env)
	if err != nil {
		t.Fatalf("IsOrbStack() unexpected error: %v", err)
	}
	if result {
		t.Error("IsOrbStack() should return false for native Linux")
	}
}

func TestIsOrbStack_DockerNotAvailable(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("docker info --format {{.OperatingSystem}}", errCommandNotFound)
	env := newMockEnv(mock)

	_, err := IsOrbStack(env)
	if err == nil {
		t.Error("IsOrbStack() should return error when docker not available")
	}
}

// =============================================================================
// IsRootlessPodman() Tests
// =============================================================================

func TestIsRootlessPodman_True(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("podman info --format {{.Host.Security.Rootless}}", []byte("true"))
	env := newMockEnv(mock)

	result, err := IsRootlessPodman(env)
	if err != nil {
		t.Fatalf("IsRootlessPodman() unexpected error: %v", err)
	}
	if !result {
		t.Error("IsRootlessPodman() should return true for rootless podman")
	}
}

func TestIsRootlessPodman_False(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("podman info --format {{.Host.Security.Rootless}}", []byte("false"))
	env := newMockEnv(mock)

	result, err := IsRootlessPodman(env)
	if err != nil {
		t.Fatalf("IsRootlessPodman() unexpected error: %v", err)
	}
	if result {
		t.Error("IsRootlessPodman() should return false for rootful podman")
	}
}

func TestIsRootlessPodman_WithWhitespace(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("podman info --format {{.Host.Security.Rootless}}", []byte("  true\n"))
	env := newMockEnv(mock)

	result, err := IsRootlessPodman(env)
	if err != nil {
		t.Fatalf("IsRootlessPodman() unexpected error: %v", err)
	}
	if !result {
		t.Error("IsRootlessPodman() should handle whitespace in output")
	}
}

func TestIsRootlessPodman_NotAvailable(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("podman info --format {{.Host.Security.Rootless}}", errCommandNotFound)
	env := newMockEnv(mock)

	_, err := IsRootlessPodman(env)
	if err == nil {
		t.Error("IsRootlessPodman() should return error when podman not available")
	}
}

// =============================================================================
// DetectPlatform() Tests
// =============================================================================

func TestDetectPlatform_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	mock := util.NewMockCommandRunner().AllowUnexpected()
	env := newMockEnv(mock)

	result := DetectPlatform(env)
	if result != PlatformLinux {
		t.Errorf("DetectPlatform() on Linux should return PlatformLinux, got %v", result)
	}
}

func TestDetectPlatform_MacOrbStack(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Test only runs on macOS")
	}

	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker info --format {{.OperatingSystem}}", []byte("OrbStack"))
	env := newMockEnv(mock)

	result := DetectPlatform(env)
	if result != PlatformMacOrbStack {
		t.Errorf("DetectPlatform() with OrbStack should return PlatformMacOrbStack, got %v", result)
	}
}

func TestDetectPlatform_MacDockerDesktop(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Test only runs on macOS")
	}

	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker info --format {{.OperatingSystem}}", []byte("Docker Desktop"))
	env := newMockEnv(mock)

	result := DetectPlatform(env)
	if result != PlatformMacDockerDesktop {
		t.Errorf("DetectPlatform() with Docker Desktop should return PlatformMacDockerDesktop, got %v", result)
	}
}

// =============================================================================
// ValidateMountExcludes() Tests
// =============================================================================

func TestValidateMountExcludes_DockerAlwaysAllowed(t *testing.T) {
	mock := util.NewMockCommandRunner().AllowUnexpected()
	env := newMockEnv(mock)

	docker := NewDocker()
	cfg := &config.Config{
		Mounts: []config.MountConfig{
			{Source: "/src", Target: "/app", Exclude: []string{"node_modules"}},
		},
	}

	err := ValidateMountExcludes(env, docker, cfg)
	if err != nil {
		t.Errorf("ValidateMountExcludes() should allow excludes with Docker, got: %v", err)
	}
}

func TestValidateMountExcludes_PodmanRootful(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("podman info --format {{.Host.Security.Rootless}}", []byte("false"))
	env := newMockEnv(mock)

	podman := NewPodman()
	cfg := &config.Config{
		Mounts: []config.MountConfig{
			{Source: "/src", Target: "/app", Exclude: []string{"node_modules"}},
		},
	}

	err := ValidateMountExcludes(env, podman, cfg)
	if err != nil {
		t.Errorf("ValidateMountExcludes() should allow excludes with rootful Podman, got: %v", err)
	}
}

func TestValidateMountExcludes_PodmanRootlessWithExcludes(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("podman info --format {{.Host.Security.Rootless}}", []byte("true"))
	env := newMockEnv(mock)

	podman := NewPodman()
	cfg := &config.Config{
		Mounts: []config.MountConfig{
			{Source: "/src", Target: "/app", Exclude: []string{"node_modules"}},
		},
	}

	err := ValidateMountExcludes(env, podman, cfg)
	if err != ErrRootlessPodmanExcludes {
		t.Errorf("ValidateMountExcludes() should return ErrRootlessPodmanExcludes, got: %v", err)
	}
}

func TestValidateMountExcludes_PodmanRootlessNoExcludes(t *testing.T) {
	mock := util.NewMockCommandRunner().AllowUnexpected()
	env := newMockEnv(mock)

	podman := NewPodman()
	cfg := &config.Config{
		Mounts: []config.MountConfig{
			{Source: "/src", Target: "/app"}, // No excludes
		},
	}

	err := ValidateMountExcludes(env, podman, cfg)
	if err != nil {
		t.Errorf("ValidateMountExcludes() should allow rootless Podman without excludes, got: %v", err)
	}
}

func TestValidateMountExcludes_PodmanInfoError(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("podman info --format {{.Host.Security.Rootless}}", errCommandNotFound)
	env := newMockEnv(mock)

	podman := NewPodman()
	cfg := &config.Config{
		Mounts: []config.MountConfig{
			{Source: "/src", Target: "/app", Exclude: []string{"node_modules"}},
		},
	}

	// Should fail open when we can't determine rootless status
	err := ValidateMountExcludes(env, podman, cfg)
	if err != nil {
		t.Errorf("ValidateMountExcludes() should fail open on podman info error, got: %v", err)
	}
}

// =============================================================================
// ShouldUseMutagen() Tests
// =============================================================================

func TestShouldUseMutagen_DockerDesktopAlways(t *testing.T) {
	// Docker Desktop always uses Mutagen for performance
	if !ShouldUseMutagen(PlatformMacDockerDesktop, false) {
		t.Error("ShouldUseMutagen(DockerDesktop, false) should return true")
	}
	if !ShouldUseMutagen(PlatformMacDockerDesktop, true) {
		t.Error("ShouldUseMutagen(DockerDesktop, true) should return true")
	}
}

func TestShouldUseMutagen_OrbStackOnlyWithExcludes(t *testing.T) {
	// OrbStack only uses Mutagen when excludes needed
	if ShouldUseMutagen(PlatformMacOrbStack, false) {
		t.Error("ShouldUseMutagen(OrbStack, false) should return false")
	}
	if !ShouldUseMutagen(PlatformMacOrbStack, true) {
		t.Error("ShouldUseMutagen(OrbStack, true) should return true")
	}
}

func TestShouldUseMutagen_LinuxOnlyWithExcludes(t *testing.T) {
	// Linux only uses Mutagen when excludes needed
	if ShouldUseMutagen(PlatformLinux, false) {
		t.Error("ShouldUseMutagen(Linux, false) should return false")
	}
	if !ShouldUseMutagen(PlatformLinux, true) {
		t.Error("ShouldUseMutagen(Linux, true) should return true")
	}
}

// =============================================================================
// Status() Tests - Container State Parsing
// =============================================================================

func TestDockerStatus_Running(t *testing.T) {
	mock := util.NewMockCommandRunner()
	// First call: find by label (returns container name)
	mock.ExpectSuccess(
		"docker ps -a --filter label=alca.project.id=test-uuid --format {{.Names}}",
		[]byte("alca-test"),
	)
	// Second call: inspect the container (5 fields: Status|Id|Name|Image|StartedAt)
	mock.ExpectSuccess(
		"docker inspect --format {{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}} alca-test",
		[]byte("running|abc123|/alca-test|test-image:latest|2024-01-15T10:00:00Z"),
	)
	env := newMockEnv(mock)

	docker := NewDocker()
	st := &state.State{
		ProjectID:     "test-uuid",
		ContainerName: "alca-test",
	}

	status, err := docker.Status(env, "/project", st)
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}

	if status.State != StateRunning {
		t.Errorf("Status().State = %v, want StateRunning", status.State)
	}
	if status.ID != "abc123" {
		t.Errorf("Status().ID = %v, want abc123", status.ID)
	}
}

func TestDockerStatus_Stopped(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess(
		"docker ps -a --filter label=alca.project.id=test-uuid --format {{.Names}}",
		[]byte("alca-test"),
	)
	mock.ExpectSuccess(
		"docker inspect --format {{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}} alca-test",
		[]byte("exited|abc123|/alca-test|test-image:latest|2024-01-15T10:00:00Z"),
	)
	env := newMockEnv(mock)

	docker := NewDocker()
	st := &state.State{
		ProjectID:     "test-uuid",
		ContainerName: "alca-test",
	}

	status, err := docker.Status(env, "/project", st)
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}

	if status.State != StateStopped {
		t.Errorf("Status().State = %v, want StateStopped", status.State)
	}
}

func TestDockerStatus_NotFound(t *testing.T) {
	mock := util.NewMockCommandRunner()
	// Label search returns empty (no container with this label)
	mock.ExpectSuccess(
		"docker ps -a --filter label=alca.project.id=test-uuid --format {{.Names}}",
		[]byte(""),
	)
	// Fallback to name-based lookup also fails
	mock.Expect(
		"docker inspect --format {{.State.Status}}|{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.StartedAt}} alca-test",
		[]byte("Error: No such container: alca-test"),
		errors.New("no such container"),
	)
	env := newMockEnv(mock)

	docker := NewDocker()
	st := &state.State{
		ProjectID:     "test-uuid",
		ContainerName: "alca-test",
	}

	status, err := docker.Status(env, "/project", st)
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}

	if status.State != StateNotFound {
		t.Errorf("Status().State = %v, want StateNotFound", status.State)
	}
}

func TestDockerStatus_NilState(t *testing.T) {
	mock := util.NewMockCommandRunner().AllowUnexpected()
	env := newMockEnv(mock)

	docker := NewDocker()
	status, err := docker.Status(env, "/project", nil)
	if err != nil {
		t.Fatalf("Status() with nil state unexpected error: %v", err)
	}

	if status.State != StateNotFound {
		t.Errorf("Status() with nil state should return StateNotFound, got %v", status.State)
	}
}
