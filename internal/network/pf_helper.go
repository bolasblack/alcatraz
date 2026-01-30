package network

import (
	"fmt"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

type pfHelper struct{}

func newPfHelper() *pfHelper {
	return &pfHelper{}
}

func (p *pfHelper) Setup(env *util.Env, projectDir string, progress ProgressFunc) (*PostCommitAction, error) {
	progress = safeProgress(progress)

	// Check if shared rule needs update (new interfaces)
	needsUpdate, _, err := p.needsRuleUpdate(env)
	if err != nil {
		return nil, err
	}

	if needsUpdate {
		progress("Detecting OrbStack subnet...\n")
		subnet, err := p.getOrbStackSubnet(env)
		if err != nil {
			return nil, fmt.Errorf("failed to get OrbStack subnet: %w", err)
		}

		progress("Getting physical interfaces...\n")
		interfaces, err := p.getPhysicalInterfaces(env)
		if err != nil {
			return nil, fmt.Errorf("failed to get interfaces: %w", err)
		}

		rules := p.generateNATRules(subnet, interfaces)
		if err := p.writeSharedRule(env, rules); err != nil {
			return nil, err
		}
	}

	// Create project file
	content := "# Project: " + projectDir + "\n"
	if err := p.writeProjectFile(env, projectDir, content); err != nil {
		return nil, err
	}

	return &PostCommitAction{}, nil
}

func (p *pfHelper) Teardown(env *util.Env, projectDir string) error {
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

func (p *pfHelper) HelperStatus(env *util.Env) HelperStatus {
	return HelperStatus{
		Installed:   p.isHelperInstalled(env),
		NeedsUpdate: p.isHelperNeedsUpdate(env),
	}
}

func (p *pfHelper) DetailedStatus(env *util.Env) DetailedStatusInfo {
	info := DetailedStatusInfo{
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
		info.RuleFiles = append(info.RuleFiles, RuleFileInfo{
			Name:    entry.Name(),
			Content: string(content),
		})
	}

	return info
}

func (p *pfHelper) InstallHelper(env *util.Env, progress ProgressFunc) (*PostCommitAction, error) {
	progress = safeProgress(progress)

	if err := p.installHelper(env, progress); err != nil {
		return nil, err
	}

	return &PostCommitAction{
		Run: func(progress ProgressFunc) error {
			return p.activateLaunchDaemon(env, progress)
		},
	}, nil
}

func (p *pfHelper) UninstallHelper(env *util.Env, progress ProgressFunc) (*PostCommitAction, error) {
	progress = safeProgress(progress)

	for _, warn := range p.uninstallHelper(env, progress) {
		progress("Warning: %v\n", warn)
	}

	return &PostCommitAction{
		Run: func(progress ProgressFunc) error {
			return p.flushPfRulesAfterUninstall(env, progress)
		},
	}, nil
}

// Ensure pfHelper implements NetworkHelper at compile time.
var _ NetworkHelper = (*pfHelper)(nil)
