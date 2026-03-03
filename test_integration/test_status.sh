# Group 3: Status Variations (requires Docker)
# Sourced by run.sh — no shebang needed.

test_status_not_running() {
  setup_test_dir
  write_lifecycle_config

  local output exit_code=0
  output=$(run_with_timeout 30 "$ALCA_BIN" status 2>&1) || exit_code=$?

  # Should not crash
  if echo "$output" | grep -qiF "panic"; then
    fail "status_not_running: no crash" "output contains panic: $output"
    teardown_test_dir
    return
  fi
  pass "status_not_running: no crash"

  # Should indicate container is not running
  if echo "$output" | grep -qi "not running\|stopped\|not found\|no container"; then
    pass "status_not_running: reports not running"
  else
    fail "status_not_running: reports not running" "output: $output"
  fi

  teardown_test_dir
}
