package config

import (
	"errors"
	"testing"
)

func TestStrictExpandEnv(t *testing.T) {
	t.Run("known var expands correctly", func(t *testing.T) {
		t.Setenv("STRICT_TEST_VAR", "/home/testuser")

		got, err := StrictExpandEnv("${STRICT_TEST_VAR}/data")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/home/testuser/data" {
			t.Errorf("got %q, want %q", got, "/home/testuser/data")
		}
	})

	t.Run("undefined var returns error containing var name", func(t *testing.T) {
		_, err := StrictExpandEnv("${STRICT_TEST_UNDEFINED_XYZ}")
		if !errors.Is(err, ErrUndefinedEnvVar) {
			t.Fatalf("expected ErrUndefinedEnvVar, got: %v", err)
		}
	})

	t.Run("multiple vars one undefined returns error", func(t *testing.T) {
		t.Setenv("STRICT_TEST_DEFINED", "ok")

		_, err := StrictExpandEnv("${STRICT_TEST_DEFINED}/${STRICT_TEST_MISSING_ABC}")
		if !errors.Is(err, ErrUndefinedEnvVar) {
			t.Fatalf("expected ErrUndefinedEnvVar, got: %v", err)
		}
	})

	t.Run("no vars in string passthrough unchanged", func(t *testing.T) {
		got, err := StrictExpandEnv("/static/path/no/vars")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/static/path/no/vars" {
			t.Errorf("got %q, want %q", got, "/static/path/no/vars")
		}
	})

	t.Run("partial interpolation works", func(t *testing.T) {
		t.Setenv("STRICT_TEST_PARTIAL", "middle")

		got, err := StrictExpandEnv("prefix${STRICT_TEST_PARTIAL}suffix")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "prefixmiddlesuffix" {
			t.Errorf("got %q, want %q", got, "prefixmiddlesuffix")
		}
	})
}
