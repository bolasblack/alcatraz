package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/bolasblack/alcatraz/internal/util"
)

// unknownRuntime is a test stub for an unsupported runtime.
type unknownRuntime struct {
	StubRuntime
}

func (r *unknownRuntime) Name() string { return "containerd" }
func (r *unknownRuntime) GetHostIP(context.Context, *RuntimeEnv) (string, error) {
	return "", fmt.Errorf("%w: unsupported runtime %q", ErrHostIPResolution, "containerd")
}

var _ Runtime = (*unknownRuntime)(nil)

func TestGetHostIP_Docker(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess(
		"docker network inspect bridge --format {{(index .IPAM.Config 0).Gateway}}",
		[]byte("172.17.0.1\n"),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewDocker()
	ip, err := rt.GetHostIP(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "172.17.0.1" {
		t.Errorf("got %q, want %q", ip, "172.17.0.1")
	}
}

func TestGetHostIP_Podman(t *testing.T) {
	podmanJSON := `[{"subnets":[{"gateway":"10.88.0.1","subnet":"10.88.0.0/16"}]}]`
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess(
		"podman network inspect podman",
		[]byte(podmanJSON),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewPodman()
	ip, err := rt.GetHostIP(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.88.0.1" {
		t.Errorf("got %q, want %q", ip, "10.88.0.1")
	}
}

func TestGetHostIP_DockerCommandFailure(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectFailure(
		"docker network inspect bridge --format {{(index .IPAM.Config 0).Gateway}}",
		fmt.Errorf("docker not running"),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewDocker()
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}

func TestGetHostIP_PodmanCommandFailure(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectFailure(
		"podman network inspect podman",
		fmt.Errorf("podman not running"),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewPodman()
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}

func TestGetHostIP_UnknownRuntime(t *testing.T) {
	cmd := util.NewMockCommandRunner()

	env := NewRuntimeEnv(cmd)
	rt := &unknownRuntime{}
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}

func TestGetHostIP_DockerEmptyOutput(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess(
		"docker network inspect bridge --format {{(index .IPAM.Config 0).Gateway}}",
		[]byte("\n"),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewDocker()
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}

func TestGetHostIP_PodmanEmptyGateway(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess(
		"podman network inspect podman",
		[]byte(`[{"subnets":[{"gateway":"","subnet":"10.88.0.0/16"}]}]`),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewPodman()
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}

func TestGetHostIP_PodmanInvalidJSON(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess(
		"podman network inspect podman",
		[]byte("not valid json at all{{{"),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewPodman()
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}

func TestGetHostIP_PodmanNoSubnets(t *testing.T) {
	cmd := util.NewMockCommandRunner()
	cmd.ExpectSuccess(
		"podman network inspect podman",
		[]byte(`[{"subnets":[]}]`),
	)
	defer cmd.AssertAllExpectationsMet(t)

	env := NewRuntimeEnv(cmd)
	rt := NewPodman()
	_, err := rt.GetHostIP(context.Background(), env)
	if !errors.Is(err, ErrHostIPResolution) {
		t.Errorf("expected ErrHostIPResolution, got: %v", err)
	}
}
