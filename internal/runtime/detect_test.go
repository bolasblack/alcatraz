package runtime

import (
	"strings"
	"testing"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

// IsOrbStack tests moved to runtime_mock_test.go (mock-based, deterministic).

func TestAll(t *testing.T) {
	runtimes := All()
	if len(runtimes) != 2 {
		t.Errorf("expected 2 runtimes, got %d", len(runtimes))
	}

	names := make(map[string]bool)
	for _, rt := range runtimes {
		names[rt.Name()] = true
	}

	if !names["Docker"] {
		t.Error("expected Docker runtime in All()")
	}
	if !names["Podman"] {
		t.Error("expected Podman runtime in All()")
	}
}

func TestByName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Docker", true},
		{"Podman", true},
		{"Unknown", false},
		{"docker", false}, // case sensitive
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := ByName(tt.name)
			if (rt != nil) != tt.expected {
				t.Errorf("ByName(%q) returned %v, expected found=%v", tt.name, rt, tt.expected)
			}
			if rt != nil && rt.Name() != tt.name {
				t.Errorf("ByName(%q).Name() = %q, expected %q", tt.name, rt.Name(), tt.name)
			}
		})
	}
}

func TestDockerName(t *testing.T) {
	d := NewDocker()
	if d.Name() != "Docker" {
		t.Errorf("expected Docker, got %s", d.Name())
	}
}

func TestPodmanName(t *testing.T) {
	p := NewPodman()
	if p.Name() != "Podman" {
		t.Errorf("expected Podman, got %s", p.Name())
	}
}

func TestSelectRuntime_DockerExplicit(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectSuccess("docker version --format {{.Server.Version}}", []byte("24.0.0"))
	env := &RuntimeEnv{Cmd: mock}

	cfg := &config.Config{Runtime: "docker"}
	rt, err := SelectRuntime(env, cfg)
	if err != nil {
		t.Fatalf("SelectRuntime failed: %v", err)
	}
	if rt.Name() != "Docker" {
		t.Errorf("expected Docker, got %s", rt.Name())
	}
}

