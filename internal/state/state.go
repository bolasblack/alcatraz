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
		if err := syncRuntime(env, projectDir, state, runtimeName); err != nil {
			return nil, false, err
		}
		return state, false, nil
	}

	state = newState(runtimeName)
	if err := Save(env, projectDir, state); err != nil {
		return nil, true, err
	}

	return state, true, nil
}

// newState creates a fresh State with a new project UUID and container name.
func newState(runtimeName string) *State {
	projectID := uuid.New().String()
	return &State{
		ProjectID:     projectID,
		ContainerName: "alca-" + projectID[:containerNameUUIDPrefixLen],
		CreatedAt:     time.Now(),
		Runtime:       runtimeName,
	}
}

// syncRuntime persists the runtime name if it has changed.
func syncRuntime(env *util.Env, projectDir string, state *State, runtimeName string) error {
	if state.Runtime == runtimeName {
		return nil
	}
	state.Runtime = runtimeName
	return Save(env, projectDir, state)
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
	// Each field is compared explicitly. This is intentional: the AGD-015
	// exhaustiveness check in enforceConfigFieldCompleteness ensures new
	// config fields cause a compile error, forcing review here.
	//
	// The mix of *[2]T (scalar diff) and bool (complex-type flag) is by
	// design - see DriftChanges doc comment.
	var c DriftChanges

	if old.Image != new.Image {
		c.Image = &[2]string{old.Image, new.Image}
	}
	if old.Workdir != new.Workdir {
		c.Workdir = &[2]string{old.Workdir, new.Workdir}
	}
	if old.Runtime != new.Runtime {
		c.Runtime = &[2]string{string(old.Runtime), string(new.Runtime)}
	}
	if old.Commands.Up != new.Commands.Up {
		c.CommandUp = &[2]string{old.Commands.Up, new.Commands.Up}
	}
	if old.Resources.Memory != new.Resources.Memory {
		c.Memory = &[2]string{old.Resources.Memory, new.Resources.Memory}
	}
	if old.Resources.CPUs != new.Resources.CPUs {
		c.CPUs = &[2]int{old.Resources.CPUs, new.Resources.CPUs}
	}
	if !slices.Equal(old.Mounts, new.Mounts) {
		c.Mounts = true
	}
	if hasEnvLiteralDrift(old.Envs, new.Envs) {
		c.Envs = true
	}

	if c == (DriftChanges{}) {
		return nil
	}
	return &c
}

// UpdateConfig updates the config in the state.
func (s *State) UpdateConfig(cfg *config.Config) {
	s.Config = cfg
}

// hasEnvLiteralDrift checks if env configuration has changed in ways that require rebuild.
// Two types of drift are detected:
// 1. Structural drift: key set changes (adding/removing keys) - regardless of value type
// 2. Value drift: literal (non-interpolated) value changes - see AGD-019
//
// Interpolated values (containing ${...}) are not compared because they depend on
// host environment at runtime. However, adding/removing interpolated keys IS detected
// as structural drift since it changes the container's environment shape.
// EnvValue.OverrideOnEnter is ignored (only affects enter behavior).
func hasEnvLiteralDrift(a, b map[string]config.EnvValue) bool {
	// First check: structural drift (key set changes)
	// This catches adding/removing ANY key, including interpolated ones
	if len(a) != len(b) {
		return true
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return true // Key removed or renamed
		}
	}

	// Second check: value drift for literal (non-interpolated) values only
	// Interpolated values can't be compared at parse time (AGD-019)
	for k, va := range a {
		vb := b[k] // Key exists (checked above)
		// Only compare if BOTH are literal (non-interpolated)
		if !va.IsInterpolated() && !vb.IsInterpolated() {
			if va.Value != vb.Value {
				return true
			}
		}
	}
	return false
}
