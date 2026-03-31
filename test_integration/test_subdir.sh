# Group: Subdirectory project discovery (requires Docker)
# Sourced by run.sh — no shebang needed.

# test_subdir_status: alca status works from a subdirectory
test_subdir_status() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "subdir_status: alca up" "setup failed"; teardown_test_dir; return; }

  # Create a subdirectory and run status from it
  mkdir -p src/pkg
  cd src/pkg

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" status 2>&1 || true)
  if echo "$output" | grep -qi "running"; then
    pass "subdir_status: status works from subdirectory"
  else
    fail "subdir_status: status works from subdirectory" "output: $output"
  fi

  # Return to project root for teardown
  cd "$CURRENT_TEST_DIR"
  teardown_test_dir
}

# test_subdir_run: alca run works from a subdirectory
test_subdir_run() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "subdir_run: alca up" "setup failed"; teardown_test_dir; return; }

  # Run a command from a nested subdirectory
  mkdir -p deep/nested/dir
  cd deep/nested/dir

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" run echo hello < /dev/null 2>&1 || true)
  assert_stdout_contains "$output" "hello" "subdir_run: run works from subdirectory"

  cd "$CURRENT_TEST_DIR"
  teardown_test_dir
}