func TestSelectRuntime_DockerNotAvailable(t *testing.T) {
	mock := util.NewMockCommandRunner()
	mock.ExpectFailure("docker version --format {{.Server.Version}}", errCommandNotFound)
	env := &RuntimeEnv{Cmd: mock}

	cfg := &config.Config{Runtime: "docker"}
	_, err := SelectRuntime(env, cfg)
	if err == nil {
		t.Error("expected error when Docker not available")
	}
	if !strings.Contains(err.Error(), "Docker not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseContainerState(t *testing.T) {
	tests := []struct {
		input    string
		expected ContainerState
	}{
		{"running", StateRunning},
		{"exited", StateStopped},
		{"stopped", StateStopped},
		{"unknown", StateUnknown},
		{"", StateUnknown},
		{"other", StateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseContainerState(tt.input)
			if result != tt.expected {
				t.Errorf("parseContainerState(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContainsNoSuchContainer(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"No such container", true},
		{"no such container", true},
		{"NO SUCH CONTAINER", true},
		{"Error: No such container: test", true},
		{"Container not found", false},
		{"", false},
		{"some other error", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := containsNoSuchContainer(tt.input)
			if result != tt.expected {
				t.Errorf("containsNoSuchContainer(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are defined correctly
	if KeepAliveCommand != "sleep" {
		t.Errorf("KeepAliveCommand = %q, expected 'sleep'", KeepAliveCommand)
	}
	if KeepAliveArg != "infinity" {
		t.Errorf("KeepAliveArg = %q, expected 'infinity'", KeepAliveArg)
	}
	if EnvDebug != "ALCA_DEBUG" {
		t.Errorf("EnvDebug = %q, expected 'ALCA_DEBUG'", EnvDebug)
	}
}

func TestBuildRunArgs(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		projectDir string
		state      *state.State
		contName   string
		wantParts  []string // substrings that must appear in args
		dontWant   []string // substrings that must NOT appear
	}{
		{
			name: "basic config",
			cfg: &config.Config{
				Image:   "test-image:latest",
				Workdir: "/app",
				Mounts:  []config.MountConfig{{Source: ".", Target: "/app"}},
			},
			projectDir: "/home/user/project",
			state: &state.State{
				ProjectID:     "test-uuid-1234",
				ContainerName: "alca-test",
			},
			contName: "alca-test",
			wantParts: []string{
				"run", "-d",
				"--name", "alca-test",
				"-w", "/app",
				"-v", "/home/user/project:/app",
				"test-image:latest",
				"sleep", "infinity",
			},
		},
		{
			name: "with mounts",
			cfg: &config.Config{
				Image:   "test-image",
				Workdir: "/workspace",
				Mounts: []config.MountConfig{
					{Source: ".", Target: "/workspace"},
					{Source: "/host/data", Target: "/container/data"},
					{Source: "/host/cache", Target: "/container/cache", Readonly: true},
				},
			},
			projectDir: "/project",
			state: &state.State{
				ProjectID:     "uuid-5678",
				ContainerName: "alca-mount-test",
			},
			contName: "alca-mount-test",
			wantParts: []string{
				"-v", "/host/data:/container/data",
				"-v", "/host/cache:/container/cache:ro",
				"-v", "/project:/workspace",
			},
		},
		{
			name: "with resources",
			cfg: &config.Config{
				Image:   "test-image",
				Workdir: "/workspace",
				Mounts:  []config.MountConfig{{Source: ".", Target: "/workspace"}},
				Resources: config.Resources{
					Memory: "4g",
					CPUs:   2,
				},
			},
			projectDir: "/project",
			state: &state.State{
				ProjectID:     "uuid-res",
				ContainerName: "alca-res-test",
			},
			contName: "alca-res-test",
			wantParts: []string{
				"-m", "4g",
				"--cpus", "2",
			},
		},
		{
			name: "with static env",
			cfg: &config.Config{
				Image:   "test-image",
				Workdir: "/workspace",
				Mounts:  []config.MountConfig{{Source: ".", Target: "/workspace"}},
				Envs: map[string]config.EnvValue{
					"MY_VAR": {Value: "my_value", OverrideOnEnter: false},
				},
			},
			projectDir: "/project",
			state: &state.State{
				ProjectID:     "uuid-env",
				ContainerName: "alca-env-test",
			},
			contName: "alca-env-test",
			wantParts: []string{
				"-e", "MY_VAR=my_value",
			},
		},
		{
			name: "no resources when zero",
			cfg: &config.Config{
				Image:   "test-image",
				Workdir: "/workspace",
				Mounts:  []config.MountConfig{{Source: ".", Target: "/workspace"}},
				Resources: config.Resources{
					Memory: "",
					CPUs:   0,
				},
			},
			projectDir: "/project",
			state: &state.State{
				ProjectID:     "uuid-nores",
				ContainerName: "alca-nores",
			},
			contName: "alca-nores",
			dontWant: []string{"-m", "--cpus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &dockerCLICompatibleRuntime{
				displayName: "Docker",
				command:     "docker",
			}
			// Mock OrbStack so mounts without excludes use bind mounts (not Mutagen).
			// On macOS, DetectPlatform defaults to DockerDesktop which always uses Mutagen.
			mockCmd := util.NewMockCommandRunner().AllowUnexpected()
			mockCmd.ExpectSuccess("docker info --format {{.OperatingSystem}}", []byte("OrbStack"))
			args := rt.buildRunArgs(&RuntimeEnv{Cmd: mockCmd}, tt.cfg, tt.projectDir, tt.state, tt.contName)

			argsStr := strings.Join(args, " ")
			for _, want := range tt.wantParts {
				if !strings.Contains(argsStr, want) {
					t.Errorf("buildRunArgs() missing %q in args: %v", want, args)
				}
			}
			for _, dontWant := range tt.dontWant {
				if strings.Contains(argsStr, dontWant) {
					t.Errorf("buildRunArgs() should not contain %q in args: %v", dontWant, args)
				}
			}
		})
	}
}

func TestBuildExecArgs(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *config.Config
		containerName string
		command       []string
		wantParts     []string
		dontWant      []string
	}{
		{
			name: "basic exec",
			cfg: &config.Config{
				Workdir: "/workspace",
			},
			containerName: "test-container",
			command:       []string{"bash"},
			wantParts: []string{
				"docker", "exec", "-i",
				"-w", "/workspace",
				"test-container",
				"bash",
			},
		},
		{
			name: "exec with multi-word command",
			cfg: &config.Config{
				Workdir: "/app",
			},
			containerName: "my-container",
			command:       []string{"npm", "run", "test"},
			wantParts: []string{
				"my-container",
				"npm", "run", "test",
			},
		},
		{
			name: "exec with override_on_enter env",
			cfg: &config.Config{
				Workdir: "/workspace",
				Envs: map[string]config.EnvValue{
					"OVERRIDE_VAR": {Value: "override_val", OverrideOnEnter: true},
					"NO_OVERRIDE":  {Value: "static_val", OverrideOnEnter: false},
				},
			},
			containerName: "env-container",
			command:       []string{"sh"},
			wantParts: []string{
				"-e", "OVERRIDE_VAR=override_val",
			},
			dontWant: []string{
				"NO_OVERRIDE=static_val",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &dockerCLICompatibleRuntime{
				displayName: "Docker",
				command:     "docker",
			}
			args := rt.buildExecArgs(tt.cfg, tt.containerName, tt.command)

			argsStr := strings.Join(args, " ")
			for _, want := range tt.wantParts {
				if !strings.Contains(argsStr, want) {
					t.Errorf("buildExecArgs() missing %q in args: %v", want, args)
				}
			}
			for _, dontWant := range tt.dontWant {
				if strings.Contains(argsStr, dontWant) {
					t.Errorf("buildExecArgs() should not contain %q in args: %v", dontWant, args)
				}
			}

			// Verify command is at the end
			cmdStart := len(args) - len(tt.command)
			if cmdStart < 0 {
				t.Fatalf("args too short: %v", args)
			}
			for i, c := range tt.command {
				if args[cmdStart+i] != c {
					t.Errorf("command not at end: expected %v at position %d, got %v", c, cmdStart+i, args[cmdStart+i])
				}
			}
		})
	}
}

func TestBuildExecArgsDefaultEnvs(t *testing.T) {
	// Test that default envs with override_on_enter=true are included
	cfg := &config.Config{
		Workdir: "/workspace",
		// No user-defined envs, rely on defaults from MergedEnvs()
	}

	rt := &dockerCLICompatibleRuntime{
		displayName: "Docker",
		command:     "docker",
	}

	// Set a test env var that defaults have
	t.Setenv("TERM", "xterm-256color")

	args := rt.buildExecArgs(cfg, "test-container", []string{"bash"})
	argsStr := strings.Join(args, " ")

	// Default TERM has override_on_enter=true, so should be included
	if !strings.Contains(argsStr, "TERM=xterm-256color") {
		t.Errorf("buildExecArgs() should include default TERM env, got: %v", args)
	}
}
