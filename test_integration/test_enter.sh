# Group 5: Enter Command Variations (requires Docker)
# Sourced by run.sh — no shebang needed.

test_run_enter_command() {
  setup_test_dir

  # Config with enter that sets an env var
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
export ALCA_TEST=integration
"""
TOML

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "run_enter_command: alca up" "setup failed"; teardown_test_dir; return; }

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" run env < /dev/null 2>&1 || true)
  assert_stdout_contains "$output" "ALCA_TEST=integration" "run_enter_command: enter sets env var"

  teardown_test_dir
}

