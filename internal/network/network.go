// Package network provides network isolation for containers.
// See AGD-027 for the decision to use nftables as the primary Linux network.
// See AGD-028 for the lan-access rule syntax specification.
//
// This package re-exports types from shared/ and provides platform-aware
// factory functions. Callers should import this package, not the subpackages.
package network

import (
	"context"
	"runtime"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/nft"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	alcaruntime "github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// Re-export types from shared package for external callers.
type (
	// Type represents the firewall backend type.
	Type = shared.Type
	// Firewall manages network isolation rules for containers.
	Firewall = shared.Firewall
	// NetworkEnv provides dependency injection for filesystem and command execution.
	NetworkEnv = shared.NetworkEnv
	// NetworkHelper manages platform-specific network isolation.
	NetworkHelper = shared.NetworkHelper
	// PostCommitAction encapsulates actions that must run after TransactFs.Commit().
	PostCommitAction = shared.PostCommitAction
	// HelperStatus reports current state of the network helper.
	HelperStatus = shared.HelperStatus
	// DetailedStatusInfo provides implementation-specific status for display.
	DetailedStatusInfo = shared.DetailedStatusInfo
	// RuleFileInfo describes a single rule file.
	RuleFileInfo = shared.RuleFileInfo
	// ProgressFunc reports progress during operations.
	ProgressFunc = shared.ProgressFunc
	// Protocol represents the transport protocol for a firewall rule.
	Protocol = shared.Protocol
	// LANAccessRule represents a parsed lan-access configuration entry.
	LANAccessRule = shared.LANAccessRule
)

// Re-export constants from shared package.
const (
	TypeNone     = shared.TypeNone
	TypeNFTables = shared.TypeNFTables
	ProtoAll     = shared.ProtoAll
	ProtoTCP     = shared.ProtoTCP
	ProtoUDP     = shared.ProtoUDP
)

// Re-export functions from shared package.
var (
	NewNetworkEnv       = shared.NewNetworkEnv
	NewTestNetworkEnv   = shared.NewTestNetworkEnv
	ParseLANAccessRule  = shared.ParseLANAccessRule
	ParseLANAccessRules = shared.ParseLANAccessRules
	HasAllLAN           = shared.HasAllLAN
)

// Detect returns the available firewall type for the current platform.
func Detect(ctx context.Context, cmd util.CommandRunner) Type {
	switch runtime.GOOS {
	case "darwin":
		return TypeNFTables
	case "linux":
		if commandExists(ctx, cmd, "nft") && nftablesWorking(ctx, cmd) {
			return TypeNFTables
		}
		return TypeNone
	default:
		return TypeNone
	}
}

// New creates a Firewall implementation based on the detected type.
// Returns nil if no firewall is available.
func New(ctx context.Context, env *NetworkEnv) (Firewall, Type) {
	t := Detect(ctx, env.Cmd)
	return newFirewallForType(t, env), t
}

// NewNetworkHelper creates a NetworkHelper for the given platform and runtime.
// Returns nil if network isolation is not needed.
func NewNetworkHelper(cfg config.Network, platform alcaruntime.RuntimePlatform) NetworkHelper {
	return newNetworkHelperForPlatform(cfg, platform)
}

// commandExists checks if a command is available in PATH.
func commandExists(ctx context.Context, cmd util.CommandRunner, name string) bool {
	_, err := cmd.Run(ctx, "which", name)
	return err == nil
}

// nftablesWorking checks if nftables kernel support is available.
func nftablesWorking(ctx context.Context, cmd util.CommandRunner) bool {
	_, err := cmd.Run(ctx, "nft", "list", "tables")
	return err == nil
}

// newFirewallForType creates a Firewall for the given type.
func newFirewallForType(t Type, env *NetworkEnv) Firewall {
	switch t {
	case TypeNFTables:
		return nft.New(env)
	default:
		return nil
	}
}

// newNetworkHelperForPlatform creates a platform-specific NetworkHelper.
func newNetworkHelperForPlatform(cfg config.Network, platform alcaruntime.RuntimePlatform) shared.NetworkHelper {
	return nft.NewHelper(cfg, platform)
}
