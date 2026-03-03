# Group 1: Config (no container runtime needed)
# Sourced by run.sh — no shebang needed.

test_config_validation() {
  setup_test_dir

  # Valid config — alca status should parse without error
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"

[commands]
up = "true"
enter = """
true
"""
TOML

  if "$ALCA_BIN" status >/dev/null 2>&1; then
    pass "config_validation: valid config accepted"
  else
    # alca status may exit non-zero if container not running, which is fine.
    # We just need it to NOT crash with a parse error.
    local output
    output=$("$ALCA_BIN" status 2>&1 || true)
    if echo "$output" | grep -qi "parse\|syntax\|toml\|unmarshal"; then
      fail "config_validation: valid config accepted" "config parse error: $output"
    else
      pass "config_validation: valid config accepted"
    fi
  fi

  teardown_test_dir
}
