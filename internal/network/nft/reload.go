package nft

import (
	"context"
	"fmt"

	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
)

// reloadNetworkHelper triggers a rule reload in the network helper container.
// Only meaningful on macOS where the network helper runs nftables inside the container runtime VM.
func (n *NFTables) reloadNetworkHelper(ctx context.Context) error {
	installed, err := vmhelper.IsInstalled(ctx, n.vmHelperEnv)
	if err != nil {
		return fmt.Errorf("network-helper: failed to check helper status: %w", err)
	}
	if !installed {
		return fmt.Errorf("network-helper: network helper container is not installed; run 'alca network-helper install' first")
	}
	return vmhelper.Reload(ctx, n.vmHelperEnv)
}
