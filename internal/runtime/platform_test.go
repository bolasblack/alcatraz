package runtime

import (
	"testing"
)

func TestRuntimePlatformConstants(t *testing.T) {
	// Verify platform constants are defined correctly
	tests := []struct {
		platform RuntimePlatform
		expected string
	}{
		{PlatformLinux, "linux"},
		{PlatformMacDockerDesktop, "docker-desktop"},
		{PlatformMacOrbStack, "orbstack"},
	}

	for _, tt := range tests {
		t.Run(string(tt.platform), func(t *testing.T) {
			if string(tt.platform) != tt.expected {
				t.Errorf("platform constant %v = %q, expected %q", tt.platform, string(tt.platform), tt.expected)
			}
		})
	}
}

// DetectPlatform and IsOrbStack tests: see runtime_mock_test.go (mock-based, deterministic).
