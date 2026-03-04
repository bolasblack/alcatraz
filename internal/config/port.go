// port.go implements port mapping configuration parsing and helpers.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"
)

// PortConfig represents a port mapping configuration for Docker -p flags.
type PortConfig struct {
	Port     int    `json:"port" toml:"port" jsonschema:"description=Container port (required, 1-65535)"`
	HostIP   string `json:"hostIp,omitempty" toml:"hostIp,omitempty" jsonschema:"description=Host IP to bind (default: all interfaces)"`
	HostPort int    `json:"hostPort,omitempty" toml:"hostPort,omitempty" jsonschema:"description=Host port (default: same as container port)"`
	Protocol string `json:"protocol,omitempty" toml:"protocol,omitempty" jsonschema:"description=Protocol: tcp (default) or udp"`
}

// ValidatePorts validates a slice of PortConfig entries.
func ValidatePorts(ports []PortConfig) error {
	for i, p := range ports {
		if err := validatePort(p); err != nil {
			return fmt.Errorf("ports[%d]: %w", i, err)
		}
	}
	return nil
}

// validatePort validates a single PortConfig.
func validatePort(p PortConfig) error {
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("port %d out of range 1-65535: %w", p.Port, ErrInvalidPort)
	}
	if p.HostPort != 0 && (p.HostPort < 1 || p.HostPort > 65535) {
		return fmt.Errorf("hostPort %d out of range 1-65535: %w", p.HostPort, ErrInvalidPort)
	}
	if p.Protocol != "" && p.Protocol != "tcp" && p.Protocol != "udp" {
		return fmt.Errorf("protocol %q must be tcp or udp: %w", p.Protocol, ErrInvalidProtocol)
	}
	if p.HostIP != "" && net.ParseIP(p.HostIP) == nil {
		return fmt.Errorf("hostIp %q is not a valid IP address: %w", p.HostIP, ErrInvalidHostIP)
	}
	return nil
}

// FormatPortArg produces the Docker -p argument string for a PortConfig.
// Examples:
//   - {Port: 8080}                           -> "8080:8080"
//   - {Port: 3000, HostPort: 3001}           -> "3001:3000"
//   - {Port: 5432, HostIP: "127.0.0.1"}      -> "127.0.0.1:5432:5432"
//   - {Port: 53, Protocol: "udp"}            -> "53:53/udp"
//   - {Port: 80, HostIP: "0.0.0.0", HostPort: 8080} -> "0.0.0.0:8080:80"
func FormatPortArg(p PortConfig) string {
	hostPort := p.HostPort
	if hostPort == 0 {
		hostPort = p.Port
	}
	protocol := p.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	var result string
	if p.HostIP != "" {
		result = fmt.Sprintf("%s:%d:%d", p.HostIP, hostPort, p.Port)
	} else {
		result = fmt.Sprintf("%d:%d", hostPort, p.Port)
	}

	if protocol != "tcp" {
		result += "/" + protocol
	}
	return result
}

// ParsePortString parses a Docker-style port string into PortConfig.
// Supported formats:
//   - "8080"                    → container port 8080
//   - "3001:3000"               → host 3001 → container 3000
//   - "127.0.0.1:5432:5432"    → bind 127.0.0.1, host 5432 → container 5432
//   - "53:53/udp"               → host 53 → container 53, UDP
//   - "0.0.0.0:8080:80/tcp"    → bind 0.0.0.0, host 8080 → container 80, TCP
func ParsePortString(s string) (PortConfig, error) {
	if s == "" {
		return PortConfig{}, fmt.Errorf("port string is empty: %w", ErrInvalidPortFormat)
	}

	var pc PortConfig

	// Split off protocol suffix first
	rest := s
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		pc.Protocol = s[idx+1:]
		rest = s[:idx]
		if pc.Protocol != "tcp" && pc.Protocol != "udp" {
			return PortConfig{}, fmt.Errorf("protocol %q must be tcp or udp: %w", pc.Protocol, ErrInvalidProtocol)
		}
	}

	// Split the rest on ":"
	parts := strings.Split(rest, ":")
	switch len(parts) {
	case 1:
		// "8080" — container port only
		port, err := parsePortNumber(parts[0])
		if err != nil {
			return PortConfig{}, err
		}
		pc.Port = port

	case 2:
		// "3001:3000" — hostPort:containerPort
		hostPort, err := parsePortNumber(parts[0])
		if err != nil {
			return PortConfig{}, err
		}
		containerPort, err := parsePortNumber(parts[1])
		if err != nil {
			return PortConfig{}, err
		}
		pc.Port = containerPort
		pc.HostPort = hostPort

	case 3:
		// "127.0.0.1:5432:5432" — hostIP:hostPort:containerPort
		if net.ParseIP(parts[0]) == nil {
			return PortConfig{}, fmt.Errorf("hostIp %q is not a valid IP address: %w", parts[0], ErrInvalidHostIP)
		}
		pc.HostIP = parts[0]
		hostPort, err := parsePortNumber(parts[1])
		if err != nil {
			return PortConfig{}, err
		}
		containerPort, err := parsePortNumber(parts[2])
		if err != nil {
			return PortConfig{}, err
		}
		pc.HostPort = hostPort
		pc.Port = containerPort

	default:
		return PortConfig{}, fmt.Errorf("invalid port format %q: too many colons: %w", s, ErrInvalidPortFormat)
	}

	return pc, nil
}

