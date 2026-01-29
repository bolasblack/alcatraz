// Package state provides project state management for Alcatraz.
// It maintains a local state file (.alca/state.json) that tracks container identity,
// ensuring containers survive directory moves and renames.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/util"
)

const (
	// StateDir is the directory name for Alcatraz state files.
	StateDir = ".alca"
	// StateFilename is the name of the state file.
	StateFilename = "state.json"
	// LabelProjectID is the container label for project UUID.
	LabelProjectID = "alca.project.id"
	// LabelProjectPath is the container label for original project path.
	LabelProjectPath = "alca.project.path"
	// LabelVersion is the container label for alca version.
	LabelVersion = "alca.version"
	// CurrentVersion is the current alca state version.
	CurrentVersion = "1"

	// containerNameUUIDPrefixLen is the number of UUID characters used in container names.
	containerNameUUIDPrefixLen = 12
	// stateDirPerm is the permission for the state directory (rwxr-xr-x).
	stateDirPerm = 0755
	// stateFilePerm is the permission for the state file (rw-r--r--).
	stateFilePerm = 0644
)

// State represents the persistent state of an Alcatraz project.
type State struct {
	// ProjectID is a unique UUID for this project, survives directory moves.
	ProjectID string `json:"project_id"`
	// ContainerName is the name of the container for this project.
	ContainerName string `json:"container_name"`
	// CreatedAt is when the state was first created.
	CreatedAt time.Time `json:"created_at"`
	// Runtime is the runtime used for this project (docker, podman).
	Runtime string `json:"runtime"`
	// Config stores the configuration at container creation time.
	// Used for detecting configuration drift.
	Config *config.Config `json:"config,omitempty"`
}

// StateFilePath returns the path to the state file for the given project directory.
func StateFilePath(projectDir string) string {
	return filepath.Join(projectDir, StateDir, StateFilename)
}

// StateDirPath returns the path to the state directory for the given project directory.
func StateDirPath(projectDir string) string {
	return filepath.Join(projectDir, StateDir)
}

// LabelFilter returns a Docker/Podman filter string for finding containers by project ID.
func LabelFilter(projectID string) string {
	return fmt.Sprintf("label=%s=%s", LabelProjectID, projectID)
}

