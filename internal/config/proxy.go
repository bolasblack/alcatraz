package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ValidateProxyAddress validates a proxy address string.
// Accepts "host:port" format where host is an IP address (not a hostname)
// and port is 1-65535. The host may contain ${alca:...} tokens which are
// validated but not resolved.
func ValidateProxyAddress(s string) error {
	if s == "" {
		return fmt.Errorf("proxy address is empty: %w", ErrInvalidProxyFormat)
	}

	// Allow alca tokens in the address — validate tokens and port
	if strings.Contains(s, "${alca:") {
		if err := ValidateAlcaTokens(s); err != nil {
			return err
		}
		// Still validate the port portion: extract after last ":"
		lastColon := strings.LastIndex(s, ":")
		if lastColon == -1 {
			return fmt.Errorf("invalid proxy address %q: expected host:port format: %w", s, ErrInvalidProxyFormat)
		}
		portStr := s[lastColon+1:]
		// The port part must not contain alca tokens (tokens belong in host)
		// and must not be inside a token (e.g. "${alca:HOST_IP}:1080" → port is "1080")
		// Check that the last colon is not inside a ${alca:...} token
		if strings.Contains(portStr, "}") || strings.Contains(portStr, "${") {
			return fmt.Errorf("invalid proxy address %q: expected host:port format: %w", s, ErrInvalidProxyFormat)
		}
		if err := validateProxyPort(portStr, s); err != nil {
			return err
		}
		return nil
	}

	_, _, err := ParseProxyAddress(s)
	return err
}

// ParseProxyAddress parses a "host:port" string into its components.
// Host must be an IP address; hostnames are not supported (use ${alca:HOST_IP} instead).
// Returns the host IP and port number.
func ParseProxyAddress(s string) (host string, port int, err error) {
	hostStr, portStr, splitErr := net.SplitHostPort(s)
	if splitErr != nil {
		return "", 0, fmt.Errorf("invalid proxy address %q: expected host:port format: %w", s, ErrInvalidProxyFormat)
	}

	if net.ParseIP(hostStr) == nil {
		return "", 0, fmt.Errorf("invalid proxy address %q: host must be an IP address, not a hostname: %w", s, ErrProxyHostNotIP)
	}

	p, convErr := strconv.Atoi(portStr)
	if convErr != nil || p < 1 || p > 65535 {
		return "", 0, fmt.Errorf("invalid proxy address %q: port must be 1-65535: %w", s, ErrProxyPortOutOfRange)
	}

	return hostStr, p, nil
}

// validateProxyPort validates a port string is numeric and in range 1-65535.
func validateProxyPort(portStr string, fullAddr string) error {
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid proxy address %q: port must be 1-65535: %w", fullAddr, ErrProxyPortOutOfRange)
	}
	return nil
}
