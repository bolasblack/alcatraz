package shared

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// LanAccessWildcard is the special value that allows all LAN access.
const LanAccessWildcard = "*"

// Protocol represents the transport protocol for a firewall rule.
type Protocol int

const (
	// ProtoAll allows both TCP and UDP (used when no port or *:// prefix).
	ProtoAll Protocol = iota
	// ProtoTCP allows only TCP connections.
	ProtoTCP
	// ProtoUDP allows only UDP connections.
	ProtoUDP
)

// String returns the protocol name.
func (p Protocol) String() string {
	switch p {
	case ProtoTCP:
		return "tcp"
	case ProtoUDP:
		return "udp"
	default:
		return "*"
	}
}

// LANAccessRule represents a parsed lan-access configuration entry.
// See AGD-028 for the rule syntax specification.
type LANAccessRule struct {
	Raw      string   // Original rule string for error messages
	IP       string   // IP address or CIDR (e.g., "192.168.1.100", "10.0.0.0/8", "fe80::1", "2001:db8::/32")
	Port     int      // Port number, 0 means all ports
	Protocol Protocol // TCP, UDP, or All
	IsIPv6   bool     // Whether this is an IPv6 address
	AllLAN   bool     // true if rule is "*" (allow all LAN)
}

// ParseLANAccessRule parses a lan-access rule string.
// Supports formats:
//
//	"*"                         → allow all
//	"192.168.1.100"             → IPv4, all ports, all protocols
//	"192.168.1.100:8080"        → IPv4, port 8080, TCP default
//	"192.168.1.100:*"           → IPv4, all ports, all protocols
//	"tcp://192.168.1.100:8080"  → IPv4, port 8080, TCP
//	"udp://192.168.1.100:53"    → IPv4, port 53, UDP
//	"*://192.168.1.100:443"     → IPv4, port 443, TCP+UDP
//	"192.168.1.0/24:8080"       → CIDR, port 8080, TCP default
//	"fe80::1"                   → IPv6, all ports
//	"[fe80::1]:8080"            → IPv6, port 8080, TCP default
//	"tcp://[2001:db8::1]:443"   → IPv6, port 443, TCP
//	"[2001:db8::/32]:*"         → IPv6 CIDR, all ports
func ParseLANAccessRule(s string) (LANAccessRule, error) {
	raw := s
	s = strings.TrimSpace(s)

	if s == "" {
		return LANAccessRule{}, fmt.Errorf("lan-access rule: empty rule string")
	}

	// Handle wildcard (allow all LAN)
	if s == "*" {
		return LANAccessRule{
			Raw:    raw,
			AllLAN: true,
		}, nil
	}

	// Parse protocol prefix
	proto := ProtoAll
	hasProtoPrefix := false

	if strings.HasPrefix(s, "tcp://") {
		proto = ProtoTCP
		hasProtoPrefix = true
		s = strings.TrimPrefix(s, "tcp://")
	} else if strings.HasPrefix(s, "udp://") {
		proto = ProtoUDP
		hasProtoPrefix = true
		s = strings.TrimPrefix(s, "udp://")
	} else if strings.HasPrefix(s, "*://") {
		proto = ProtoAll
		hasProtoPrefix = true
		s = strings.TrimPrefix(s, "*://")
	}

	var ipStr, portStr string
	var isIPv6 bool

	// Split address into IP and port components
	if strings.HasPrefix(s, "[") {
		var err error
		ipStr, portStr, err = parseIPv6WithBrackets(s, raw)
		if err != nil {
			return LANAccessRule{}, err
		}
		isIPv6 = true
	} else {
		ipStr, portStr, isIPv6 = parseIPv4OrBareIPv6(s)
	}

	// Validate IP address or CIDR
	if err := validateIP(ipStr, isIPv6); err != nil {
		return LANAccessRule{}, fmt.Errorf("lan-access rule %q: %w", raw, err)
	}

	// Parse port
	port, err := parsePort(portStr, raw)
	if err != nil {
		return LANAccessRule{}, err
	}

	// Determine final protocol
	// - If explicit protocol prefix was given, use it
	// - If port is specified (and not *), default to TCP
	// - If no port or port is *, use ProtoAll
	if !hasProtoPrefix {
		if port > 0 {
			proto = ProtoTCP // Default to TCP when port is specified
		} else {
			proto = ProtoAll // All ports → all protocols
		}
	}

	return LANAccessRule{
		Raw:      raw,
		IP:       ipStr,
		Port:     port,
		Protocol: proto,
		IsIPv6:   isIPv6,
		AllLAN:   false,
	}, nil
}

