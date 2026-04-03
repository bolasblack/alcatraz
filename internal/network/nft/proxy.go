// proxy.go provides proxy-related helpers for nftables (AGD-037).
package nft

import (
	"strings"

	"github.com/bolasblack/alcatraz/internal/network/shared"
)

// proxyTableName returns the nftables table name for proxy DNAT rules.
// Derived from the container ID via the isolation table naming convention.
func proxyTableName(containerID string) string {
	return "alca-proxy-" + shared.ShortContainerID(containerID)
}

// proxyTableFromIsolationTable derives the proxy table name from an isolation table name.
// The isolation table has the form "alca-<short-id>"; the proxy table is "alca-proxy-<short-id>".
// Returns empty string if tableName doesn't have the expected "alca-" prefix.
func proxyTableFromIsolationTable(tableName string) string {
	if !strings.HasPrefix(tableName, "alca-") {
		return ""
	}
	return "alca-proxy-" + tableName[len("alca-"):]
}
