// Package state provides project state management for Alcatraz.
// It maintains a local state file (.alca/state.json) that tracks container identity,
// ensuring containers survive directory moves and renames.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
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
)

// State represents the persistent state of an Alcatraz project.
type State struct {
	// ProjectID is a unique UUID for this project, survives directory moves.
	ProjectID string `json:"project_id"`
	// ContainerName is the name of the container for this project.
	ContainerName string `json:"container_name"`
	// CreatedAt is when the state was first created.
	CreatedAt time.Time `json:"created_at"`
	// Runtime is the runtime used for this project (docker, podman, apple).
	Runtime string `json:"runtime"`
	// Config stores the configuration snapshot at container creation time.
	// Used for detecting configuration drift.
	Config *ConfigSnapshot `json:"config,omitempty"`
}

// ConfigSnapshot captures the configuration at container creation time.
// See ConfigDrift.HasDrift() for which fields trigger rebuild.
type ConfigSnapshot struct {
	Image    string   `json:"image"`
	Workdir  string   `json:"workdir"`
	Runtime  string   `json:"runtime"`
	Mounts   []string `json:"mounts,omitempty"`
	CmdUp    string   `json:"cmd_up,omitempty"`
	CmdEnter string   `json:"cmd_enter,omitempty"`
}

// NewConfigSnapshot creates a snapshot from config values.
func NewConfigSnapshot(image, workdir, runtime string, mounts []string, cmdUp, cmdEnter string) *ConfigSnapshot {
	return &ConfigSnapshot{
		Image:    image,
		Workdir:  workdir,
		Runtime:  runtime,
		Mounts:   mounts,
		CmdUp:    cmdUp,
		CmdEnter: cmdEnter,
	}
}

// StateFilePath returns the path to the state file for the given project directory.
func StateFilePath(projectDir string) string {
	return filepath.Join(projectDir, StateDir, StateFilename)
}

// StateDirPath returns the path to the state directory for the given project directory.
func StateDirPath(projectDir string) string {
	return filepath.Join(projectDir, StateDir)
}

// Load reads the state file from the given project directory.
// Returns nil and no error if the state file does not exist.
func Load(projectDir string) (*State, error) {
	path := StateFilePath(projectDir)

	data, err := os.ReadFile(path)
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
func Save(projectDir string, state *State) error {
	dir := StateDirPath(projectDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	path := StateFilePath(projectDir)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadOrCreate loads the state file if it exists, or creates a new one.
// The runtimeName should be the name of the runtime being used (e.g., "Docker").
func LoadOrCreate(projectDir string, runtimeName string) (*State, bool, error) {
	state, err := Load(projectDir)
	if err != nil {
		return nil, false, err
	}

	if state != nil {
		// Update runtime if changed
		if state.Runtime != runtimeName {
			state.Runtime = runtimeName
			if err := Save(projectDir, state); err != nil {
				return nil, false, err
			}
		}
		return state, false, nil
	}

	// Create new state
	projectID := uuid.New().String()
	state = &State{
		ProjectID:     projectID,
		ContainerName: "alca-" + projectID[:12],
		CreatedAt:     time.Now(),
		Runtime:       runtimeName,
	}

	if err := Save(projectDir, state); err != nil {
		return nil, true, err
	}

	return state, true, nil
}

// Delete removes the state file (but not the .alca directory).
func Delete(projectDir string) error {
	path := StateFilePath(projectDir)
	err := os.Remove(path)
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

// ConfigDrift represents configuration changes between state and current config.
type ConfigDrift struct {
	Old *ConfigSnapshot
	New *ConfigSnapshot
}

// HasDrift returns true if configuration has changed in ways that require rebuild.
// See AGD-015 for the struct field exhaustiveness check pattern used here.
func (d *ConfigDrift) HasDrift() bool {
	if d == nil || d.Old == nil || d.New == nil {
		return false
	}

	old, new := d.Old, d.New

	// Compile-time check: must match ConfigSnapshot fields exactly.
	// If ConfigSnapshot adds a field, this line fails to compile,
	// forcing you to update 'fields' and decide whether to compare it.
	// See AGD-015 for pattern details.
	type fields struct {
		Image    string
		Workdir  string
		Runtime  string
		Mounts   []string
		CmdUp    string
		CmdEnter string
	}
	_ = fields(*old)

	// Fields that trigger rebuild:
	if old.Image != new.Image ||
		old.Workdir != new.Workdir ||
		old.Runtime != new.Runtime ||
		old.CmdUp != new.CmdUp ||
		!equalStringSlices(old.Mounts, new.Mounts) {
		return true
	}
	// CmdEnter: intentionally excluded, doesn't require rebuild

	return false
}

// DetectConfigDrift compares the state's config snapshot with the given config.
// Returns nil if no drift or if state has no config snapshot.
func (s *State) DetectConfigDrift(current *ConfigSnapshot) *ConfigDrift {
	if s.Config == nil {
		return nil
	}
	drift := &ConfigDrift{Old: s.Config, New: current}
	if drift.HasDrift() {
		return drift
	}
	return nil
}

// UpdateConfig updates the config snapshot in the state.
func (s *State) UpdateConfig(snapshot *ConfigSnapshot) {
	s.Config = snapshot
}

// equalStringSlices compares two string slices for equality.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
