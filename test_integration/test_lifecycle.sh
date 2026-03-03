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
  run_output=$(run_with_timeout 30 "$ALCA_BIN" run echo hello 2>&1)
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

test_run_args_propagation() {
  setup_test_dir
  write_lifecycle_config
  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "run_args: alca up" "setup failed"; teardown_test_dir; return; }

  # Multiple args
  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" run echo foo bar baz 2>&1)
  assert_stdout_contains "$output" "foo bar baz" "run_args: echo foo bar baz"

  # Arithmetic with unique result
  output=$(run_with_timeout 30 "$ALCA_BIN" run sh -c 'echo $((12345+54321))' 2>&1)
  assert_stdout_contains "$output" "66666" "run_args: sh -c arithmetic"

  # Args with spaces
  output=$(run_with_timeout 30 "$ALCA_BIN" run echo "hello world" 2>&1)
  assert_stdout_contains "$output" "hello world" "run_args: echo with spaces"

  teardown_test_dir
}

test_run_exit_code() {
  setup_test_dir
  write_lifecycle_config
  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "run_exit_code: alca up" "setup failed"; teardown_test_dir; return; }

  # true → exit 0
  if run_with_timeout 30 "$ALCA_BIN" run true 2>/dev/null; then
    pass "run_exit_code: true exits 0"
  else
    fail "run_exit_code: true exits 0" "expected exit 0"
  fi

  # false → exit non-zero
  if run_with_timeout 30 "$ALCA_BIN" run false 2>/dev/null; then
    fail "run_exit_code: false exits non-zero" "expected non-zero exit"
  else
    pass "run_exit_code: false exits non-zero"
  fi

  # Specific exit code propagation
  local exit_code=0
  run_with_timeout 30 "$ALCA_BIN" run sh -c 'exit 42' 2>/dev/null || exit_code=$?
  if [[ $exit_code -eq 42 ]]; then
    pass "run_exit_code: exit 42 propagated"
  else
    fail "run_exit_code: exit 42 propagated" "expected 42, got $exit_code"
  fi

  teardown_test_dir
}

test_list() {
  setup_test_dir
  write_lifecycle_config
  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "list: alca up" "setup failed"; teardown_test_dir; return; }

  # List should show our container
  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" list 2>&1)
  if [[ -n "$output" ]] && echo "$output" | grep -qi "alca\|running\|container"; then
    pass "list: shows container after up"
  else
    fail "list: shows container after up" "output: $output"
  fi

  run_with_timeout 30 "$ALCA_BIN" down 2>&1 || true

  # After down, list should not show a running container for this project
  output=$(run_with_timeout 30 "$ALCA_BIN" list 2>&1 || true)
  if echo "$output" | grep -qi "running"; then
    fail "list: no running container after down" "output: $output"
  else
    pass "list: no running container after down"
  fi

  teardown_test_dir
}

test_up_idempotent() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "up_idempotent: first up" "first alca up failed"; teardown_test_dir; return; }
  pass "up_idempotent: first up"

  if run_with_timeout 120 "$ALCA_BIN" up -q -f 2>&1; then
    pass "up_idempotent: second up"
  else
    fail "up_idempotent: second up" "second alca up failed"
  fi

  teardown_test_dir
}

test_down_idempotent() {
  setup_test_dir
  write_lifecycle_config
  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "down_idempotent: alca up" "setup failed"; teardown_test_dir; return; }

  # First down — should succeed
  if run_with_timeout 30 "$ALCA_BIN" down 2>&1; then
    pass "down_idempotent: first down"
  else
    fail "down_idempotent: first down" "alca down failed"
    teardown_test_dir
    return
  fi

  # Second down — should not error on already-stopped container
  if run_with_timeout 30 "$ALCA_BIN" down 2>&1; then
    pass "down_idempotent: second down"
  else
    fail "down_idempotent: second down" "alca down failed on already-stopped container"
  fi

  teardown_test_dir
}
