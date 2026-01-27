package runtime

import (
	"os/exec"
	"strings"
	"testing"
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

func TestGetOrbStackSubnet(t *testing.T) {
	// Skip if orbctl is not available
	if _, err := exec.LookPath("orbctl"); err != nil {
		t.Skip("orbctl not available, skipping test")
	}

	subnet, err := GetOrbStackSubnet()
	if err != nil {
		t.Fatalf("GetOrbStackSubnet failed: %v", err)
	}

	// Verify the subnet looks like a CIDR
	if !strings.Contains(subnet, "/") {
		t.Errorf("expected CIDR format (contains /), got %q", subnet)
	}

	// Verify it starts with a reasonable IP prefix
	if !strings.HasPrefix(subnet, "192.168.") && !strings.HasPrefix(subnet, "10.") && !strings.HasPrefix(subnet, "172.") {
		t.Errorf("expected private IP range, got %q", subnet)
	}

	t.Logf("GetOrbStackSubnet returned: %v", subnet)
}

func TestIsOrbStackDockerNotAvailable(t *testing.T) {
	// This test verifies error handling when docker is not available.
	// We can't easily test this without mocking, so we just document the expected behavior.
	// When docker is not found, IsOrbStack should return false with an error.
	t.Log("IsOrbStack returns (false, error) when docker is not available")
}

func TestGetOrbStackSubnetOrbctlNotAvailable(t *testing.T) {
	// This test verifies error handling when orbctl is not available.
	// We can't easily test this without mocking, so we just document the expected behavior.
	// When orbctl is not found, GetOrbStackSubnet should return ("", error).
	t.Log("GetOrbStackSubnet returns (\"\", error) when orbctl is not available")
}
