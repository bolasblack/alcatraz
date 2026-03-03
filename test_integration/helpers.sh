# Shared infrastructure for integration tests
# This file is sourced by run.sh — no shebang needed.

ALCA_BIN="${ALCA_BIN:-./out/bin/alca}"
PASSED=0
FAILED=0
FAIL_NAMES=()
CURRENT_TEST_DIR=""

# ---------------------------------------------------------------------------
# Assertions
# ---------------------------------------------------------------------------

pass() {
  echo "  PASS: $1"
  PASSED=$((PASSED + 1))
}

fail() {
  echo "  FAIL: $1 — $2"
  FAILED=$((FAILED + 1))
  FAIL_NAMES+=("$1")
}

assert_file_exists() {
  local file="$1" name="$2"
  if [[ -f "$file" ]]; then
    pass "$name"
  else
    fail "$name" "file not found: $file"
  fi
}

assert_file_contains() {
  local file="$1" pattern="$2" name="$3"
  if grep -qF "$pattern" "$file" 2>/dev/null; then
    pass "$name"
  else
    fail "$name" "pattern '$pattern' not found in $file"
  fi
}

assert_file_not_contains() {
  local file="$1" pattern="$2" name="$3"
  if ! grep -qF "$pattern" "$file" 2>/dev/null; then
    pass "$name"
  else
    fail "$name" "pattern '$pattern' unexpectedly found in $file"
  fi
}

assert_stdout_contains() {
  local output="$1" expected="$2" name="$3"
  if echo "$output" | grep -qF "$expected"; then
    pass "$name"
  else
    fail "$name" "expected '$expected' in stdout, got: $output"
  fi
}

assert_stdout_matches_regex() {
  local output="$1" pattern="$2" name="$3"
  if echo "$output" | grep -q "$pattern"; then
    pass "$name"
  else
    fail "$name" "regex '$pattern' not matched in stdout"
  fi
}

assert_count() {
  local file="$1" pattern="$2" expected_count="$3" name="$4"
  local actual_count
  actual_count=$(grep -c "$pattern" "$file" 2>/dev/null || true)
  if [[ "$actual_count" -eq "$expected_count" ]]; then
    pass "$name"
  else
    fail "$name" "expected $expected_count occurrences of '$pattern', got $actual_count"
  fi
}

# ---------------------------------------------------------------------------
# Test setup / teardown
# ---------------------------------------------------------------------------

setup_test_dir() {
  CURRENT_TEST_DIR="$(mktemp -d)"
  cd "$CURRENT_TEST_DIR"
}

teardown_test_dir() {
  if [[ -n "$CURRENT_TEST_DIR" && -d "$CURRENT_TEST_DIR" ]]; then
    cd "$CURRENT_TEST_DIR"
    "$ALCA_BIN" down --force 2>/dev/null || true
    cd /
    rm -rf "$CURRENT_TEST_DIR"
    CURRENT_TEST_DIR=""
  fi
}

cleanup_on_exit() {
  if [[ -n "$CURRENT_TEST_DIR" && -d "$CURRENT_TEST_DIR" ]]; then
    cd "$CURRENT_TEST_DIR"
    "$ALCA_BIN" down --force 2>/dev/null || true
    rm -rf "$CURRENT_TEST_DIR"
  fi
}

trap cleanup_on_exit EXIT

# ---------------------------------------------------------------------------
# Timeout wrapper
# ---------------------------------------------------------------------------

# Run a command with timeout (default 120s for up, 30s for others)
run_with_timeout() {
  local seconds="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "$seconds" "$@"
  else
    # macOS may not have timeout — run without it
    "$@"
  fi
}

# ---------------------------------------------------------------------------
# Container runtime detection
# ---------------------------------------------------------------------------

# Auto-detect container runtime. Priority: Podman on Linux, Docker elsewhere.
# Override with CONTAINER_RUNTIME=docker or CONTAINER_RUNTIME=podman.
detect_container_runtime() {
  if [[ -n "${CONTAINER_RUNTIME:-}" ]]; then
    if command -v "$CONTAINER_RUNTIME" >/dev/null 2>&1 && "$CONTAINER_RUNTIME" info >/dev/null 2>&1; then
      return 0
    fi
    return 1
  fi

  # Linux: prefer Podman (rootless-friendly), fall back to Docker
  # macOS: prefer Docker (Podman support is less mature)
  if [[ "$(uname -s)" == "Linux" ]]; then
    if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
      CONTAINER_RUNTIME="podman"
      return 0
    fi
  fi

  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    CONTAINER_RUNTIME="docker"
    return 0
  fi

  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    CONTAINER_RUNTIME="podman"
    return 0
  fi

  return 1
}

CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-}"

container_runtime_available() {
  detect_container_runtime
}

# ---------------------------------------------------------------------------
# nft (nftables) availability
# ---------------------------------------------------------------------------

nft_available() {
  command -v nft >/dev/null 2>&1 && nft list ruleset >/dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Shared config writer
# ---------------------------------------------------------------------------

# Minimal config for lifecycle tests.
# lan-access = ["*"] skips firewall/nftables — essential for CI.
write_lifecycle_config() {
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = """
true
"""
TOML
}
