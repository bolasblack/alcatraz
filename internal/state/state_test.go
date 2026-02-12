package state

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/util"
)

// newTestEnv creates a test environment with in-memory filesystem.
func newTestEnv(t *testing.T) *util.Env {
	t.Helper()
	return &util.Env{Fs: afero.NewMemMapFs()}
}

func TestStateFilePath(t *testing.T) {
	tests := []struct {
		name       string
		projectDir string
		want       string
	}{
		{"simple", "/project", "/project/.alca/state.json"},
		{"nested", "/home/user/projects/foo", "/home/user/projects/foo/.alca/state.json"},
		{"trailing slash removed by filepath", "/project/", "/project/.alca/state.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StateFilePath(tt.projectDir)
			if got != tt.want {
				t.Errorf("StateFilePath(%q) = %q, want %q", tt.projectDir, got, tt.want)
			}
		})
	}
}

func TestStateDirPath(t *testing.T) {
	tests := []struct {
		name       string
		projectDir string
		want       string
	}{
		{"simple", "/project", "/project/.alca"},
		{"nested", "/home/user/projects/foo", "/home/user/projects/foo/.alca"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StateDirPath(tt.projectDir)
			if got != tt.want {
				t.Errorf("StateDirPath(%q) = %q, want %q", tt.projectDir, got, tt.want)
			}
		})
	}
}

func TestLabelFilter(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		want      string
	}{
		{"uuid", "abc-123-def", "label=alca.project.id=abc-123-def"},
		{"empty", "", "label=alca.project.id="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LabelFilter(tt.projectID)
			if got != tt.want {
				t.Errorf("LabelFilter(%q) = %q, want %q", tt.projectID, got, tt.want)
			}
		})
	}
}

func TestContainerLabels(t *testing.T) {
	state := &State{
		ProjectID:     "test-project-id",
		ContainerName: "alca-test",
	}
	projectDir := "/home/user/myproject"

	labels := state.ContainerLabels(projectDir)

	if labels[LabelProjectID] != "test-project-id" {
		t.Errorf("expected project ID label %q, got %q", "test-project-id", labels[LabelProjectID])
	}
	if labels[LabelProjectPath] != projectDir {
		t.Errorf("expected project path label %q, got %q", projectDir, labels[LabelProjectPath])
	}
	if labels[LabelVersion] != CurrentVersion {
		t.Errorf("expected version label %q, got %q", CurrentVersion, labels[LabelVersion])
	}
}

func TestLoadNonexistent(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	state, err := Load(env, projectDir)
	if err != nil {
		t.Fatalf("Load() returned error for nonexistent file: %v", err)
	}
	if state != nil {
		t.Error("Load() should return nil state for nonexistent file")
	}
}

func TestSaveAndLoad(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"
	createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	original := &State{
		ProjectID:     "test-uuid-1234",
		ContainerName: "alca-test-uuid-12",
		CreatedAt:     createdAt,
		Runtime:       "Docker",
	}

	if err := Save(env, projectDir, original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file exists
	statePath := StateFilePath(projectDir)
	exists, err := afero.Exists(env.Fs, statePath)
	if err != nil || !exists {
		t.Fatal("state file was not created")
	}

	// Load and verify
	loaded, err := Load(env, projectDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: got %q, want %q", loaded.ProjectID, original.ProjectID)
	}
	if loaded.ContainerName != original.ContainerName {
		t.Errorf("ContainerName mismatch: got %q, want %q", loaded.ContainerName, original.ContainerName)
	}
	if !loaded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", loaded.CreatedAt, original.CreatedAt)
	}
	if loaded.Runtime != original.Runtime {
		t.Errorf("Runtime mismatch: got %q, want %q", loaded.Runtime, original.Runtime)
	}
}

