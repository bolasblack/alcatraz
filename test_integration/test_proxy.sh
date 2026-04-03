# Group: Transparent proxy DNAT (requires Docker + network helper)
# Sourced by run.sh — no shebang needed.
#
# Tests that network.proxy redirects container traffic via nftables DNAT.
# Uses a listener INSIDE a Docker container (not on the host) to avoid
# macOS firewall issues. All traffic stays within the Docker network.

PROXY_LISTENER_NAME="alca-test-proxy-listener"

cleanup_proxy_listener() {
  $CONTAINER_RUNTIME rm -f "$PROXY_LISTENER_NAME" 2>/dev/null || true
}

# test_proxy_redirect: container TCP traffic is DNATed to proxy address
test_proxy_redirect() {
  if ! network_helper_installed; then
    skip "test_proxy_redirect — network helper not installed"
    return
  fi

  setup_test_dir
  cleanup_proxy_listener

  local proxy_port=19876

  # Start a listener in a separate container on the Docker bridge network.
  # nc -l -p PORT listens for one connection, saves data, then sleep keeps
  # the container alive so we can read the file.
  $CONTAINER_RUNTIME run -d --name "$PROXY_LISTENER_NAME" alpine \
    sh -c "nc -l -p $proxy_port > /tmp/received 2>/dev/null; sleep 30" >/dev/null 2>&1

  sleep 1

  # Get the listener container's IP on the bridge network
  local listener_ip
  listener_ip=$($CONTAINER_RUNTIME inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$PROXY_LISTENER_NAME" 2>/dev/null)
  if [[ -z "$listener_ip" ]]; then
    fail "proxy_redirect: get listener IP" "could not get listener container IP"
    cleanup_proxy_listener
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
    cleanup_proxy_listener
    teardown_test_dir
    return
  fi

  # Send a marker string from inside the test container to an arbitrary address.
  # DNAT should redirect this connection to the listener container.
  local marker="PROXY_TEST_$$"
  run_with_timeout 10 "$ALCA_BIN" run sh -c "echo $marker | nc -w 3 8.8.8.8 80" < /dev/null 2>&1 || true

  # Give the listener a moment to flush
  sleep 1

  # Read what the listener captured
  local received
  received=$($CONTAINER_RUNTIME exec "$PROXY_LISTENER_NAME" cat /tmp/received 2>/dev/null || true)

  if echo "$received" | grep -qF "$marker"; then
    pass "proxy_redirect: traffic redirected to proxy"
  else
    fail "proxy_redirect: traffic redirected to proxy" "listener did not receive marker '$marker'"
  fi

  cleanup_proxy_listener
  teardown_test_dir
}
