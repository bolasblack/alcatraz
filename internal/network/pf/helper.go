//go:build darwin

package pf

import (
	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// pfHelper implements shared.NetworkHelper for macOS using pf.
type pfHelper struct{}

// Compile-time interface assertion.
var _ shared.NetworkHelper = (*pfHelper)(nil)

func (p *pfHelper) Setup(env *shared.NetworkEnv, projectDir string, _ shared.ProgressFunc) (*shared.PostCommitAction, error) {
	// Ensure _shared file exists (empty - NAT rules are generated per-container in project files)
	// The _shared file must exist for file ordering: project files (e.g., "-Users-foo") sort
	// before "_shared" in ASCII, so project-specific rules take precedence.
	if err := p.writeSharedRule(env, "# Shared rules (NAT handled per-container in project files)\n"); err != nil {
		return nil, err
	}

	// Create project file
	content := "# Project: " + projectDir + "\n"
	if err := p.writeProjectFile(env, projectDir, content); err != nil {
		return nil, err
	}

	return &shared.PostCommitAction{}, nil
}

func (p *pfHelper) Teardown(env *shared.NetworkEnv, projectDir string) error {
	removeShared, err := p.deleteProjectFile(env, projectDir)
	if err != nil {
		return err
	}

	if removeShared {
		if err := p.deleteSharedRule(env); err != nil {
			return err
		}
	}
	return nil
}

func (p *pfHelper) HelperStatus(env *shared.NetworkEnv) shared.HelperStatus {
	return shared.HelperStatus{
		Installed:   p.isHelperInstalled(env),
		NeedsUpdate: p.isHelperNeedsUpdate(env),
	}
}

func (p *pfHelper) DetailedStatus(env *shared.NetworkEnv) shared.DetailedStatusInfo {
	info := shared.DetailedStatusInfo{
		DaemonLoaded: p.isLaunchDaemonLoaded(env),
	}

	if !fileExists(env, pfAnchorDir) {
		return info
	}

	entries, err := afero.ReadDir(env.Fs, pfAnchorDir)
	if err != nil {
		return info
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := afero.ReadFile(env.Fs, pfAnchorDir+"/"+entry.Name())
		if err != nil || len(content) == 0 {
			continue
		}
		info.RuleFiles = append(info.RuleFiles, shared.RuleFileInfo{
			Name:    entry.Name(),
			Content: string(content),
		})
	}

	return info
}

func (p *pfHelper) InstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	if err := p.installHelper(env, progress); err != nil {
		return nil, err
	}

	return &shared.PostCommitAction{
		Run: func(progress shared.ProgressFunc) error {
			return p.activateLaunchDaemon(env, progress)
		},
	}, nil
}

func (p *pfHelper) UninstallHelper(env *shared.NetworkEnv, progress shared.ProgressFunc) (*shared.PostCommitAction, error) {
	progress = shared.SafeProgress(progress)

	for _, warn := range p.uninstallHelper(env, progress) {
		progress("Warning: %v\n", warn)
	}

	return &shared.PostCommitAction{
		Run: func(progress shared.ProgressFunc) error {
			return p.flushPfRulesAfterUninstall(env, progress)
		},
	}, nil
}