func TestSaveWithConfig(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	cfg := &config.Config{
		Image:   "ubuntu:latest",
		Workdir: "/app",
		Runtime: config.RuntimeDocker,
		Mounts:  []config.MountConfig{{Source: "/host", Target: "/container"}},
	}
	state := &State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
		CreatedAt:     time.Now(),
		Runtime:       "Docker",
		Config:        cfg,
	}

	if err := Save(env, projectDir, state); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(env, projectDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Config == nil {
		t.Fatal("loaded config is nil")
	}
	if loaded.Config.Image != cfg.Image {
		t.Errorf("Config.Image mismatch: got %q, want %q", loaded.Config.Image, cfg.Image)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"
	statePath := StateFilePath(projectDir)

	if err := env.Fs.MkdirAll(StateDirPath(projectDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(env.Fs, statePath, []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(env, projectDir)
	if err == nil {
		t.Error("Load() should return error for invalid JSON")
	}
}

func TestDelete(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	state := &State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
		CreatedAt:     time.Now(),
		Runtime:       "Docker",
	}

	if err := Save(env, projectDir, state); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	if err := Delete(env, projectDir); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Verify file is deleted
	statePath := StateFilePath(projectDir)
	exists, _ := afero.Exists(env.Fs, statePath)
	if exists {
		t.Error("state file should be deleted")
	}

	// Verify directory still exists
	stateDir := StateDirPath(projectDir)
	dirExists, _ := afero.DirExists(env.Fs, stateDir)
	if !dirExists {
		t.Error("state directory should NOT be deleted")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	err := Delete(env, projectDir)
	if err != nil {
		t.Errorf("Delete() should not error for nonexistent file: %v", err)
	}
}

func TestLoadOrCreate_New(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	state, isNew, err := LoadOrCreate(env, projectDir, "Docker")
	if err != nil {
		t.Fatalf("LoadOrCreate() failed: %v", err)
	}

	if !isNew {
		t.Error("expected isNew=true for new state")
	}
	if state.ProjectID == "" {
		t.Error("ProjectID should not be empty")
	}
	if state.ContainerName == "" {
		t.Error("ContainerName should not be empty")
	}
	if state.Runtime != "Docker" {
		t.Errorf("Runtime mismatch: got %q, want %q", state.Runtime, "Docker")
	}

	// Verify file was saved
	statePath := StateFilePath(projectDir)
	exists, _ := afero.Exists(env.Fs, statePath)
	if !exists {
		t.Error("state file should be created")
	}
}

func TestLoadOrCreate_Existing(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	original := &State{
		ProjectID:     "existing-id",
		ContainerName: "alca-existing",
		CreatedAt:     time.Now(),
		Runtime:       "Docker",
	}
	if err := Save(env, projectDir, original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	state, isNew, err := LoadOrCreate(env, projectDir, "Docker")
	if err != nil {
		t.Fatalf("LoadOrCreate() failed: %v", err)
	}

	if isNew {
		t.Error("expected isNew=false for existing state")
	}
	if state.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: got %q, want %q", state.ProjectID, original.ProjectID)
	}
}

func TestLoadOrCreate_RuntimeUpdate(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	original := &State{
		ProjectID:     "existing-id",
		ContainerName: "alca-existing",
		CreatedAt:     time.Now(),
		Runtime:       "Docker",
	}
	if err := Save(env, projectDir, original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load with different runtime
	state, isNew, err := LoadOrCreate(env, projectDir, "Podman")
	if err != nil {
		t.Fatalf("LoadOrCreate() failed: %v", err)
	}

	if isNew {
		t.Error("expected isNew=false")
	}
	if state.Runtime != "Podman" {
		t.Errorf("Runtime should be updated to Podman, got %q", state.Runtime)
	}

	// Verify update was persisted
	reloaded, err := Load(env, projectDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if reloaded.Runtime != "Podman" {
		t.Errorf("Persisted runtime should be Podman, got %q", reloaded.Runtime)
	}
}

func TestUpdateConfig(t *testing.T) {
	state := &State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
	}

	cfg := &config.Config{
		Image:   "ubuntu:latest",
		Workdir: "/app",
	}

	state.UpdateConfig(cfg)

	if state.Config != cfg {
		t.Error("UpdateConfig did not set config")
	}
}

func TestDetectConfigDrift_NilConfig(t *testing.T) {
	state := &State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
		Config:        nil,
	}

	current := &config.Config{
		Image: "ubuntu:latest",
	}

	changes := state.DetectConfigDrift(current)
	if changes != nil {
		t.Error("DetectConfigDrift should return nil when state has no config")
	}
}

func TestDetectConfigDrift_NoChanges(t *testing.T) {
	cfg := &config.Config{
		Image:   "ubuntu:latest",
		Workdir: "/app",
		Runtime: config.RuntimeDocker,
		Commands: config.Commands{
			Up:    config.CommandValue{Command: "apt update"},
			Enter: config.CommandValue{Command: "bash"},
		},
		Mounts: []config.MountConfig{{Source: "/host", Target: "/container"}},
		Resources: config.Resources{
			Memory: "4g",
			CPUs:   2,
		},
		Envs: map[string]config.EnvValue{
			"FOO": {Value: "bar"},
		},
	}
	state := &State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
		Config:        cfg,
	}

	// Same config
	current := &config.Config{
		Image:   "ubuntu:latest",
		Workdir: "/app",
		Runtime: config.RuntimeDocker,
		Commands: config.Commands{
			Up:    config.CommandValue{Command: "apt update"},
			Enter: config.CommandValue{Command: "bash"},
		},
		Mounts: []config.MountConfig{{Source: "/host", Target: "/container"}},
		Resources: config.Resources{
			Memory: "4g",
			CPUs:   2,
		},
		Envs: map[string]config.EnvValue{
			"FOO": {Value: "bar"},
		},
	}

	changes := state.DetectConfigDrift(current)
	if changes != nil {
		t.Errorf("DetectConfigDrift should return nil for identical configs, got %+v", changes)
	}
}

func TestDetectConfigDrift_ImageChange(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Image: "ubuntu:20.04",
		},
	}
	current := &config.Config{
		Image: "ubuntu:22.04",
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil {
		t.Fatal("expected drift changes")
	}
	if changes.Image == nil {
		t.Fatal("expected Image change")
	}
	if (*changes.Image)[0] != "ubuntu:20.04" || (*changes.Image)[1] != "ubuntu:22.04" {
		t.Errorf("Image change: got [%q, %q], want [%q, %q]",
			(*changes.Image)[0], (*changes.Image)[1], "ubuntu:20.04", "ubuntu:22.04")
	}
}

func TestDetectConfigDrift_WorkdirChange(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Workdir: "/old",
		},
	}
	current := &config.Config{
		Workdir: "/new",
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil || changes.Workdir == nil {
		t.Fatal("expected Workdir change")
	}
	if (*changes.Workdir)[0] != "/old" || (*changes.Workdir)[1] != "/new" {
		t.Errorf("Workdir change: got [%q, %q], want [%q, %q]",
			(*changes.Workdir)[0], (*changes.Workdir)[1], "/old", "/new")
	}
}

func TestDetectConfigDrift_RuntimeChange(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Runtime: config.RuntimeAuto,
		},
	}
	current := &config.Config{
		Runtime: config.RuntimeDocker,
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil || changes.Runtime == nil {
		t.Fatal("expected Runtime change")
	}
}

