package fslint

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("fslint", New)
}

// New creates a new fslint plugin for golangci-lint.
func New(settings any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[PluginSettings](settings)
	if err != nil {
		return nil, err
	}
	return &fslintPlugin{settings: s}, nil
}

// PluginSettings represents the settings passed from golangci-lint.
type PluginSettings struct {
	Config string `json:"config"`
}

type fslintPlugin struct {
	settings PluginSettings
}

func (p *fslintPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	// Set the config file path for the analyzer
	configFile = p.settings.Config
	return []*analysis.Analyzer{Analyzer}, nil
}

func (p *fslintPlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}
