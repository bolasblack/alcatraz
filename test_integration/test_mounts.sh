# Group 6: Mounts (requires container runtime)
# Sourced by run.sh — no shebang needed.
#
# test_mount_persistence: bind mount data survives restart
# test_workdir_exclude: excluded files not visible inside container (requires Mutagen)

mutagen_available() {
  command -v mutagen >/dev/null 2>&1
}

test_mount_persistence() {
  setup_test_dir
  mkdir -p .alca.mounts/data

  cat > .alca.toml <<'TOML'
image = "alpine:3.21"
mounts = [".alca.mounts/data:/data"]

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
true
"""
TOML

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "mount_persistence: first up" "setup failed"; teardown_test_dir; return; }

  # Write data inside container
  run_with_timeout 30 "$ALCA_BIN" run sh -c 'echo persist > /data/test.txt' 2>&1 || {
    fail "mount_persistence: write data" "run failed"
    teardown_test_dir
    return
  }

  # Restart container
  run_with_timeout 30 "$ALCA_BIN" down 2>&1 || true
  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "mount_persistence: second up" "restart failed"; teardown_test_dir; return; }

  # Read data back from container
  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" run cat /data/test.txt 2>&1)
  assert_stdout_contains "$output" "persist" "mount_persistence: data survives restart"

  # Verify host file
  assert_file_exists ".alca.mounts/data/test.txt" "mount_persistence: host file exists"
  assert_file_contains ".alca.mounts/data/test.txt" "persist" "mount_persistence: host file has data"

  teardown_test_dir
}

test_workdir_exclude() {
  if ! mutagen_available; then
    echo "  SKIP: test_workdir_exclude — Mutagen not installed"
    return
  fi

  setup_test_dir

  # Create files that should be excluded
  echo "secret" > .env
  mkdir -p node_modules
  echo "pkg" > node_modules/test.js
  echo "visible" > keep-me.txt

  cat > .alca.toml <<'TOML'
image = "alpine:3.21"
workdir_exclude = [".env", "node_modules"]

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
true
"""
TOML

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "workdir_exclude: alca up" "setup failed"; teardown_test_dir; return; }

  # Excluded files should NOT be visible inside container
  local ls_output
  ls_output=$(run_with_timeout 30 "$ALCA_BIN" run ls -a /workspace 2>&1)

  if echo "$ls_output" | grep -qF ".env"; then
    fail "workdir_exclude: .env excluded" ".env visible inside container"
  else
    pass "workdir_exclude: .env excluded"
  fi

  if echo "$ls_output" | grep -qF "node_modules"; then
    fail "workdir_exclude: node_modules excluded" "node_modules visible inside container"
  else
    pass "workdir_exclude: node_modules excluded"
  fi

  # Non-excluded files SHOULD be visible
  if echo "$ls_output" | grep -qF "keep-me.txt"; then
    pass "workdir_exclude: non-excluded files visible"
  else
    fail "workdir_exclude: non-excluded files visible" "keep-me.txt not found in: $ls_output"
  fi

  teardown_test_dir
}
