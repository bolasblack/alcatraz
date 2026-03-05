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
  net_output=$(run_with_timeout 30 "$ALCA_BIN" run wget -q --spider https://dl-cdn.alpinelinux.org/alpine/MIRRORS.txt < /dev/null 2>&1) || exit_code=$?
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
    skip "test_network_isolation — network helper not installed"
    return
  fi

  setup_test_dir

  # Config with specific lan-access rules (only allow one address)
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"
runtime = "docker"

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
  pass "network_isolation: alca up with isolation config"

  # Verify firewall rules are actually applied (real system state check)
  local helper_output
  helper_output=$("$ALCA_BIN" network-helper status 2>&1 || true)
  if echo "$helper_output" | grep -qF "Rules applied: Yes"; then
    pass "network_isolation: firewall rules applied"
  else
    fail "network_isolation: firewall rules applied" "network-helper status: $helper_output"
  fi

  # lan-access blocks RFC1918 (LAN) traffic except allowlisted addresses.
  # Internet (WAN) traffic is NOT affected — still allowed.

  # Verify internet still works (WAN not blocked by lan-access rules)
  local wan_output wan_exit=0
  wan_output=$(run_with_timeout 30 "$ALCA_BIN" run wget -q --spider https://dl-cdn.alpinelinux.org/alpine/MIRRORS.txt < /dev/null 2>&1) || wan_exit=$?
  if [[ $wan_exit -eq 0 ]]; then
    pass "network_isolation: internet (WAN) still works"
  else
    fail "network_isolation: internet (WAN) still works" "wget failed: $wan_output"
  fi

  # Verify LAN gateway is blocked (real behavioral test).
  local gw_ip
  gw_ip=$(ip route | awk '/default/ {print $3}')
  if [[ -n "$gw_ip" ]]; then
    local lan_output lan_exit=0
    lan_output=$(run_with_timeout 10 "$ALCA_BIN" run ping -c 1 -W 3 "$gw_ip" < /dev/null 2>&1) || lan_exit=$?
    if [[ $lan_exit -ne 0 ]]; then
      pass "network_isolation: LAN gateway blocked"
    else
      fail "network_isolation: LAN gateway blocked" "ping to gateway $gw_ip succeeded"
    fi
  else
    skip "network_isolation: LAN gateway blocked — could not determine gateway IP"
  fi

  teardown_test_dir
}