// parsePortNumber parses and validates a port number string.
func parsePortNumber(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("port number is empty: %w", ErrInvalidPortFormat)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("port %q is not a number: %w", s, ErrInvalidPortFormat)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port %d out of range 1-65535: %w", n, ErrInvalidPort)
	}
	return n, nil
}

// UnmarshalJSON supports both string ("8080", "3001:3000") and object formats.
// This provides backward compatibility with state files saved before string port support.
func (p *PortConfig) UnmarshalJSON(data []byte) error {
	// Try string format first
	var s string
	if json.Unmarshal(data, &s) == nil {
		parsed, err := ParsePortString(s)
		if err != nil {
			return err
		}
		*p = parsed
		return nil
	}

	// Object format
	type portConfigAlias PortConfig
	var alias portConfigAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*p = PortConfig(alias)
	return nil
}

// RawPortSlice is a slice of raw port values for RawConfig.
// Used for both TOML parsing (accepts string or object) and JSON schema generation.
type RawPortSlice []any

// JSONSchema implements jsonschema.JSONSchemer to generate correct schema.
func (RawPortSlice) JSONSchema() *jsonschema.Schema {
	portProps := jsonschema.NewProperties()
	portProps.Set("port", &jsonschema.Schema{Type: "integer", Description: "Container port (required, 1-65535)"})
	portProps.Set("hostIp", &jsonschema.Schema{Type: "string", Description: "Host IP to bind (default: all interfaces)"})
	portProps.Set("hostPort", &jsonschema.Schema{Type: "integer", Description: "Host port (default: same as container port)"})
	portProps.Set("protocol", &jsonschema.Schema{Type: "string", Description: "Protocol: tcp (default) or udp"})

	return &jsonschema.Schema{
		Type: "array",
		Items: &jsonschema.Schema{
			OneOf: []*jsonschema.Schema{
				{Type: "string", Description: "String format: [hostIp:]hostPort:containerPort[/protocol] or just containerPort"},
				{
					Type:                 "object",
					Properties:           portProps,
					Required:             []string{"port"},
					AdditionalProperties: jsonschema.FalseSchema,
					Description:          "Object format with explicit fields",
				},
			},
		},
		Description: "Port mappings (Docker -p flags)",
	}
}

// parsePorts converts raw port values to PortConfig slice.
// Accepts both string format ("8080", "3001:3000") and object format.
func parsePorts(raw []any) ([]PortConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	ports := make([]PortConfig, 0, len(raw))
	for i, val := range raw {
		p, err := parsePortValue(val)
		if err != nil {
			return nil, fmt.Errorf("ports[%d]: %w", i, err)
		}
		ports = append(ports, p)
	}
	return ports, nil
}

// parsePortValue converts a single raw port value to PortConfig.
func parsePortValue(val any) (PortConfig, error) {
	switch v := val.(type) {
	case string:
		return ParsePortString(v)
	case map[string]any:
		return parsePortObject(v)
	default:
		return PortConfig{}, fmt.Errorf("invalid type: %T: %w", val, ErrInvalidType)
	}
}

// parsePortObject parses a port object with port, hostIp, hostPort, protocol fields.
func parsePortObject(m map[string]any) (PortConfig, error) {
	var pc PortConfig

	// Port is required — TOML decodes integers as int64
	switch v := m["port"].(type) {
	case int64:
		pc.Port = int(v)
	case float64:
		pc.Port = int(v)
	default:
		return PortConfig{}, fmt.Errorf("port is required and must be an integer")
	}

	if hostIP, ok := m["hostIp"].(string); ok {
		pc.HostIP = hostIP
	}

	switch v := m["hostPort"].(type) {
	case int64:
		pc.HostPort = int(v)
	case float64:
		pc.HostPort = int(v)
	case nil:
		// optional, leave zero
	}

	if protocol, ok := m["protocol"].(string); ok {
		pc.Protocol = protocol
	}

	return pc, nil
}

// PortsEqual compares two slices of PortConfig for equality.
func PortsEqual(a, b []PortConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !portEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// portEqual compares two PortConfig structs for equality.
func portEqual(a, b PortConfig) bool {
	// Mirror type ensures all PortConfig fields are explicitly handled (AGD-015).
	type fields struct {
		Port     int
		HostIP   string
		HostPort int
		Protocol string
	}
	_ = fields(a)
	_ = fields(b)

	return a.Port == b.Port &&
		a.HostIP == b.HostIP &&
		a.HostPort == b.HostPort &&
		a.Protocol == b.Protocol
}
