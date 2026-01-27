package runtime

// Docker implements the Runtime interface using the Docker CLI.
type Docker struct {
	*dockerCLICompatibleRuntime
}

// NewDocker creates a new Docker runtime instance.
func NewDocker() *Docker {
	return &Docker{
		dockerCLICompatibleRuntime: &dockerCLICompatibleRuntime{
			displayName: "Docker",
			command:     "docker",
		},
	}
}
