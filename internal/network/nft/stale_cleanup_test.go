package nft

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/transact"
	"github.com/bolasblack/alcatraz/internal/util"
)

// TestCleanupStaleFiles_DirRenameWithTransactFs verifies that CleanupStaleFiles
// detects stale files when the old project dir no longer exists on the underlying
// fs. TransactFs reads through for paths never written, so isStaleProject works
// correctly with the standard Fs field.
func TestCleanupStaleFiles_DirRenameWithTransactFs(t *testing.T) {
	actualFs := afero.NewMemMapFs()
	dir := nftDirOnLinux()
	_ = actualFs.MkdirAll(dir, 0755)

	projectID := "test-uuid-1234"
	oldProjectDir := "/path/old-name"

	// Old nft file on "disk" from previous run
	oldRuleset := generateRuleset("alca-old123", "172.17.0.2", nil, "filter - 1", oldProjectDir, projectID)
	_ = afero.WriteFile(actualFs, dir+"/"+nftFileName(oldProjectDir), []byte(oldRuleset), 0644)

	// Old dir does NOT exist (user renamed it)

	// Fs=TransactFs (for staged writes); TransactFs reads through to actualFs
	// for paths never written, so isStaleProject sees the real filesystem state.
	tfs := transact.New(transact.WithActualFs(actualFs))
	env := shared.NewNetworkEnv(tfs, util.NewMockCommandRunner(), "/path/new-name", "", runtime.PlatformLinux)
	env.ProjectID = projectID
	n := New(env).(*NFTables)

	count, err := n.CleanupStaleFiles()
	if err != nil {
		t.Fatalf("CleanupStaleFiles() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupStaleFiles() = %d, want 1", count)
	}

	exists, _ := afero.Exists(tfs, dir+"/"+nftFileName(oldProjectDir))
	if exists {
		t.Error("old nft file should be staged for removal")
	}
}

// TestCleanupStaleFiles_RunsIndependentOfLANAccessRules verifies that stale file
// cleanup works regardless of what lan-access rules the project has configured.
// This is the regression test for the bug where CleanupStaleFiles was only called
// inside setupFirewall, which has early returns for HasAllLAN and TypeNone.
func TestCleanupStaleFiles_RunsIndependentOfLANAccessRules(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	dir := nftDirOnLinux()
	_ = mockFs.MkdirAll(dir, 0755)

	// Stale project: directory no longer exists
	staleDir := "/home/user/deleted-project"
	staleRuleset := generateRuleset("alca-stale", "172.17.0.2", nil, "filter - 1", staleDir, "stale-uuid")
	_ = afero.WriteFile(mockFs, dir+"/"+nftFileName(staleDir), []byte(staleRuleset), 0644)

	// Active project with lan-access = ["*"] (HasAllLAN=true)
	activeDir := "/home/user/active-project"
	_ = mockFs.MkdirAll(activeDir+"/.alca", 0755)
	_ = afero.WriteFile(mockFs, activeDir+"/.alca/state.json",
		[]byte(`{"project_id":"active-uuid"}`), 0644)
	activeRuleset := generateRuleset("alca-active", "172.17.0.3", nil, "filter - 1", activeDir, "active-uuid")
	_ = afero.WriteFile(mockFs, dir+"/"+nftFileName(activeDir), []byte(activeRuleset), 0644)

	// CleanupStaleFiles operates on the firewall instance, not on lan-access rules.
	// Even if the calling project uses lan-access=["*"], cleanup still runs.
	env := shared.NewNetworkEnv(mockFs, util.NewMockCommandRunner(), activeDir, "", runtime.PlatformLinux)
	env.ProjectID = "active-uuid"
	n := New(env).(*NFTables)

	count, err := n.CleanupStaleFiles()
	if err != nil {
		t.Fatalf("CleanupStaleFiles() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupStaleFiles() = %d, want 1 (stale file removed)", count)
	}

	// Stale file should be removed
	exists, _ := afero.Exists(mockFs, dir+"/"+nftFileName(staleDir))
	if exists {
		t.Error("stale nft file should be removed")
	}

	// Active file should be kept
	exists, _ = afero.Exists(mockFs, dir+"/"+nftFileName(activeDir))
	if !exists {
		t.Error("active nft file should be kept")
	}
}

// TestCleanupStaleFiles_TwoFilesForSameProjectID verifies that when two nft files
// exist for the same project ID (old path + new path), the old one is cleaned up.
func TestCleanupStaleFiles_TwoFilesForSameProjectID(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	dir := nftDirOnLinux()
	_ = mockFs.MkdirAll(dir, 0755)

	projectID := "shared-uuid"
	oldDir := "/home/user/old-name"
	newDir := "/home/user/new-name"

	// Old nft file (project dir no longer exists)
	oldRuleset := generateRuleset("alca-old", "172.17.0.2", nil, "filter - 1", oldDir, projectID)
	_ = afero.WriteFile(mockFs, dir+"/"+nftFileName(oldDir), []byte(oldRuleset), 0644)

	// New nft file (project dir exists with matching state)
	newRuleset := generateRuleset("alca-new", "172.17.0.3", nil, "filter - 1", newDir, projectID)
	_ = afero.WriteFile(mockFs, dir+"/"+nftFileName(newDir), []byte(newRuleset), 0644)
	_ = mockFs.MkdirAll(newDir+"/.alca", 0755)
	_ = afero.WriteFile(mockFs, newDir+"/.alca/state.json",
		[]byte(fmt.Sprintf(`{"project_id":"%s"}`, projectID)), 0644)

	env := shared.NewNetworkEnv(mockFs, util.NewMockCommandRunner(), newDir, "", runtime.PlatformLinux)
	env.ProjectID = projectID
	n := New(env).(*NFTables)

	count, err := n.CleanupStaleFiles()
	if err != nil {
		t.Fatalf("CleanupStaleFiles() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupStaleFiles() = %d, want 1 (only old file should be cleaned)", count)
	}

	exists, _ := afero.Exists(mockFs, dir+"/"+nftFileName(oldDir))
	if exists {
		t.Error("old nft file should be removed")
	}

	exists, _ = afero.Exists(mockFs, dir+"/"+nftFileName(newDir))
	if !exists {
		t.Error("new nft file should be kept")
	}
}
