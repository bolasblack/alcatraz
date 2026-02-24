package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bolasblack/alcatraz/internal/util"
)

// ErrHostIPResolution is returned when the host IP cannot be resolved from the container runtime.
var ErrHostIPResolution = errors.New("host IP resolution failed")

// GetHostIP queries the container runtime's bridge network to find the gateway IP
// reachable from inside containers. This is used to resolve ${alca:HOST_IP} tokens.
func (r *dockerCLICompatibleRuntime) GetHostIP(ctx context.Context, env *RuntimeEnv) (string, error) {
	switch r.command {
	case "docker":
		return resolveDockerHostIP(ctx, env.Cmd)
	case "podman":
		return resolvePodmanHostIP(ctx, env.Cmd)
	default:
		return "", fmt.Errorf("%w: unsupported runtime %q", ErrHostIPResolution, r.command)
	}
}

// resolveDockerHostIP gets the host IP from Docker's bridge network gateway.
func resolveDockerHostIP(ctx context.Context, cmd util.CommandRunner) (string, error) {
	output, err := cmd.RunQuiet(ctx, "docker", "network", "inspect", "bridge",
		"--format", "{{(index .IPAM.Config 0).Gateway}}")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHostIPResolution, err)
	}

	ip := strings.TrimSpace(string(output))
	if ip == "" {
		return "", fmt.Errorf("%w: empty gateway in docker bridge network", ErrHostIPResolution)
	}
	return ip, nil
}

// podmanNetworkInspect represents the relevant fields from `podman network inspect podman` JSON output.
type podmanNetworkInspect struct {
	Subnets []podmanSubnet `json:"subnets"`
}

type podmanSubnet struct {
	Gateway string `json:"gateway"`
}

// resolvePodmanHostIP gets the host IP from Podman's default network gateway.
func resolvePodmanHostIP(ctx context.Context, cmd util.CommandRunner) (string, error) {
	output, err := cmd.RunQuiet(ctx, "podman", "network", "inspect", "podman")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHostIPResolution, err)
	}

	// podman network inspect returns a JSON array
	var networks []podmanNetworkInspect
	if err := json.Unmarshal(output, &networks); err != nil {
		return "", fmt.Errorf("%w: failed to parse podman network inspect output: %v", ErrHostIPResolution, err)
	}

	if len(networks) == 0 || len(networks[0].Subnets) == 0 || networks[0].Subnets[0].Gateway == "" {
		return "", fmt.Errorf("%w: no gateway found in podman network", ErrHostIPResolution)
	}

	return networks[0].Subnets[0].Gateway, nil
}
