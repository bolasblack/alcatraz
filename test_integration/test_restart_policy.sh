# Group: Container restart policy (requires Docker)
# Sourced by run.sh — no shebang needed.

# test_restart_policy: containers are created with --restart=unless-stopped
test_restart_policy() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "restart_policy: alca up" "setup failed"; teardown_test_dir; return; }

  # Get the container name from status
  local status_output container_name restart_policy
  status_output=$(run_with_timeout 30 "$ALCA_BIN" status 2>&1 || true)

  # Find the alca container — it starts with "alca-"
  container_name=$($CONTAINER_RUNTIME ps --format '{{.Names}}' | grep '^alca-' | head -1)
  if [[ -z "$container_name" ]]; then
    fail "restart_policy: find container" "no alca- container found"
    teardown_test_dir
    return
  fi

  # Inspect the restart policy
  restart_policy=$($CONTAINER_RUNTIME inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$container_name" 2>&1 || true)
  assert_stdout_contains "$restart_policy" "unless-stopped" "restart_policy: container has unless-stopped policy"

  teardown_test_dir
}
