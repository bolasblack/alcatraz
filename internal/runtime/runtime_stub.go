package runtime

import (
	"context"
	"io"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
)

// StubRuntime provides a no-op implementation of the Runtime interface.
// Embed it in test-specific runtime stubs to avoid duplicating all methods.
// Only override the methods your test actually uses.
type StubRuntime struct{}

var _ Runtime = (*StubRuntime)(nil)

func (s *StubRuntime) Name() string { return "StubRuntime" }
func (s *StubRuntime) Available(_ context.Context, _ *RuntimeEnv) bool {
	return false
}
func (s *StubRuntime) Up(_ context.Context, _ *RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ io.Writer) error {
	return nil
}
func (s *StubRuntime) Down(_ context.Context, _ *RuntimeEnv, _ string, _ *state.State) error {
	return nil
}
func (s *StubRuntime) Exec(_ context.Context, _ *RuntimeEnv, _ *config.Config, _ string, _ *state.State, _ []string) error {
	return nil
}
func (s *StubRuntime) Status(_ context.Context, _ *RuntimeEnv, _ string, _ *state.State) (ContainerStatus, error) {
	return ContainerStatus{}, nil
}
func (s *StubRuntime) Reload(_ context.Context, _ *RuntimeEnv, _ *config.Config, _ string, _ *state.State) error {
	return nil
}
func (s *StubRuntime) ListContainers(_ context.Context, _ *RuntimeEnv) ([]ContainerInfo, error) {
	return nil, nil
}
func (s *StubRuntime) RemoveContainer(_ context.Context, _ *RuntimeEnv, _ string) error {
	return nil
}
func (s *StubRuntime) GetContainerIP(_ context.Context, _ *RuntimeEnv, _ string) (string, error) {
	return "", nil
}
func (s *StubRuntime) GetHostIP(_ context.Context, _ *RuntimeEnv) (string, error) {
	return "", nil
}
