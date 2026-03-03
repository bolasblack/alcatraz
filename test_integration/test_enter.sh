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
  output=$(run_with_timeout 30 "$ALCA_BIN" run env 2>&1)
  assert_stdout_contains "$output" "ALCA_TEST=integration" "run_enter_command: enter sets env var"

  teardown_test_dir
}

test_enter_multiline() {
  setup_test_dir

  cat > .alca.toml <<'TOML'
image = "alpine:3.21"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
export ALCA_FOO=hello
export ALCA_BAR=world
"""
TOML

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "enter_multiline: alca up" "setup failed"; teardown_test_dir; return; }

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" run env 2>&1)
  assert_stdout_contains "$output" "ALCA_FOO=hello" "enter_multiline: first env var set"
  assert_stdout_contains "$output" "ALCA_BAR=world" "enter_multiline: second env var set"

  teardown_test_dir
}

test_enter_concatenation_regression() {
  setup_test_dir

  cat > .alca.toml <<'TOML'
image = "alpine:3.21"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
. /dev/null
"""
TOML

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "enter_concat_regression: alca up" "setup failed"; teardown_test_dir; return; }

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" run echo canary 2>&1)
  # Without the newline fix, ". /dev/null echo canary" executes — "canary" never prints
  assert_stdout_contains "$output" "canary" "enter_concat_regression: enter doesn't eat run command"

  teardown_test_dir
}
