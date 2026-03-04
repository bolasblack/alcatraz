# Group 2: Lifecycle (requires Docker)
# Sourced by run.sh — no shebang needed.

test_lifecycle_basic() {
  setup_test_dir
  write_lifecycle_config

  # Up
  if ! run_with_timeout 120 "$ALCA_BIN" up -q 2>&1; then
    fail "lifecycle_basic: alca up" "alca up failed"
    teardown_test_dir
    return
  fi
  pass "lifecycle_basic: alca up"

  # Status — should indicate running
  local status_output
  status_output=$(run_with_timeout 30 "$ALCA_BIN" status 2>&1 || true)
  if echo "$status_output" | grep -qi "running"; then
    pass "lifecycle_basic: status shows running"
  else
    fail "lifecycle_basic: status shows running" "output: $status_output"
  fi

  # Run echo
  local run_output
  run_output=$(run_with_timeout 30 "$ALCA_BIN" run echo hello < /dev/null 2>&1 || true)
  assert_stdout_contains "$run_output" "hello" "lifecycle_basic: run echo hello"

  # Down
  if run_with_timeout 30 "$ALCA_BIN" down 2>&1; then
    pass "lifecycle_basic: alca down"
  else
    fail "lifecycle_basic: alca down" "alca down failed"
  fi

  # Verify container is gone
  local post_down_status
  post_down_status=$(run_with_timeout 30 "$ALCA_BIN" status 2>&1 || true)
  if echo "$post_down_status" | grep -qi "running"; then
    fail "lifecycle_basic: container gone after down" "status still shows running"
  else
    pass "lifecycle_basic: container gone after down"
  fi

  teardown_test_dir
}

