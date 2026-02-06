package shared

import "strings"

// ShortContainerID returns the first 12 characters of a container ID.
// This is the standard Docker short ID format.
func ShortContainerID(containerID string) string {
	if len(containerID) > 12 {
		return containerID[:12]
	}
	return containerID
}

// IsIPv6 returns true if the IP address string is IPv6.
func IsIPv6(ip string) bool {
	return strings.Contains(ip, ":")
}

// PrivateIPv4Ranges are RFC1918 and other private IPv4 ranges to block.
var PrivateIPv4Ranges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16", // Link-local
	"127.0.0.0/8",    // Loopback
}

// PrivateIPv6Ranges are private IPv6 ranges to block.
var PrivateIPv6Ranges = []string{
	"fe80::/10", // Link-local
	"fc00::/7",  // ULA
	"::1/128",   // Loopback
}

// SafeProgress returns a no-op ProgressFunc if the given one is nil.
func SafeProgress(progress ProgressFunc) ProgressFunc {
	if progress == nil {
		return func(string, ...any) {}
	}
	return progress
}
