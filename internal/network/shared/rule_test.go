package shared

import (
	"testing"
)

func TestParseLANAccessRule(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     LANAccessRule
		wantErr  bool
		errMatch string
	}{
		// Wildcard
		{
			name:  "wildcard allows all",
			input: "*",
			want:  LANAccessRule{Raw: "*", AllLAN: true},
		},

		// IPv4 without port
		{
			name:  "IPv4 only",
			input: "192.168.1.100",
			want:  LANAccessRule{Raw: "192.168.1.100", IP: "192.168.1.100", Port: 0, Protocol: ProtoAll, IsIPv6: false},
		},
		{
			name:  "IPv4 with explicit all ports",
			input: "192.168.1.100:*",
			want:  LANAccessRule{Raw: "192.168.1.100:*", IP: "192.168.1.100", Port: 0, Protocol: ProtoAll, IsIPv6: false},
		},

		// IPv4 with port
		{
			name:  "IPv4 with port defaults to TCP",
			input: "192.168.1.100:8080",
			want:  LANAccessRule{Raw: "192.168.1.100:8080", IP: "192.168.1.100", Port: 8080, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "IPv4 with port 443",
			input: "192.168.1.100:443",
			want:  LANAccessRule{Raw: "192.168.1.100:443", IP: "192.168.1.100", Port: 443, Protocol: ProtoTCP, IsIPv6: false},
		},

		// IPv4 with protocol prefix
		{
			name:  "IPv4 TCP explicit",
			input: "tcp://192.168.1.100:8080",
			want:  LANAccessRule{Raw: "tcp://192.168.1.100:8080", IP: "192.168.1.100", Port: 8080, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "IPv4 UDP explicit",
			input: "udp://192.168.1.100:53",
			want:  LANAccessRule{Raw: "udp://192.168.1.100:53", IP: "192.168.1.100", Port: 53, Protocol: ProtoUDP, IsIPv6: false},
		},
		{
			name:  "IPv4 all protocols explicit",
			input: "*://192.168.1.100:443",
			want:  LANAccessRule{Raw: "*://192.168.1.100:443", IP: "192.168.1.100", Port: 443, Protocol: ProtoAll, IsIPv6: false},
		},
		{
			name:  "TCP prefix with all ports",
			input: "tcp://192.168.1.100:*",
			want:  LANAccessRule{Raw: "tcp://192.168.1.100:*", IP: "192.168.1.100", Port: 0, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "UDP prefix without port",
			input: "udp://192.168.1.100",
			want:  LANAccessRule{Raw: "udp://192.168.1.100", IP: "192.168.1.100", Port: 0, Protocol: ProtoUDP, IsIPv6: false},
		},

		// IPv4 CIDR
		{
			name:  "IPv4 CIDR without port",
			input: "192.168.1.0/24",
			want:  LANAccessRule{Raw: "192.168.1.0/24", IP: "192.168.1.0/24", Port: 0, Protocol: ProtoAll, IsIPv6: false},
		},
		{
			name:  "IPv4 CIDR with port",
			input: "192.168.1.0/24:8080",
			want:  LANAccessRule{Raw: "192.168.1.0/24:8080", IP: "192.168.1.0/24", Port: 8080, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "IPv4 CIDR with TCP prefix",
			input: "tcp://10.0.0.0/8:*",
			want:  LANAccessRule{Raw: "tcp://10.0.0.0/8:*", IP: "10.0.0.0/8", Port: 0, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "IPv4 /32 single host",
			input: "10.10.42.230/32:8080",
			want:  LANAccessRule{Raw: "10.10.42.230/32:8080", IP: "10.10.42.230/32", Port: 8080, Protocol: ProtoTCP, IsIPv6: false},
		},

		// IPv6 without brackets (no port)
		{
			name:  "IPv6 link-local",
			input: "fe80::1",
			want:  LANAccessRule{Raw: "fe80::1", IP: "fe80::1", Port: 0, Protocol: ProtoAll, IsIPv6: true},
		},
		{
			name:  "IPv6 full address",
			input: "2001:db8::1",
			want:  LANAccessRule{Raw: "2001:db8::1", IP: "2001:db8::1", Port: 0, Protocol: ProtoAll, IsIPv6: true},
		},
		{
			name:  "IPv6 with protocol prefix no port",
			input: "tcp://fe80::1",
			want:  LANAccessRule{Raw: "tcp://fe80::1", IP: "fe80::1", Port: 0, Protocol: ProtoTCP, IsIPv6: true},
		},

		// IPv6 with brackets
		{
			name:  "IPv6 brackets no port",
			input: "[fe80::1]",
			want:  LANAccessRule{Raw: "[fe80::1]", IP: "fe80::1", Port: 0, Protocol: ProtoAll, IsIPv6: true},
		},
		{
			name:  "IPv6 brackets with port",
			input: "[fe80::1]:8080",
			want:  LANAccessRule{Raw: "[fe80::1]:8080", IP: "fe80::1", Port: 8080, Protocol: ProtoTCP, IsIPv6: true},
		},
		{
			name:  "IPv6 brackets with wildcard port",
			input: "[2001:db8::1]:*",
			want:  LANAccessRule{Raw: "[2001:db8::1]:*", IP: "2001:db8::1", Port: 0, Protocol: ProtoAll, IsIPv6: true},
		},
		{
			name:  "IPv6 with TCP prefix and port",
			input: "tcp://[2001:db8::1]:443",
			want:  LANAccessRule{Raw: "tcp://[2001:db8::1]:443", IP: "2001:db8::1", Port: 443, Protocol: ProtoTCP, IsIPv6: true},
		},
		{
			name:  "IPv6 with UDP prefix and port",
			input: "udp://[fe80::1]:53",
			want:  LANAccessRule{Raw: "udp://[fe80::1]:53", IP: "fe80::1", Port: 53, Protocol: ProtoUDP, IsIPv6: true},
		},
		{
			name:  "IPv6 with all protocols and port",
			input: "*://[2001:db8::1]:443",
			want:  LANAccessRule{Raw: "*://[2001:db8::1]:443", IP: "2001:db8::1", Port: 443, Protocol: ProtoAll, IsIPv6: true},
		},

		// IPv6 CIDR
		{
			name:  "IPv6 CIDR without port",
			input: "2001:db8::/32",
			want:  LANAccessRule{Raw: "2001:db8::/32", IP: "2001:db8::/32", Port: 0, Protocol: ProtoAll, IsIPv6: true},
		},
		{
			name:  "IPv6 CIDR with brackets and port",
			input: "[2001:db8::/32]:8080",
			want:  LANAccessRule{Raw: "[2001:db8::/32]:8080", IP: "2001:db8::/32", Port: 8080, Protocol: ProtoTCP, IsIPv6: true},
		},
		{
			name:  "IPv6 CIDR with brackets and wildcard port",
			input: "[fe80::/10]:*",
			want:  LANAccessRule{Raw: "[fe80::/10]:*", IP: "fe80::/10", Port: 0, Protocol: ProtoAll, IsIPv6: true},
		},

		// Edge cases
		{
			name:  "whitespace trimmed",
			input: "  192.168.1.100:8080  ",
			want:  LANAccessRule{Raw: "  192.168.1.100:8080  ", IP: "192.168.1.100", Port: 8080, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "port 1 (minimum)",
			input: "192.168.1.100:1",
			want:  LANAccessRule{Raw: "192.168.1.100:1", IP: "192.168.1.100", Port: 1, Protocol: ProtoTCP, IsIPv6: false},
		},
		{
			name:  "port 65535 (maximum)",
			input: "192.168.1.100:65535",
			want:  LANAccessRule{Raw: "192.168.1.100:65535", IP: "192.168.1.100", Port: 65535, Protocol: ProtoTCP, IsIPv6: false},
		},

		// Error cases
		{
			name:     "empty string",
			input:    "",
			wantErr:  true,
			errMatch: "empty rule string",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			wantErr:  true,
			errMatch: "empty rule string",
		},
		{
			name:     "invalid IP",
			input:    "999.999.999.999",
			wantErr:  true,
			errMatch: "invalid IP address",
		},
		{
			name:     "invalid IP with port",
			input:    "not.an.ip:8080",
			wantErr:  true,
			errMatch: "invalid IP address",
		},
		{
			name:     "port out of range high",
			input:    "192.168.1.100:65536",
			wantErr:  true,
			errMatch: "port 65536 out of range",
		},
		{
			name:     "port out of range zero",
			input:    "192.168.1.100:0",
			wantErr:  true,
			errMatch: "port 0 out of range",
		},
		{
			name:     "port negative",
			input:    "192.168.1.100:-1",
			wantErr:  true,
			errMatch: "out of range",
		},
		{
			name:     "port non-numeric",
			input:    "192.168.1.100:abc",
			wantErr:  true,
			errMatch: "invalid port",
		},
		{
			name:     "invalid CIDR prefix",
			input:    "192.168.1.0/33",
			wantErr:  true,
			errMatch: "invalid CIDR",
		},
		{
			name:     "invalid IPv6 CIDR prefix",
			input:    "2001:db8::/129",
			wantErr:  true,
			errMatch: "invalid CIDR",
		},
		{
			name:     "IPv6 missing closing bracket",
			input:    "[fe80::1:8080",
			wantErr:  true,
			errMatch: "missing closing bracket",
		},
		{
			name:     "IPv6 garbage after bracket",
			input:    "[fe80::1]garbage",
			wantErr:  true,
			errMatch: "unexpected characters",
		},
		{
			name:     "malformed CIDR",
			input:    "192.168.1.0/",
			wantErr:  true,
			errMatch: "invalid CIDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLANAccessRule(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLANAccessRule(%q) expected error containing %q, got nil", tt.input, tt.errMatch)
					return
				}
				if tt.errMatch != "" && !contains(err.Error(), tt.errMatch) {
					t.Errorf("ParseLANAccessRule(%q) error = %q, want error containing %q", tt.input, err.Error(), tt.errMatch)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseLANAccessRule(%q) unexpected error: %v", tt.input, err)
				return
			}

			if got.Raw != tt.want.Raw {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.want.Raw)
			}
			if got.IP != tt.want.IP {
				t.Errorf("IP = %q, want %q", got.IP, tt.want.IP)
			}
			if got.Port != tt.want.Port {
				t.Errorf("Port = %d, want %d", got.Port, tt.want.Port)
			}
			if got.Protocol != tt.want.Protocol {
				t.Errorf("Protocol = %v, want %v", got.Protocol, tt.want.Protocol)
			}
			if got.IsIPv6 != tt.want.IsIPv6 {
				t.Errorf("IsIPv6 = %v, want %v", got.IsIPv6, tt.want.IsIPv6)
			}
			if got.AllLAN != tt.want.AllLAN {
				t.Errorf("AllLAN = %v, want %v", got.AllLAN, tt.want.AllLAN)
			}
		})
	}
}

func TestParseLANAccessRules(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
		wantErr bool
	}{
		{
			name:    "empty slice",
			input:   []string{},
			wantLen: 0,
		},
		{
			name:    "single rule",
			input:   []string{"192.168.1.100:8080"},
			wantLen: 1,
		},
		{
			name:    "multiple valid rules",
			input:   []string{"*", "192.168.1.100", "tcp://10.0.0.1:443", "[fe80::1]:8080"},
			wantLen: 4,
		},
		{
			name:    "error on invalid rule",
			input:   []string{"192.168.1.100", "invalid.ip", "10.0.0.1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLANAccessRules(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(rules) = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestHasAllLAN(t *testing.T) {
	tests := []struct {
		name  string
		rules []LANAccessRule
		want  bool
	}{
		{
			name:  "empty rules",
			rules: []LANAccessRule{},
			want:  false,
		},
		{
			name: "no AllLAN",
			rules: []LANAccessRule{
				{IP: "192.168.1.100"},
				{IP: "10.0.0.1"},
			},
			want: false,
		},
		{
			name: "has AllLAN first",
			rules: []LANAccessRule{
				{AllLAN: true},
				{IP: "10.0.0.1"},
			},
			want: true,
		},
		{
			name: "has AllLAN last",
			rules: []LANAccessRule{
				{IP: "192.168.1.100"},
				{AllLAN: true},
			},
			want: true,
		},
		{
			name: "only AllLAN",
			rules: []LANAccessRule{
				{AllLAN: true},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasAllLAN(tt.rules)
			if got != tt.want {
				t.Errorf("HasAllLAN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProtocolString(t *testing.T) {
	tests := []struct {
		proto Protocol
		want  string
	}{
		{ProtoAll, "*"},
		{ProtoTCP, "tcp"},
		{ProtoUDP, "udp"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.proto.String(); got != tt.want {
				t.Errorf("Protocol.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// contains checks if substr is in s (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
