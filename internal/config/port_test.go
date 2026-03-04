package config

import (
	"errors"
	"testing"

	"github.com/spf13/afero"
)

func TestFormatPortArg(t *testing.T) {
	tests := []struct {
		name string
		port PortConfig
		want string
	}{
		{
			name: "container port only",
			port: PortConfig{Port: 8080},
			want: "8080:8080",
		},
		{
			name: "with host port",
			port: PortConfig{Port: 3000, HostPort: 3001},
			want: "3001:3000",
		},
		{
			name: "with host IP",
			port: PortConfig{Port: 5432, HostIP: "127.0.0.1"},
			want: "127.0.0.1:5432:5432",
		},
		{
			name: "with UDP protocol",
			port: PortConfig{Port: 53, Protocol: "udp"},
			want: "53:53/udp",
		},
		{
			name: "full spec",
			port: PortConfig{Port: 80, HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp"},
			want: "0.0.0.0:8080:80",
		},
		{
			name: "host IP with UDP",
			port: PortConfig{Port: 53, HostIP: "127.0.0.1", Protocol: "udp"},
			want: "127.0.0.1:53:53/udp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPortArg(tt.port)
			if got != tt.want {
				t.Errorf("FormatPortArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidatePorts(t *testing.T) {
	tests := []struct {
		name    string
		ports   []PortConfig
		wantErr error
	}{
		{
			name:    "valid single port",
			ports:   []PortConfig{{Port: 8080}},
			wantErr: nil,
		},
		{
			name: "valid multiple ports",
			ports: []PortConfig{
				{Port: 8080},
				{Port: 3000, HostPort: 3001},
				{Port: 5432, HostIP: "127.0.0.1"},
				{Port: 53, Protocol: "udp"},
			},
			wantErr: nil,
		},
		{
			name:    "empty slice is valid",
			ports:   nil,
			wantErr: nil,
		},
		{
			name:    "port zero",
			ports:   []PortConfig{{Port: 0}},
			wantErr: ErrInvalidPort,
		},
		{
			name:    "port negative",
			ports:   []PortConfig{{Port: -1}},
			wantErr: ErrInvalidPort,
		},
		{
			name:    "port too high",
			ports:   []PortConfig{{Port: 65536}},
			wantErr: ErrInvalidPort,
		},
		{
			name:    "host port too high",
			ports:   []PortConfig{{Port: 80, HostPort: 70000}},
			wantErr: ErrInvalidPort,
		},
		{
			name:    "invalid protocol",
			ports:   []PortConfig{{Port: 80, Protocol: "sctp"}},
			wantErr: ErrInvalidProtocol,
		},
		{
			name:    "invalid host IP",
			ports:   []PortConfig{{Port: 80, HostIP: "not-an-ip"}},
			wantErr: ErrInvalidHostIP,
		},
		{
			name:    "boundary port 1",
			ports:   []PortConfig{{Port: 1}},
			wantErr: nil,
		},
		{
			name:    "boundary port 65535",
			ports:   []PortConfig{{Port: 65535}},
			wantErr: nil,
		},
		{
			name:    "tcp protocol explicit",
			ports:   []PortConfig{{Port: 80, Protocol: "tcp"}},
			wantErr: nil,
		},
		{
			name:    "udp protocol",
			ports:   []PortConfig{{Port: 53, Protocol: "udp"}},
			wantErr: nil,
		},
		{
			name:    "IPv6 host IP",
			ports:   []PortConfig{{Port: 80, HostIP: "::1"}},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePorts(tt.ports)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidatePorts() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("ValidatePorts() expected error %v, got nil", tt.wantErr)
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidatePorts() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPortsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []PortConfig
		b    []PortConfig
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "both empty",
			a:    []PortConfig{},
			b:    []PortConfig{},
			want: true,
		},
		{
			name: "equal single",
			a:    []PortConfig{{Port: 8080}},
			b:    []PortConfig{{Port: 8080}},
			want: true,
		},
		{
			name: "equal full",
			a:    []PortConfig{{Port: 80, HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp"}},
			b:    []PortConfig{{Port: 80, HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp"}},
			want: true,
		},
		{
			name: "different port",
			a:    []PortConfig{{Port: 8080}},
			b:    []PortConfig{{Port: 9090}},
			want: false,
		},
		{
			name: "different host port",
			a:    []PortConfig{{Port: 80, HostPort: 8080}},
			b:    []PortConfig{{Port: 80, HostPort: 9090}},
			want: false,
		},
		{
			name: "different host IP",
			a:    []PortConfig{{Port: 80, HostIP: "127.0.0.1"}},
			b:    []PortConfig{{Port: 80, HostIP: "0.0.0.0"}},
			want: false,
		},
		{
			name: "different protocol",
			a:    []PortConfig{{Port: 53, Protocol: "tcp"}},
			b:    []PortConfig{{Port: 53, Protocol: "udp"}},
			want: false,
		},
		{
			name: "different length",
			a:    []PortConfig{{Port: 80}},
			b:    []PortConfig{{Port: 80}, {Port: 443}},
			want: false,
		},
		{
			name: "nil vs non-nil empty",
			a:    nil,
			b:    []PortConfig{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PortsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("PortsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePortString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    PortConfig
		wantErr error
	}{
		{
			name:  "container port only",
			input: "8080",
			want:  PortConfig{Port: 8080},
		},
		{
			name:  "host and container port",
			input: "3001:3000",
			want:  PortConfig{Port: 3000, HostPort: 3001},
		},
		{
			name:  "IP host and container port",
			input: "127.0.0.1:5432:5432",
			want:  PortConfig{Port: 5432, HostIP: "127.0.0.1", HostPort: 5432},
		},
		{
			name:  "with UDP protocol",
			input: "53:53/udp",
			want:  PortConfig{Port: 53, HostPort: 53, Protocol: "udp"},
		},
		{
			name:  "full form",
			input: "0.0.0.0:8080:80/tcp",
			want:  PortConfig{Port: 80, HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp"},
		},
		// Error cases
		{
			name:    "empty string",
			input:   "",
			wantErr: ErrInvalidPortFormat,
		},
		{
			name:    "non-numeric",
			input:   "abc",
			wantErr: ErrInvalidPortFormat,
		},
		{
			name:    "port zero",
			input:   "0",
			wantErr: ErrInvalidPort,
		},
		{
			name:    "port out of range",
			input:   "99999",
			wantErr: ErrInvalidPort,
		},
		{
			name:    "invalid protocol",
			input:   "8080/ftp",
			wantErr: ErrInvalidProtocol,
		},
		{
			name:    "malformed leading colon",
			input:   ":8080",
			wantErr: ErrInvalidPortFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePortString(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !portEqual(got, tt.want) {
				t.Errorf("ParsePortString(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadConfigWithMixedPorts(t *testing.T) {
	content := `
image = "ubuntu:latest"

[network]
ports = [
  "8080",
  "3001:3000",
  "127.0.0.1:5432:5432",
  "53:53/udp",
  { port = 9090 },
]
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	if err := afero.WriteFile(memFs, path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(env, path, noExpandEnv)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Network.Ports) != 5 {
		t.Fatalf("expected 5 ports, got %d", len(cfg.Network.Ports))
	}

	expectations := []PortConfig{
		{Port: 8080},
		{Port: 3000, HostPort: 3001},
		{Port: 5432, HostIP: "127.0.0.1", HostPort: 5432},
		{Port: 53, HostPort: 53, Protocol: "udp"},
		{Port: 9090},
	}
	for i, want := range expectations {
		got := cfg.Network.Ports[i]
		if !portEqual(got, want) {
			t.Errorf("port[%d] = %+v, want %+v", i, got, want)
		}
	}
}

func TestLoadConfigWithPorts(t *testing.T) {
	content := `
image = "ubuntu:latest"

[network]
ports = [
  { port = 8080 },
  { port = 3000, hostPort = 3001 },
  { port = 5432, hostIp = "127.0.0.1" },
  { port = 53, protocol = "udp" },
  { port = 80, hostIp = "0.0.0.0", hostPort = 8080, protocol = "tcp" },
]
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	if err := afero.WriteFile(memFs, path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(env, path, noExpandEnv)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Network.Ports) != 5 {
		t.Fatalf("expected 5 ports, got %d", len(cfg.Network.Ports))
	}

	// Verify each port parsed correctly
	expectations := []PortConfig{
		{Port: 8080},
		{Port: 3000, HostPort: 3001},
		{Port: 5432, HostIP: "127.0.0.1"},
		{Port: 53, Protocol: "udp"},
		{Port: 80, HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp"},
	}
	for i, want := range expectations {
		got := cfg.Network.Ports[i]
		if !portEqual(got, want) {
			t.Errorf("port[%d] = %+v, want %+v", i, got, want)
		}
	}
}

func TestLoadConfigWithInvalidPorts(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr error
	}{
		{
			name: "port zero",
			toml: `
image = "ubuntu:latest"
[network]
ports = [{ port = 0 }]
`,
			wantErr: ErrInvalidPort,
		},
		{
			name: "invalid protocol",
			toml: `
image = "ubuntu:latest"
[network]
ports = [{ port = 80, protocol = "icmp" }]
`,
			wantErr: ErrInvalidProtocol,
		},
		{
			name: "invalid host IP",
			toml: `
image = "ubuntu:latest"
[network]
ports = [{ port = 80, hostIp = "bad" }]
`,
			wantErr: ErrInvalidHostIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, memFs := newTestEnv(t)
			path := "/test/.alca.toml"
			if err := afero.WriteFile(memFs, path, []byte(tt.toml), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			_, err := LoadConfig(env, path, noExpandEnv)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
