package runtime

// Podman implements the Runtime interface using the Podman CLI.
// See AGD-002 for Linux isolation solution rationale.
type Podman struct {
	*dockerCLICompatibleRuntime
}

// NewPodman creates a new Podman runtime instance.
func NewPodman() *Podman {
	return &Podman{
		dockerCLICompatibleRuntime: &dockerCLICompatibleRuntime{
			displayName: "Podman",
			command:     "podman",
		},
	}
}
