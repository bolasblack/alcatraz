# Group 8: Cleanup Command (requires container runtime)
# Sourced by run.sh — no shebang needed.

test_cleanup_no_orphans() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "cleanup_no_orphans: alca up" "setup failed"; teardown_test_dir; return; }

  # Cleanup should find no orphans — project dir exists and state is valid
  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" cleanup 2>&1 || true)
  if echo "$output" | grep -qi "orphan"; then
    fail "cleanup_no_orphans: no orphans reported" "output: $output"
  else
    pass "cleanup_no_orphans: no orphans reported"
  fi

  teardown_test_dir
}