func TestDetectConfigDrift_CommandUpChange(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Commands: config.Commands{Up: config.CommandValue{Command: "old command"}},
		},
	}
	current := &config.Config{
		Commands: config.Commands{Up: config.CommandValue{Command: "new command"}},
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil || changes.CommandUp == nil {
		t.Fatal("expected CommandUp change")
	}
}

func TestDetectConfigDrift_CommandEnterIgnored(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Commands: config.Commands{Enter: config.CommandValue{Command: "old enter"}},
		},
	}
	current := &config.Config{
		Commands: config.Commands{Enter: config.CommandValue{Command: "new enter"}},
	}

	changes := state.DetectConfigDrift(current)
	if changes != nil {
		t.Error("Commands.Enter change should be ignored")
	}
}

func TestDetectConfigDrift_ResourcesChange(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Resources: config.Resources{Memory: "2g", CPUs: 1},
		},
	}
	current := &config.Config{
		Resources: config.Resources{Memory: "4g", CPUs: 2},
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil {
		t.Fatal("expected drift changes")
	}
	if changes.Memory == nil {
		t.Error("expected Memory change")
	}
	if changes.CPUs == nil {
		t.Error("expected CPUs change")
	}
}

func TestDetectConfigDrift_MountsChange(t *testing.T) {
	state := &State{
		Config: &config.Config{
			Mounts: []config.MountConfig{{Source: "/old", Target: "/old"}},
		},
	}
	current := &config.Config{
		Mounts: []config.MountConfig{{Source: "/new", Target: "/new"}},
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil || !changes.Mounts {
		t.Error("expected Mounts=true for mount changes")
	}
}

func TestDetectConfigDrift_EnvsChange(t *testing.T) {
	tests := []struct {
		name      string
		oldEnvs   map[string]config.EnvValue
		newEnvs   map[string]config.EnvValue
		wantDrift bool
	}{
		{
			name:      "value changed",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "old"}},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "new"}},
			wantDrift: true,
		},
		{
			name:      "key added",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}, "BAZ": {Value: "qux"}},
			wantDrift: true,
		},
		{
			name:      "key removed",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}, "BAZ": {Value: "qux"}},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}},
			wantDrift: true,
		},
		{
			name:      "identical - no drift",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}},
			wantDrift: false,
		},
		{
			name:      "interpolated value change ignored",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "${HOST_VAR}"}},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "${OTHER_VAR}"}},
			wantDrift: false,
		},
		{
			name:      "OverrideOnEnter change ignored",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar", OverrideOnEnter: false}},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar", OverrideOnEnter: true}},
			wantDrift: false,
		},
		{
			name:      "both nil - no drift",
			oldEnvs:   nil,
			newEnvs:   nil,
			wantDrift: false,
		},
		{
			name:      "empty vs nil - no drift",
			oldEnvs:   map[string]config.EnvValue{},
			newEnvs:   nil,
			wantDrift: false,
		},
		{
			name:      "nil to non-nil - envs added",
			oldEnvs:   nil,
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}},
			wantDrift: true,
		},
		{
			name:      "empty to non-nil - envs added",
			oldEnvs:   map[string]config.EnvValue{},
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "bar"}},
			wantDrift: true,
		},
		{
			name:      "add interpolated key - structural change should trigger drift",
			oldEnvs:   nil,
			newEnvs:   map[string]config.EnvValue{"FOO": {Value: "${BAR}"}},
			wantDrift: true, // Key set changed - structural drift
		},
		{
			name:      "remove interpolated key - structural change should trigger drift",
			oldEnvs:   map[string]config.EnvValue{"FOO": {Value: "${BAR}"}},
			newEnvs:   nil,
			wantDrift: true, // Key set changed - structural drift
		},
		{
			name:      "add interpolated key to existing literal - structural change",
			oldEnvs:   map[string]config.EnvValue{"EXISTING": {Value: "literal"}},
			newEnvs:   map[string]config.EnvValue{"EXISTING": {Value: "literal"}, "NEW": {Value: "${VAR}"}},
			wantDrift: true, // Key set changed - NEW key added
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &State{
				Config: &config.Config{Envs: tt.oldEnvs},
			}
			current := &config.Config{Envs: tt.newEnvs}

			changes := state.DetectConfigDrift(current)
			gotDrift := changes != nil && changes.Envs
			if gotDrift != tt.wantDrift {
				t.Errorf("DetectConfigDrift().Envs = %v, want %v", gotDrift, tt.wantDrift)
			}
		})
	}
}

