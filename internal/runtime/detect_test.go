package runtime

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/bolasblack/alcatraz/internal/config"
)

func TestIsOrbStack(t *testing.T) {
	// Skip if Docker is not available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available, skipping test")
	}

	result, err := IsOrbStack()
	if err != nil {
		// If Docker is installed but not running, skip
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			t.Skip("docker daemon not running, skipping test")
		}
		t.Fatalf("IsOrbStack failed: %v", err)
	}

	// We can't assert the result since it depends on the environment.
	// Just verify that the function returns without error and returns a boolean.
	t.Logf("IsOrbStack returned: %v", result)
}

func TestIsOrbStackDockerNotAvailable(t *testing.T) {
	// This test verifies error handling when docker is not available.
	// We can't easily test this without mocking, so we just document the expected behavior.
	// When docker is not found, IsOrbStack should return false with an error.
	t.Log("IsOrbStack returns (false, error) when docker is not available")
}

func TestAll(t *testing.T) {
	runtimes := All()
	if len(runtimes) != 2 {
		t.Errorf("expected 2 runtimes, got %d", len(runtimes))
	}

	names := make(map[string]bool)
	for _, rt := range runtimes {
		names[rt.Name()] = true
	}

	if !names["Docker"] {
		t.Error("expected Docker runtime in All()")
	}
	if !names["Podman"] {
		t.Error("expected Podman runtime in All()")
	}
}

func TestByName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Docker", true},
		{"Podman", true},
		{"Unknown", false},
		{"docker", false}, // case sensitive
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := ByName(tt.name)
			if (rt != nil) != tt.expected {
				t.Errorf("ByName(%q) returned %v, expected found=%v", tt.name, rt, tt.expected)
			}
			if rt != nil && rt.Name() != tt.name {
				t.Errorf("ByName(%q).Name() = %q, expected %q", tt.name, rt.Name(), tt.name)
			}
		})
	}
}

func TestDockerName(t *testing.T) {
	d := NewDocker()
	if d.Name() != "Docker" {
		t.Errorf("expected Docker, got %s", d.Name())
	}
}

func TestPodmanName(t *testing.T) {
	p := NewPodman()
	if p.Name() != "Podman" {
		t.Errorf("expected Podman, got %s", p.Name())
	}
}

func TestSelectRuntimeWithDockerConfig(t *testing.T) {
	// Skip if Docker is not available
	docker := NewDocker()
	if !docker.Available() {
		t.Skip("docker not available, skipping test")
	}

	cfg := &config.Config{
		Runtime: "docker",
	}

	rt, err := SelectRuntime(cfg)
	if err != nil {
		t.Fatalf("SelectRuntime failed: %v", err)
	}

	if rt.Name() != "Docker" {
		t.Errorf("expected Docker runtime, got %s", rt.Name())
	}
}

func TestSelectRuntimeWithAutoConfig(t *testing.T) {
	cfg := &config.Config{
		Runtime: "auto",
	}

	rt, err := SelectRuntime(cfg)
	if err != nil {
		// No runtime available is acceptable
		t.Logf("SelectRuntime returned error (no runtime available): %v", err)
		return
	}

	// Should return a valid runtime
	if rt.Name() != "Docker" && rt.Name() != "Podman" {
		t.Errorf("unexpected runtime: %s", rt.Name())
	}
	t.Logf("SelectRuntime returned: %s", rt.Name())
}

func TestSelectRuntimeDockerNotAvailable(t *testing.T) {
	// Skip if Docker IS available (we want to test the error case)
	docker := NewDocker()
	if docker.Available() {
		t.Skip("docker is available, cannot test error case")
	}

	cfg := &config.Config{
		Runtime: "docker",
	}

	_, err := SelectRuntime(cfg)
	if err == nil {
		t.Error("expected error when Docker not available")
	}

	if !strings.Contains(err.Error(), "Docker not available") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseContainerState(t *testing.T) {
	tests := []struct {
		input    string
		expected ContainerState
	}{
		{"running", StateRunning},
		{"exited", StateStopped},
		{"stopped", StateStopped},
		{"unknown", StateUnknown},
		{"", StateUnknown},
		{"other", StateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseContainerState(tt.input)
			if result != tt.expected {
				t.Errorf("parseContainerState(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContainsNoSuchContainer(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"No such container", true},
		{"no such container", true},
		{"NO SUCH CONTAINER", true},
		{"Error: No such container: test", true},
		{"Container not found", false},
		{"", false},
		{"some other error", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := containsNoSuchContainer(tt.input)
			if result != tt.expected {
				t.Errorf("containsNoSuchContainer(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are defined correctly
	if KeepAliveCommand != "sleep" {
		t.Errorf("KeepAliveCommand = %q, expected 'sleep'", KeepAliveCommand)
	}
	if KeepAliveArg != "infinity" {
		t.Errorf("KeepAliveArg = %q, expected 'infinity'", KeepAliveArg)
	}
	if EnvDebug != "ALCA_DEBUG" {
		t.Errorf("EnvDebug = %q, expected 'ALCA_DEBUG'", EnvDebug)
	}
}
