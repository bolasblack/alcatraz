package config

import (
	"errors"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test: proxy field is loaded from TOML
func TestLoadConfig_ProxyField(t *testing.T) {
	content := `
image = "alpine"

[network]
proxy = "10.0.0.1:1080"
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	require.NoError(t, afero.WriteFile(memFs, path, []byte(content), 0644))

	cfg, err := LoadConfig(env, path, noExpandEnv)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1:1080", cfg.Network.Proxy)
}

// Test: proxy field with alca token passes validation
func TestLoadConfig_ProxyWithAlcaToken(t *testing.T) {
	content := `
image = "alpine"

[network]
proxy = "${alca:HOST_IP}:1080"
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	require.NoError(t, afero.WriteFile(memFs, path, []byte(content), 0644))

	cfg, err := LoadConfig(env, path, noExpandEnv)
	require.NoError(t, err)
	assert.Equal(t, "${alca:HOST_IP}:1080", cfg.Network.Proxy)
}

// Test: invalid proxy address is rejected at load time
func TestLoadConfig_ProxyInvalidAddress(t *testing.T) {
	content := `
image = "alpine"

[network]
proxy = "not-an-ip:1080"
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	require.NoError(t, afero.WriteFile(memFs, path, []byte(content), 0644))

	_, err := LoadConfig(env, path, noExpandEnv)
	assert.Error(t, err)
	// LoadConfig wraps with "network.proxy: ..." context, so use errors.Is for sentinel
	assert.True(t, errors.Is(err, ErrProxyHostNotIP), "expected ErrProxyHostNotIP, got: %v", err)
	// Also verify the wrapping adds path context
	assert.Contains(t, err.Error(), "network.proxy")
}

// Test: proxy field is empty by default
func TestLoadConfig_ProxyDefaultEmpty(t *testing.T) {
	content := `
image = "alpine"
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	require.NoError(t, afero.WriteFile(memFs, path, []byte(content), 0644))

	cfg, err := LoadConfig(env, path, noExpandEnv)
	require.NoError(t, err)
	assert.Empty(t, cfg.Network.Proxy)
}

// Test: proxy in overlay overrides base during merge
func TestMergeConfigs_ProxyOverlayWins(t *testing.T) {
	// Use includes to test merge: base extends, overlay includes
	env, memFs := newTestEnv(t)

	basePath := "/test/base.toml"
	require.NoError(t, afero.WriteFile(memFs, basePath, []byte(`
image = "alpine"
[network]
proxy = "10.0.0.1:1080"
`), 0644))

	mainPath := "/test/.alca.toml"
	require.NoError(t, afero.WriteFile(memFs, mainPath, []byte(`
extends = ["./base.toml"]
[network]
proxy = "10.0.0.2:3128"
`), 0644))

	cfg, err := LoadWithIncludes(env, mainPath, noExpandEnv)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.2:3128", cfg.Network.Proxy)
}

// Test: empty overlay proxy preserves base
func TestMergeConfigs_ProxyEmptyOverlayPreservesBase(t *testing.T) {
	env, memFs := newTestEnv(t)

	basePath := "/test/base.toml"
	require.NoError(t, afero.WriteFile(memFs, basePath, []byte(`
image = "alpine"
[network]
proxy = "10.0.0.1:1080"
`), 0644))

	mainPath := "/test/.alca.toml"
	require.NoError(t, afero.WriteFile(memFs, mainPath, []byte(`
extends = ["./base.toml"]
`), 0644))

	cfg, err := LoadWithIncludes(env, mainPath, noExpandEnv)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1:1080", cfg.Network.Proxy)
}