// TestDetectConfigDrift_MultipleChanges_EnvsAndCommandUp verifies that when both
// envs and commands.up change, BOTH are reported. Regression test for bug where
// adding envs to a config that previously had none was not detected as drift.
func TestDetectConfigDrift_MultipleChanges_EnvsAndCommandUp(t *testing.T) {
	// Scenario from bug report:
	// - Old config: no envs (nil), commands.up = "old"
	// - New config: has envs, commands.up = "new"
	// - Expected: DriftChanges should have BOTH CommandUp AND Envs set
	state := &State{
		Config: &config.Config{
			Commands: config.Commands{Up: config.CommandValue{Command: "old command"}},
			Envs:     nil, // No envs in original config
		},
	}
	current := &config.Config{
		Commands: config.Commands{Up: config.CommandValue{Command: "new command"}},
		Envs:     map[string]config.EnvValue{"NEW_VAR": {Value: "value"}},
	}

	changes := state.DetectConfigDrift(current)
	if changes == nil {
		t.Fatal("expected drift changes, got nil")
	}
	if changes.CommandUp == nil {
		t.Error("expected CommandUp change to be detected")
	}
	if !changes.Envs {
		t.Error("expected Envs change to be detected (nil -> non-nil)")
	}
}

// TestDetectConfigDrift_AfterSaveLoad_EnvsNilToNonNil tests the exact bug scenario:
// 1. State saved with config that has nil Envs
// 2. State loaded from disk
// 3. New config has Envs
// 4. DetectConfigDrift should report Envs changed
func TestDetectConfigDrift_AfterSaveLoad_EnvsNilToNonNil(t *testing.T) {
	env := newTestEnv(t)
	projectDir := "/project"

	// Step 1: Create and save state with config that has NO envs and commands.up = "old"
	originalConfig := &config.Config{
		Image:    "ubuntu:latest",
		Commands: config.Commands{Up: config.CommandValue{Command: "old command"}},
		Envs:     nil, // No envs
	}
	state := &State{
		ProjectID:     "test-id",
		ContainerName: "alca-test",
		CreatedAt:     time.Now(),
		Runtime:       "Docker",
		Config:        originalConfig,
	}

	if err := Save(env, projectDir, state); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Step 2: Load state back from disk
	loaded, err := Load(env, projectDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Step 3: New config with envs AND changed commands.up
	newConfig := &config.Config{
		Image:    "ubuntu:latest",
		Commands: config.Commands{Up: config.CommandValue{Command: "new command"}},
		Envs:     map[string]config.EnvValue{"NEW_VAR": {Value: "value"}},
	}

	// Step 4: Detect drift - should report BOTH CommandUp AND Envs changed
	changes := loaded.DetectConfigDrift(newConfig)
	if changes == nil {
		t.Fatal("expected drift changes, got nil")
	}
	if changes.CommandUp == nil {
		t.Error("expected CommandUp change to be detected")
	}
	if !changes.Envs {
		t.Error("expected Envs change to be detected (nil -> non-nil after save/load cycle)")
	}
}

func TestStateJSONSerialization(t *testing.T) {
	original := &State{
		ProjectID:     "test-uuid",
		ContainerName: "alca-test",
		CreatedAt:     time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Runtime:       "Docker",
		Config: &config.Config{
			Image:   "ubuntu:latest",
			Workdir: "/app",
		},
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}

	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal state: %v", err)
	}

	if loaded.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: got %q, want %q", loaded.ProjectID, original.ProjectID)
	}
	if loaded.ContainerName != original.ContainerName {
		t.Errorf("ContainerName mismatch: got %q, want %q", loaded.ContainerName, original.ContainerName)
	}
	if loaded.Config == nil || loaded.Config.Image != "ubuntu:latest" {
		t.Error("Config not properly serialized")
	}
}
