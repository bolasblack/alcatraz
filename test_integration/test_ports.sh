# Group 8: Port mapping tests (requires container runtime)
# Sourced by run.sh — no shebang needed.

# ---------------------------------------------------------------------------
# Helper: get container name from state.json
# ---------------------------------------------------------------------------

get_container_name() {
  local state_file=".alca/state.json"
  if [[ ! -f "$state_file" ]]; then
    echo ""
    return
  fi
  # Try python3, fall back to grep+cut
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "import sys,json; print(json.load(sys.stdin)['container_name'])" < "$state_file" 2>/dev/null || true
  else
    grep -o '"container_name":"[^"]*"' "$state_file" | cut -d'"' -f4
  fi
}

# ---------------------------------------------------------------------------
# Helper: get port mappings from container runtime
# ---------------------------------------------------------------------------

get_port_mappings() {
  local container_name="$1"
  $CONTAINER_RUNTIME port "$container_name" 2>&1 || true
}

# ---------------------------------------------------------------------------
# Test: Port mapping
# ---------------------------------------------------------------------------

test_ports_mapping() {
  setup_test_dir
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"
runtime = "docker"

[network]
lan-access = ["*"]
ports = ["8080"]

[commands]
up = "true"
enter = """
true
"""
TOML

  local up_output exit_code=0
  up_output=$(run_with_timeout 120 "$ALCA_BIN" up -q 2>&1) || exit_code=$?
  if [[ $exit_code -ne 0 ]]; then
    fail "ports_mapping: alca up" "failed: $up_output"
    teardown_test_dir
    return
  fi

  # Verify port 8080 is mapped
  local container_name port_output
  container_name=$(get_container_name)
  port_output=$(get_port_mappings "$container_name")

  if echo "$port_output" | grep -q "8080"; then
    pass "ports_mapping: port 8080 mapped"
  else
    fail "ports_mapping: port 8080 mapped" "docker port output: $port_output"
  fi

  teardown_test_dir
}
