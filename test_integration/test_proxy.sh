# Group: Transparent proxy DNAT (requires Docker + network helper)
# Sourced by run.sh — no shebang needed.
#
# Tests that network.proxy redirects container traffic via nftables DNAT.
#
# IMPORTANT: br_netfilter must be loaded for DNAT between containers on the
# same bridge. Without it, response packets (SYN-ACK) go through bridge L2
# forwarding which bypasses netfilter — conntrack can't do reverse NAT and
# the container sees the wrong source IP.

PROXY__LISTENER_NAME="alca-test-proxy-listener"

proxy__cleanup_listener() {
  $CONTAINER_RUNTIME rm -f "$PROXY__LISTENER_NAME" 2>/dev/null || true
}

# test_proxy_redirect: container TCP traffic is DNATed to proxy address
test_proxy_redirect() {
  if ! network__helper_installed; then
    skip "test_proxy_redirect — network helper not installed"
    return
  fi

  # Ensure br_netfilter is loaded so bridge packets go through netfilter
  # (needed for conntrack reverse NAT on DNAT'd same-bridge traffic).
  if [[ "$(uname -s)" == "Linux" ]]; then
    # br_netfilter is required so bridge packets go through netfilter
    # (conntrack reverse NAT for DNAT between same-bridge containers).
    # CI loads it via workflow step; NixOS via boot.kernelModules.
    if [[ ! -f /proc/sys/net/bridge/bridge-nf-call-iptables ]]; then
      fail "proxy_redirect: br_netfilter" "kernal module \`br_netfilter\` module not loaded."
      teardown_test_dir
      return
    fi
  fi

  setup_test_dir
  proxy__cleanup_listener

  local proxy_port=19876

  # Start a listener in a separate container on the Docker bridge network.
  $CONTAINER_RUNTIME run -d --name "$PROXY__LISTENER_NAME" alpine \
    sh -c "while true; do nc -l -p $proxy_port >> /tmp/received 2>/dev/null; done" >/dev/null 2>&1

  sleep 1

  # Get the listener container's IP on the bridge network
  local listener_ip
  listener_ip=$($CONTAINER_RUNTIME inspect \
    --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' \
    "$PROXY__LISTENER_NAME" 2>/dev/null)
  if [[ -z "$listener_ip" ]]; then
    fail "proxy_redirect: get listener IP" "could not get listener container IP"
    proxy__cleanup_listener
    teardown_test_dir
    return
  fi

  # Verify listener is ready
  local ready=0
  for i in $(seq 1 5); do
    if $CONTAINER_RUNTIME exec "$PROXY__LISTENER_NAME" sh -c \
      "netstat -tln 2>/dev/null | grep -q $proxy_port || ss -tln 2>/dev/null | grep -q $proxy_port" 2>/dev/null; then
      ready=1
      break
    fi
    sleep 1
  done
  if [[ $ready -eq 0 ]]; then
    fail "proxy_redirect: listener ready" "listener not listening on port $proxy_port after 5s"
    proxy__cleanup_listener
    teardown_test_dir
    return
  fi

  # Config with transparent proxy pointing to the listener container.
  # The ruleset template auto-injects an accept rule for the proxy
  # destination, so no need to add it to lan-access (AGD-037).
  cat > .alca.toml <<TOML
image = "alpine:3.21"
runtime = "docker"

[network]
proxy = "${listener_ip}:${proxy_port}"

[commands]
up = "true"
enter = """
true
"""
TOML

  local up_exit=0
  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || up_exit=$?
  if [[ $up_exit -ne 0 ]]; then
    fail "proxy_redirect: alca up" "alca up failed"
    proxy__cleanup_listener
    teardown_test_dir
    return
  fi

  # Send a marker string to an arbitrary address — DNAT should redirect.
  local marker="PROXY_TEST_$$"
  run_with_timeout 10 "$ALCA_BIN" run sh -c "echo $marker | nc -w 3 8.8.8.8 80" < /dev/null 2>&1 || true
  sleep 2

  # Check if the listener captured the marker.
  local received
  received=$($CONTAINER_RUNTIME exec "$PROXY__LISTENER_NAME" cat /tmp/received 2>/dev/null || true)

  if echo "$received" | grep -qF "$marker"; then
    pass "proxy_redirect: traffic redirected to proxy"
  else
    fail "proxy_redirect: traffic redirected to proxy" "listener did not receive marker '$marker'"
  fi

  proxy__cleanup_listener
  teardown_test_dir
}