// Load reads the state file from the given project directory.
// Returns nil and no error if the state file does not exist.
func Load(env *util.Env, projectDir string) (*State, error) {
	path := StateFilePath(projectDir)

	data, err := afero.ReadFile(env.Fs, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// Save writes the state file to the given project directory.
// Creates the .alca directory if it does not exist.
func Save(env *util.Env, projectDir string, state *State) error {
	dir := StateDirPath(projectDir)
	if err := env.Fs.MkdirAll(dir, stateDirPerm); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	path := StateFilePath(projectDir)
	if err := afero.WriteFile(env.Fs, path, data, stateFilePerm); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadOrCreate loads the state file if it exists, or creates a new one.
// The runtimeName should be the name of the runtime being used (e.g., "Docker").
func LoadOrCreate(env *util.Env, projectDir string, runtimeName string) (*State, bool, error) {
	state, err := Load(env, projectDir)
	if err != nil {
		return nil, false, err
	}

	if state != nil {
		// Update runtime if changed
		if state.Runtime != runtimeName {
			state.Runtime = runtimeName
			if err := Save(env, projectDir, state); err != nil {
				return nil, false, err
			}
		}
		return state, false, nil
	}

	// Create new state
	projectID := uuid.New().String()
	state = &State{
		ProjectID:     projectID,
		ContainerName: "alca-" + projectID[:containerNameUUIDPrefixLen],
		CreatedAt:     time.Now(),
		Runtime:       runtimeName,
	}

	if err := Save(env, projectDir, state); err != nil {
		return nil, true, err
	}

	return state, true, nil
}

// Delete removes the state file (but not the .alca directory).
func Delete(env *util.Env, projectDir string) error {
	path := StateFilePath(projectDir)
	err := env.Fs.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}
	return nil
}

// ContainerLabels returns the labels to add to a container for this state.
// The projectDir is the absolute path to the project directory.
func (s *State) ContainerLabels(projectDir string) map[string]string {
	return map[string]string{
		LabelProjectID:   s.ProjectID,
		LabelProjectPath: projectDir,
		LabelVersion:     CurrentVersion,
	}
}

// DriftChanges describes specific configuration changes that require rebuild.
//
// Design: Pointer fields (*[2]T) provide old/new values for user-facing diff display.
// Boolean fields are used for complex types (slices, maps) where showing the full
// diff would be verbose - the CLI just reports "changed" for these.
type DriftChanges struct {
	Image     *[2]string // [old, new] if changed
	Workdir   *[2]string
	Runtime   *[2]string
	CommandUp *[2]string
	Memory    *[2]string
	CPUs      *[2]int
	Mounts    bool // true if changed (slice comparison, no diff detail)
	Envs      bool // true if changed (map comparison, no diff detail)
}

// DetectConfigDrift compares the state's config with the given config.
// Returns nil if no drift or if state has no config.
// See AGD-015 for the struct field exhaustiveness check pattern used here.
func (s *State) DetectConfigDrift(current *config.Config) *DriftChanges {
	if s.Config == nil {
		return nil
	}

	enforceConfigFieldCompleteness(s.Config)
	return compareConfigs(s.Config, current)
}

// enforceConfigFieldCompleteness ensures all config.Config fields are handled.
// Compile-time check: if Config adds a field, this fails to compile,
// forcing you to update the mirror types and decide whether to compare it.
// See AGD-015 for pattern details.
func enforceConfigFieldCompleteness(cfg *config.Config) {
	type fields struct {
		Image     string
		Workdir   string
		Runtime   config.RuntimeType
		Commands  config.Commands
		Mounts    []string
		Resources config.Resources
		Envs      map[string]config.EnvValue
		Network   config.Network
	}
	_ = fields(*cfg)

	type fieldsCommands struct {
		Up    string
		Enter string
	}
	_ = fieldsCommands(cfg.Commands)

	type fieldsResources struct {
		Memory string
		CPUs   int
	}
	_ = fieldsResources(cfg.Resources)

	type fieldsEnvValue struct {
		Value           string
		OverrideOnEnter bool
	}
	for _, v := range cfg.Envs {
		_ = fieldsEnvValue(v)
		break // Only need to check one value for type compatibility
	}
}

// compareConfigs compares two configs and returns the differences.
// Returns nil if configs are equivalent.
//
// Intentionally excluded fields (don't require rebuild):
//   - Commands.Enter: only affects enter behavior
//   - EnvValue.OverrideOnEnter: only affects enter behavior
//   - Network.LANAccess: pf rules are external, no container rebuild needed
func compareConfigs(old, new *config.Config) *DriftChanges {
	var changes DriftChanges
	hasAny := false

	if old.Image != new.Image {
		changes.Image = &[2]string{old.Image, new.Image}
		hasAny = true
	}
	if old.Workdir != new.Workdir {
		changes.Workdir = &[2]string{old.Workdir, new.Workdir}
		hasAny = true
	}
	if old.Runtime != new.Runtime {
		changes.Runtime = &[2]string{string(old.Runtime), string(new.Runtime)}
		hasAny = true
	}
	if old.Commands.Up != new.Commands.Up {
		changes.CommandUp = &[2]string{old.Commands.Up, new.Commands.Up}
		hasAny = true
	}
	if old.Resources.Memory != new.Resources.Memory {
		changes.Memory = &[2]string{old.Resources.Memory, new.Resources.Memory}
		hasAny = true
	}
	if old.Resources.CPUs != new.Resources.CPUs {
		changes.CPUs = &[2]int{old.Resources.CPUs, new.Resources.CPUs}
		hasAny = true
	}
	if !slices.Equal(old.Mounts, new.Mounts) {
		changes.Mounts = true
		hasAny = true
	}
	if hasEnvLiteralDrift(old.Envs, new.Envs) {
		changes.Envs = true
		hasAny = true
	}

	if !hasAny {
		return nil
	}
	return &changes
}

// UpdateConfig updates the config in the state.
func (s *State) UpdateConfig(cfg *config.Config) {
	s.Config = cfg
}

// hasEnvLiteralDrift checks if literal (non-interpolated) env values have changed.
// Interpolated values (containing ${...}) are ignored - see AGD-019.
// EnvValue.OverrideOnEnter is also ignored (only affects enter behavior).
func hasEnvLiteralDrift(a, b map[string]config.EnvValue) bool {
	// Collect literal values only
	aLiterals := make(map[string]string)
	for k, v := range a {
		if !v.IsInterpolated() {
			aLiterals[k] = v.Value
		}
	}

	bLiterals := make(map[string]string)
	for k, v := range b {
		if !v.IsInterpolated() {
			bLiterals[k] = v.Value
		}
	}

	// Compare literal maps
	if len(aLiterals) != len(bLiterals) {
		return true
	}
	for k, va := range aLiterals {
		if vb, ok := bLiterals[k]; !ok || va != vb {
			return true
		}
	}
	return false
}
