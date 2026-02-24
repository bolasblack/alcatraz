package config

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateAlcaTokens(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErrIs error
	}{
		{
			name:  "valid known token",
			input: "${alca:HOST_IP}:8080",
		},
		{
			name:  "no tokens",
			input: "no tokens here",
		},
		{
			name:  "env var not alca token",
			input: "${SOME_VAR}",
		},
		{
			name:      "unknown token",
			input:     "${alca:UNKNOWN_TOKEN}",
			wantErrIs: ErrUnknownAlcaToken,
		},
		{
			name:      "invalid syntax hyphen",
			input:     "${alca:bad-name}",
			wantErrIs: ErrInvalidAlcaToken,
		},
		{
			name:      "invalid syntax empty name",
			input:     "${alca:}",
			wantErrIs: ErrInvalidAlcaToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAlcaTokens(tt.input)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("expected %v, got: %v", tt.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestExpandAlcaTokens(t *testing.T) {
	hostResolver := func(name string) (string, error) {
		if name == "HOST_IP" {
			return "172.17.0.1", nil
		}
		return "", fmt.Errorf("unexpected token: %s", name)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "expand single token",
			input: "${alca:HOST_IP}:8080",
			want:  "172.17.0.1:8080",
		},
		{
			name:  "expand multiple tokens",
			input: "${alca:HOST_IP}:8080,${alca:HOST_IP}:9090",
			want:  "172.17.0.1:8080,172.17.0.1:9090",
		},
		{
			name:  "no tokens passthrough",
			input: "192.168.1.1:8080",
			want:  "192.168.1.1:8080",
		},
		{
			name:  "env var syntax untouched",
			input: "${HOME}/path",
			want:  "${HOME}/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandAlcaTokens(tt.input, hostResolver)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("resolver error propagated", func(t *testing.T) {
		resolverErr := errors.New("network unavailable")
		failResolver := func(string) (string, error) {
			return "", resolverErr
		}
		_, err := ExpandAlcaTokens("${alca:HOST_IP}", failResolver)
		if !errors.Is(err, resolverErr) {
			t.Fatalf("expected resolver error, got: %v", err)
		}
	})

	t.Run("unknown token errors before resolver", func(t *testing.T) {
		called := false
		spyResolver := func(string) (string, error) {
			called = true
			return "", nil
		}
		_, err := ExpandAlcaTokens("${alca:UNKNOWN_TOKEN}", spyResolver)
		if !errors.Is(err, ErrUnknownAlcaToken) {
			t.Fatalf("expected ErrUnknownAlcaToken, got: %v", err)
		}
		if called {
			t.Error("resolver should not have been called for unknown token")
		}
	})
}

func TestExpandAlcaTokensInStrings(t *testing.T) {
	hostResolver := func(name string) (string, error) {
		if name == "HOST_IP" {
			return "172.17.0.1", nil
		}
		return "", fmt.Errorf("unexpected token: %s", name)
	}

	t.Run("no tokens returns original slice", func(t *testing.T) {
		input := []string{"192.168.1.1:8080", "10.0.0.1:9090"}
		got, err := ExpandAlcaTokensInStrings(input, hostResolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Fatalf("got %d strings, want %d", len(got), len(input))
		}
		for i := range input {
			if got[i] != input[i] {
				t.Errorf("index %d: got %q, want %q", i, got[i], input[i])
			}
		}
	})

	t.Run("mixed tokens and plain strings", func(t *testing.T) {
		input := []string{"${alca:HOST_IP}:8080", "192.168.1.1:9090", "${alca:HOST_IP}:3000"}
		got, err := ExpandAlcaTokensInStrings(input, hostResolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"172.17.0.1:8080", "192.168.1.1:9090", "172.17.0.1:3000"}
		if len(got) != len(want) {
			t.Fatalf("got %d strings, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		got, err := ExpandAlcaTokensInStrings(nil, hostResolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got, err := ExpandAlcaTokensInStrings([]string{}, hostResolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %v", got)
		}
	})

	t.Run("resolver error propagated", func(t *testing.T) {
		resolverErr := errors.New("network unavailable")
		failResolver := func(string) (string, error) {
			return "", resolverErr
		}
		_, err := ExpandAlcaTokensInStrings([]string{"${alca:HOST_IP}:8080"}, failResolver)
		if !errors.Is(err, resolverErr) {
			t.Fatalf("expected resolver error, got: %v", err)
		}
	})
}

func TestContainsAlcaTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "contains token",
			input: "${alca:HOST_IP}:8080",
			want:  true,
		},
		{
			name:  "no token",
			input: "192.168.1.1:8080",
			want:  false,
		},
		{
			name:  "env var not alca token",
			input: "${HOME}/path",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsAlcaTokens(tt.input); got != tt.want {
				t.Errorf("ContainsAlcaTokens(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
