# Group 4: Config Drift Detection (requires Docker)
# Sourced by run.sh — no shebang needed.

test_config_drift() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "config_drift: alca up" "setup failed"; teardown_test_dir; return; }

  # Modify config — change image
  cat > .alca.toml <<'TOML'
image = "alpine:3.20"
runtime = "docker"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
true
"""
TOML

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" status 2>&1 || true)
  if echo "$output" | grep -qi "changed\|drift\|rebuild"; then
    pass "config_drift: detects image change"
  else
    fail "config_drift: detects image change" "output: $output"
  fi

  run_with_timeout 30 "$ALCA_BIN" down --force 2>/dev/null || true
  teardown_test_dir
}
