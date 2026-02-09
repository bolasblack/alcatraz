package nft

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// hasIncludeLineOnLinux Tests
// =============================================================================

func TestHasIncludeLineOnLinux_ReturnsTrueWhenPresent(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\n\n" + alcatrazIncludeLineOnLinux + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	assert.True(t, h.hasIncludeLineOnLinux(fs), "should detect existing include line")
}

func TestHasIncludeLineOnLinux_ReturnsFalseWhenAbsent(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\n\ntable inet filter {\n}\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	assert.False(t, h.hasIncludeLineOnLinux(fs), "should return false when include line is absent")
}

func TestHasIncludeLineOnLinux_ReturnsFalseWhenFileMissing(t *testing.T) {
	fs := afero.NewMemMapFs()

	h := &nftLinuxHelper{}
	assert.False(t, h.hasIncludeLineOnLinux(fs), "should return false when nftables.conf doesn't exist")
}

func TestHasIncludeLineOnLinux_ReturnsFalseForPartialMatch(t *testing.T) {
	fs := afero.NewMemMapFs()
	// A similar but not exact include line
	content := `include "/etc/nftables.d/other/*.nft"` + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	assert.False(t, h.hasIncludeLineOnLinux(fs), "should not match a different include line")
}

// =============================================================================
// addIncludeLineOnLinux Tests
// =============================================================================

func TestAddIncludeLineOnLinux_AppendsToExistingFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	existing := "#!/usr/sbin/nft -f\n\ntable inet filter {\n}\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(existing), 0644))

	h := &nftLinuxHelper{}
	err := h.addIncludeLineOnLinux(fs)
	require.NoError(t, err)

	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	assert.Contains(t, string(content), alcatrazIncludeLineOnLinux,
		"should contain include line after adding")
	// Original content should be preserved
	assert.Contains(t, string(content), "table inet filter",
		"should preserve original content")
}

func TestAddIncludeLineOnLinux_CreatesFileWhenMissing(t *testing.T) {
	fs := afero.NewMemMapFs()

	h := &nftLinuxHelper{}
	err := h.addIncludeLineOnLinux(fs)
	require.NoError(t, err)

	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	assert.Contains(t, string(content), "#!/usr/sbin/nft -f",
		"newly created file should have shebang")
	assert.Contains(t, string(content), alcatrazIncludeLineOnLinux,
		"newly created file should have include line")
}

func TestAddIncludeLineOnLinux_HandlesNoTrailingNewline(t *testing.T) {
	fs := afero.NewMemMapFs()
	// File without trailing newline
	existing := "#!/usr/sbin/nft -f\ntable inet filter { }"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(existing), 0644))

	h := &nftLinuxHelper{}
	err := h.addIncludeLineOnLinux(fs)
	require.NoError(t, err)

	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	s := string(content)
	// The include line should appear on its own line (not concatenated with previous content)
	assert.Contains(t, s, "{ }\n"+alcatrazIncludeLineOnLinux,
		"should add newline before include line when original has no trailing newline")
}

func TestAddIncludeLineOnLinux_EndsWithNewline(t *testing.T) {
	fs := afero.NewMemMapFs()
	existing := "#!/usr/sbin/nft -f\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(existing), 0644))

	h := &nftLinuxHelper{}
	require.NoError(t, h.addIncludeLineOnLinux(fs))

	content, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	assert.True(t, len(content) > 0 && content[len(content)-1] == '\n',
		"file should end with newline")
}

// =============================================================================
// removeIncludeLineOnLinux Tests
// =============================================================================

func TestRemoveIncludeLineOnLinux_RemovesTheLine(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\n\n" + alcatrazIncludeLineOnLinux + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	err := h.removeIncludeLineOnLinux(fs)
	require.NoError(t, err)

	result, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	assert.NotContains(t, string(result), alcatrazIncludeLineOnLinux,
		"include line should be removed")
	// Original content should remain
	assert.Contains(t, string(result), "#!/usr/sbin/nft -f",
		"should preserve other lines")
}

func TestRemoveIncludeLineOnLinux_HandlesFileMissing(t *testing.T) {
	fs := afero.NewMemMapFs()

	h := &nftLinuxHelper{}
	err := h.removeIncludeLineOnLinux(fs)
	assert.NoError(t, err, "should not error when file doesn't exist")
}

func TestRemoveIncludeLineOnLinux_HandlesFileWithoutIncludeLine(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\ntable inet filter {\n}\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	err := h.removeIncludeLineOnLinux(fs)
	require.NoError(t, err)

	result, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	// Content should be preserved
	assert.Contains(t, string(result), "table inet filter",
		"should preserve existing content when include line was not present")
}

func TestRemoveIncludeLineOnLinux_PreservesOtherIncludes(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\n" +
		`include "/etc/nftables.d/other/*.nft"` + "\n" +
		alcatrazIncludeLineOnLinux + "\n" +
		"table inet filter {\n}\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	err := h.removeIncludeLineOnLinux(fs)
	require.NoError(t, err)

	result, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	s := string(result)
	assert.NotContains(t, s, alcatrazIncludeLineOnLinux,
		"alcatraz include line should be removed")
	assert.Contains(t, s, `include "/etc/nftables.d/other/*.nft"`,
		"other include lines should be preserved")
	assert.Contains(t, s, "table inet filter",
		"other content should be preserved")
}

func TestRemoveIncludeLineOnLinux_ResultEndsWithNewline(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := "#!/usr/sbin/nft -f\n" + alcatrazIncludeLineOnLinux + "\n"
	require.NoError(t, afero.WriteFile(fs, nftablesConfPathOnLinux, []byte(content), 0644))

	h := &nftLinuxHelper{}
	require.NoError(t, h.removeIncludeLineOnLinux(fs))

	result, err := afero.ReadFile(fs, nftablesConfPathOnLinux)
	require.NoError(t, err)

	assert.True(t, len(result) > 0 && result[len(result)-1] == '\n',
		"file should end with newline after removal")
}
