package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProxyAddress_Valid(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantPort int
	}{
		{"192.168.1.1:1080", "192.168.1.1", 1080},
		{"10.0.0.1:8080", "10.0.0.1", 8080},
		{"127.0.0.1:3128", "127.0.0.1", 3128},
		{"172.17.0.1:1", "172.17.0.1", 1},
		{"172.17.0.1:65535", "172.17.0.1", 65535},
		{"[::1]:1080", "::1", 1080},
		{"[fe80::1]:8080", "fe80::1", 8080},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host, port, err := ParseProxyAddress(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

// TestParseProxyAddress_Invalid is covered by TestParseProxyAddress_SentinelErrors
// which asserts both error presence AND error type. Removed to avoid redundancy.

func TestValidateProxyAddress_WithAlcaTokens(t *testing.T) {
	// Alca tokens should be validated but not parsed as host:port
	err := ValidateProxyAddress("${alca:HOST_IP}:1080")
	assert.NoError(t, err)

	// Invalid alca token
	err = ValidateProxyAddress("${alca:INVALID_TOKEN}:1080")
	assert.Error(t, err)
}

func TestValidateProxyAddress_AlcaTokenEdgeCases(t *testing.T) {
	tests := []struct {
		input   string
		desc    string
		wantErr error
	}{
		{"${alca:HOST_IP}", "no port", ErrInvalidProxyFormat},
		{"${alca:HOST_IP}:abc", "non-numeric port", ErrProxyPortOutOfRange},
		{"${alca:HOST_IP}:0", "port zero", ErrProxyPortOutOfRange},
		{"${alca:HOST_IP}:65536", "port out of range", ErrProxyPortOutOfRange},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := ValidateProxyAddress(tt.input)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestValidateProxyAddress_Empty(t *testing.T) {
	err := ValidateProxyAddress("")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProxyFormat)
}

func TestParseProxyAddress_SentinelErrors(t *testing.T) {
	tests := []struct {
		input   string
		desc    string
		wantErr error
	}{
		{"", "empty string", ErrInvalidProxyFormat},
		{"192.168.1.1", "no port", ErrInvalidProxyFormat},
		{"just-a-string", "no host:port format", ErrInvalidProxyFormat},
		{"example.com:1080", "hostname not IP", ErrProxyHostNotIP},
		{"192.168.1.1:0", "port zero", ErrProxyPortOutOfRange},
		{"192.168.1.1:99999", "port out of range", ErrProxyPortOutOfRange},
		{"192.168.1.1:abc", "non-numeric port", ErrProxyPortOutOfRange},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, _, err := ParseProxyAddress(tt.input)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
