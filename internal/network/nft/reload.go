package nft

import (
	"fmt"

	"github.com/bolasblack/alcatraz/internal/network/darwin/vmhelper"
)

// reloadVMHelper triggers a rule reload in the VM helper container.
// Only meaningful on macOS where the VM helper runs nftables inside the container runtime VM.
func (n *NFTables) reloadVMHelper() error {
	installed, err := vmhelper.IsInstalled(n.vmEnv)
	if err != nil {
		return fmt.Errorf("vmhelper: failed to check helper status: %w", err)
	}
	if !installed {
		return fmt.Errorf("vmhelper: network helper container is not installed; run 'alca network-helper install' first")
	}
	return vmhelper.Reload(n.vmEnv)
}
