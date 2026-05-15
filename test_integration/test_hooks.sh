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

# test_hooks_container_env: hooks receive ALCA_CONTAINER_NAME and ALCA_CONTAINER_ID.
# The hook writes both env vars to disk so we can assert they are non-empty and
# resolvable to a real container via docker inspect.
test_hooks_container_env() {
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
post_up = "printf '%s\\n%s\\n' \"$ALCA_CONTAINER_NAME\" \"$ALCA_CONTAINER_ID\" > post_up.env"
pre_down = "printf '%s\\n%s\\n' \"$ALCA_CONTAINER_NAME\" \"$ALCA_CONTAINER_ID\" > pre_down.env"
TOML

  if ! run_with_timeout 120 "$ALCA_BIN" up -q 2>&1; then
    fail "hooks_container_env: alca up" "alca up failed"
    teardown_test_dir
    return
  fi

  assert_file_exists "$CURRENT_TEST_DIR/post_up.env" "hooks_container_env: post_up env captured"

  local env_name env_id
  env_name=$(sed -n '1p' "$CURRENT_TEST_DIR/post_up.env")
  env_id=$(sed -n '2p' "$CURRENT_TEST_DIR/post_up.env")

  if [[ -z "$env_name" ]]; then
    fail "hooks_container_env: ALCA_CONTAINER_NAME set" "empty in post_up"
  else
    pass "hooks_container_env: ALCA_CONTAINER_NAME set ($env_name)"
  fi

  if [[ -z "$env_id" || ${#env_id} -lt 12 ]]; then
    fail "hooks_container_env: ALCA_CONTAINER_ID set" "short/empty: '$env_id'"
  else
    pass "hooks_container_env: ALCA_CONTAINER_ID set"
  fi

  # Name should resolve to a real running container.
  if docker inspect --format '{{.Id}}' "$env_name" >/dev/null 2>&1; then
    pass "hooks_container_env: ALCA_CONTAINER_NAME resolves via docker inspect"
  else
    fail "hooks_container_env: name resolves" "docker inspect '$env_name' failed"
  fi

  if ! run_with_timeout 30 "$ALCA_BIN" down 2>&1; then
    fail "hooks_container_env: alca down" "alca down failed"
    teardown_test_dir
    return
  fi

  assert_file_exists "$CURRENT_TEST_DIR/pre_down.env" "hooks_container_env: pre_down env captured"
  local pre_name
  pre_name=$(sed -n '1p' "$CURRENT_TEST_DIR/pre_down.env")
  if [[ "$pre_name" == "$env_name" ]]; then
    pass "hooks_container_env: pre_down sees same container name"
  else
    fail "hooks_container_env: pre_down container name" "got '$pre_name' vs '$env_name'"
  fi

  teardown_test_dir
}
