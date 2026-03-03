# Group 7: Network tests (requires container runtime)
# Sourced by run.sh — no shebang needed.
#
# test_network_allow_all: works with any runtime, no special deps
# test_network_isolation: requires network-helper installed (platform-agnostic check)

test_network_allow_all() {
  setup_test_dir
  write_lifecycle_config  # uses lan-access = ["*"]

  local up_output
  up_output=$(run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || true)

  # Verify actual network connectivity — wget makes a real HTTP request.
  # alpine:3.21 ships with wget (BusyBox). --spider avoids downloading.
  local net_output exit_code=0
  net_output=$(run_with_timeout 30 "$ALCA_BIN" run wget -q --spider https://dl-cdn.alpinelinux.org/alpine/MIRRORS.txt 2>&1) || exit_code=$?
  if [[ $exit_code -eq 0 ]]; then
    pass "network_allow_all: real network connectivity"
  else
    fail "network_allow_all: real network connectivity" "wget failed: $net_output"
  fi

  # Up output should NOT mention firewall/isolation setup
  if echo "$up_output" | grep -qi "Network isolation\|firewall"; then
    fail "network_allow_all: no firewall with lan-access=*" "output: $up_output"
  else
    pass "network_allow_all: no firewall with lan-access=*"
  fi

  teardown_test_dir
}

# Platform-agnostic check: uses 'alca network-helper status' which works on both
# Linux (nftables) and macOS (helper container). Exits 0 with "Installed: Yes/No".
network_helper_installed() {
  local output
  output=$("$ALCA_BIN" network-helper status 2>&1 || true)
  echo "$output" | grep -qF "Installed: Yes"
}

test_network_isolation() {
  if ! network_helper_installed; then
    echo "  SKIP: test_network_isolation — network helper not installed"
    return
  fi

  setup_test_dir

  # Config with specific lan-access rules (only allow one address)
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"

[network]
lan-access = ["192.168.1.100:8080"]

[commands]
up = "true"
enter = """
true
"""
TOML

  local up_output exit_code=0
  up_output=$(run_with_timeout 120 "$ALCA_BIN" up -q 2>&1) || exit_code=$?

  if [[ $exit_code -ne 0 ]]; then
    fail "network_isolation: alca up" "failed: $up_output"
    teardown_test_dir
    return
  fi

  # Verify isolation was applied (check up output for confirmation)
  if echo "$up_output" | grep -qi "isolation\|firewall\|rules"; then
    pass "network_isolation: firewall rules applied"
  else
    # Also check via network-helper status for active rules
    local helper_output
    helper_output=$("$ALCA_BIN" network-helper status 2>&1 || true)
    if echo "$helper_output" | grep -qF "Rules applied: Yes"; then
      pass "network_isolation: firewall rules applied"
    else
      fail "network_isolation: firewall rules applied" "no evidence of rules in up output or helper status"
    fi
  fi

  # lan-access blocks RFC1918 (LAN) traffic except allowlisted addresses.
  # Internet (WAN) traffic is NOT affected — still allowed.

  # Verify internet still works (WAN not blocked by lan-access rules)
  local wan_output wan_exit=0
  wan_output=$(run_with_timeout 30 "$ALCA_BIN" run wget -q --spider https://dl-cdn.alpinelinux.org/alpine/MIRRORS.txt 2>&1) || wan_exit=$?
  if [[ $wan_exit -eq 0 ]]; then
    pass "network_isolation: internet (WAN) still works"
  else
    fail "network_isolation: internet (WAN) still works" "wget failed: $wan_output"
  fi

  # Verify LAN traffic to non-allowlisted address is blocked.
  # Try reaching 10.255.255.1 (RFC1918, not in our lan-access list).
  # wget is available in alpine:3.21 (BusyBox). --spider avoids downloading.
  local lan_output lan_exit=0
  lan_output=$(run_with_timeout 10 "$ALCA_BIN" run wget -q --timeout=3 --spider http://10.255.255.1:80 2>&1) || lan_exit=$?
  if [[ $lan_exit -ne 0 ]]; then
    pass "network_isolation: non-allowlisted LAN blocked"
  else
    fail "network_isolation: non-allowlisted LAN blocked" "wget to 10.255.255.1 succeeded"
  fi

  # Down — verify cleanup
  run_with_timeout 30 "$ALCA_BIN" down 2>&1 || true

  # Verify firewall rules are cleaned up via helper status
  local post_down_helper
  post_down_helper=$("$ALCA_BIN" network-helper status 2>&1 || true)
  if echo "$post_down_helper" | grep -qF "Rules applied: Yes"; then
    fail "network_isolation: firewall rules cleaned up" "rules still active after down"
  else
    pass "network_isolation: firewall rules cleaned up"
  fi

  teardown_test_dir
}
