# Group: Host-side lifecycle hooks (requires Docker)
# Sourced by run.sh — no shebang needed.

# test_hooks_run: post_up fires after container is ready; pre_down fires before teardown.
# Hooks run on the host with cwd set to the project directory, so markers land in $CURRENT_TEST_DIR.
test_hooks_run() {
  setup_test_dir

  cat > .alca.toml <<'TOML'
image = "alpine:3.21"
runtime = "docker"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = "true"

[hooks]
post_up = "echo ok > post_up.marker"
pre_down = "echo ok > pre_down.marker"
TOML

  if ! run_with_timeout 120 "$ALCA_BIN" up -q 2>&1; then
    fail "hooks_run: alca up" "alca up failed"
    teardown_test_dir
    return
  fi

  assert_file_exists "$CURRENT_TEST_DIR/post_up.marker" "hooks_run: post_up ran after up"

  if [[ -f "$CURRENT_TEST_DIR/pre_down.marker" ]]; then
    fail "hooks_run: pre_down not yet run" "pre_down.marker exists before down"
  else
    pass "hooks_run: pre_down not yet run"
  fi

  if ! run_with_timeout 30 "$ALCA_BIN" down 2>&1; then
    fail "hooks_run: alca down" "alca down failed"
    teardown_test_dir
    return
  fi

  assert_file_exists "$CURRENT_TEST_DIR/pre_down.marker" "hooks_run: pre_down ran before down"

  teardown_test_dir
}