// parseIPv6WithBrackets parses bracketed IPv6 notation: [ip]:port or [ip].
func parseIPv6WithBrackets(s string, raw string) (ipStr string, portStr string, err error) {
	closeBracket := strings.Index(s, "]")
	if closeBracket == -1 {
		return "", "", fmt.Errorf("lan-access rule %q: missing closing bracket for IPv6 address", raw)
	}

	ipStr = s[1:closeBracket]
	remainder := s[closeBracket+1:]

	switch {
	case remainder == "":
		// No port specified: [fe80::1]
		portStr = ""
	case strings.HasPrefix(remainder, ":"):
		// Port specified: [fe80::1]:8080
		portStr = remainder[1:]
	default:
		return "", "", fmt.Errorf("lan-access rule %q: unexpected characters after IPv6 address: %q", raw, remainder)
	}

	return ipStr, portStr, nil
}

// parseIPv4OrBareIPv6 parses an address without brackets.
// Handles IPv4, IPv4 CIDR, IPv4:port, or bare IPv6 (without port).
func parseIPv4OrBareIPv6(s string) (ipStr string, portStr string, isIPv6 bool) {
	colonCount := strings.Count(s, ":")

	switch {
	case colonCount > 1:
		// IPv6 without brackets (must be no port)
		return s, "", true
	case colonCount == 1:
		// IPv4 with port OR IPv4 CIDR with port
		lastColon := strings.LastIndex(s, ":")
		return s[:lastColon], s[lastColon+1:], false
	default:
		// No colon: IPv4 or IPv4 CIDR without port
		return s, "", false
	}
}

// parsePort parses a port string into a port number.
// Returns 0 for empty or wildcard ("*") port strings.
func parsePort(portStr string, raw string) (int, error) {
	if portStr == "" || portStr == "*" {
		return 0, nil
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("lan-access rule %q: invalid port %q", raw, portStr)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("lan-access rule %q: port %d out of range (1-65535)", raw, p)
	}
	return p, nil
}

// ParseLANAccessRules parses multiple rule strings.
// Returns an error if any rule is invalid.
func ParseLANAccessRules(rules []string) ([]LANAccessRule, error) {
	result := make([]LANAccessRule, 0, len(rules))
	for _, r := range rules {
		rule, err := ParseLANAccessRule(r)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, nil
}

// HasAllLAN returns true if any rule allows all LAN access.
func HasAllLAN(rules []LANAccessRule) bool {
	for _, r := range rules {
		if r.AllLAN {
			return true
		}
	}
	return false
}

// validateIP validates an IP address or CIDR notation.
func validateIP(ipStr string, expectIPv6 bool) error {
	// Check if it's CIDR notation
	if strings.Contains(ipStr, "/") {
		ip, ipNet, err := net.ParseCIDR(ipStr)
		if err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", ipStr, err)
		}

		isV6 := ip.To4() == nil
		if expectIPv6 && !isV6 {
			return fmt.Errorf("expected IPv6 address but got IPv4: %q", ipStr)
		}
		if !expectIPv6 && isV6 {
			return fmt.Errorf("expected IPv4 address but got IPv6: %q", ipStr)
		}

		// Validate prefix length
		ones, bits := ipNet.Mask.Size()
		if isV6 {
			if ones < 0 || ones > 128 {
				return fmt.Errorf("invalid IPv6 CIDR prefix /%d (must be 0-128)", ones)
			}
		} else {
			if ones < 0 || ones > 32 {
				return fmt.Errorf("invalid IPv4 CIDR prefix /%d (must be 0-32)", ones)
			}
		}
		_ = bits // suppress unused warning

		return nil
	}

	// Plain IP address
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("invalid IP address %q", ipStr)
	}

	isV6 := ip.To4() == nil
	if expectIPv6 && !isV6 {
		return fmt.Errorf("expected IPv6 address but got IPv4: %q", ipStr)
	}
	if !expectIPv6 && isV6 {
		return fmt.Errorf("expected IPv4 address but got IPv6: %q", ipStr)
	}

	return nil
}
