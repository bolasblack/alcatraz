# Group 14: sing-box TUN sidecar cookbook smoke test (default-on, disable with ALCA_TEST_SINGBOX_TUN=0)
# Sourced by run.sh — no shebang needed.

SINGBOX__DEFAULT_IMAGE="ghcr.io/sagernet/sing-box:v1.11.0"
SINGBOX__SIDECAR_NAME=""

singbox__cleanup_sidecar() {
  if [[ -n "$SINGBOX__SIDECAR_NAME" ]]; then
    docker rm -f "$SINGBOX__SIDECAR_NAME" 2>/dev/null || true
  fi
}

singbox__docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

singbox__image_available() {
  docker image inspect "$1" >/dev/null 2>&1
}

singbox__ensure_image_available() {
  local image=$1
  if singbox__image_available "$image"; then
    return 0
  fi
  run_with_timeout 300 docker pull "$image" >/dev/null 2>&1
}

singbox__tun_available() {
  docker run --rm --cap-add NET_ADMIN --device /dev/net/tun alpine:3.21 \
    sh -c 'test -c /dev/net/tun' >/dev/null 2>&1
}

singbox__network_available() {
  docker run --rm alpine:3.21 sh -c '
    nslookup example.com 1.1.1.1 >/dev/null &&
    printf "GET / HTTP/1.0\r\nHost: 1.1.1.1\r\n\r\n" | nc -w 5 1.1.1.1 80 | grep -q "HTTP/"
  ' >/dev/null 2>&1
}

test_singbox_tun_sidecar() {
  case "${ALCA_TEST_SINGBOX_TUN:-}" in
    0|false|False|FALSE)
      skip "singbox_tun_sidecar — disabled by ALCA_TEST_SINGBOX_TUN=${ALCA_TEST_SINGBOX_TUN}"
      return
      ;;
  esac

  if ! singbox__docker_available; then
    skip "singbox_tun_sidecar — Docker unavailable"
    return
  fi

  local singbox_image
  singbox_image="${ALCA_TEST_SINGBOX_IMAGE:-$SINGBOX__DEFAULT_IMAGE}"
  if [[ ! "$singbox_image" =~ ^[A-Za-z0-9._/:@-]+$ ]]; then
    skip "singbox_tun_sidecar — ALCA_TEST_SINGBOX_IMAGE is not a plain Docker image reference"
    return
  fi
  if ! singbox__ensure_image_available "$singbox_image"; then
    fail "singbox_tun_sidecar: sing-box image available" "could not pull or inspect $singbox_image"
    return
  fi

  if ! singbox__tun_available; then
    skip "singbox_tun_sidecar — /dev/net/tun unavailable to Docker"
    return
  fi

  if ! singbox__network_available; then
    skip "singbox_tun_sidecar — external TCP/UDP network unavailable"
    return
  fi

  setup_test_dir
  SINGBOX__SIDECAR_NAME="alca-test-singbox-tun-$$"
  singbox__cleanup_sidecar

  cat > sing-box.json <<'JSON'
{
  "log": { "level": "info" },
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "utun0",
      "address": ["172.20.0.1/30"],
      "auto_route": true,
      "strict_route": false,
      "stack": "system"
    }
  ],
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct",
      "bind_interface": "eth0"
    }
  ],
  "route": {
    "final": "direct"
  }
}
JSON

  cat > .alca.toml <<TOML
image = "alpine:3.21"
runtime = "docker"

[network]
lan-access = ["*"]

[commands]
up = "true"
enter = "true"

[hooks]
post_up = """
docker rm -f $SINGBOX__SIDECAR_NAME >/dev/null 2>&1 || true
docker run -d --name $SINGBOX__SIDECAR_NAME \\
  --network container:\"\$ALCA_CONTAINER_NAME\" \\
  --cap-add NET_ADMIN \\
  --device /dev/net/tun \\
  -v \"\$PWD/sing-box.json:/etc/sing-box/config.json:ro\" \\
  $singbox_image \\
  run -c /etc/sing-box/config.json
"""
pre_down = "docker rm -f $SINGBOX__SIDECAR_NAME >/dev/null 2>&1 || true"
TOML

  local up_output up_exit=0
  up_output=$(run_with_timeout 120 "$ALCA_BIN" up -q 2>&1) || up_exit=$?
  if [[ $up_exit -ne 0 ]]; then
    fail "singbox_tun_sidecar: alca up" "failed: $up_output"
    singbox__cleanup_sidecar
    teardown_test_dir
    return
  fi

  local ready=0
  for _ in $(seq 1 10); do
    if docker inspect "$SINGBOX__SIDECAR_NAME" >/dev/null 2>&1 && \
       docker logs "$SINGBOX__SIDECAR_NAME" 2>&1 | grep -qF "sing-box started"; then
      ready=1
      break
    fi
    sleep 1
  done
  if [[ $ready -ne 1 ]]; then
    fail "singbox_tun_sidecar: sidecar started" "logs: $(docker logs "$SINGBOX__SIDECAR_NAME" 2>&1 || true)"
    singbox__cleanup_sidecar
    teardown_test_dir
    return
  fi
  pass "singbox_tun_sidecar: sidecar started from post_up hook"

  local tun_exit=0
  run_with_timeout 10 "$ALCA_BIN" run test -e /sys/class/net/utun0 < /dev/null 2>&1 || tun_exit=$?
  if [[ $tun_exit -eq 0 ]]; then
    pass "singbox_tun_sidecar: TUN visible in sandbox netns"
  else
    fail "singbox_tun_sidecar: TUN visible in sandbox netns" "utun0 not found"
  fi

  local tcp_output tcp_exit=0
  tcp_output=$(run_with_timeout 20 "$ALCA_BIN" run sh -c 'printf "GET / HTTP/1.0\r\nHost: 1.1.1.1\r\n\r\n" | nc -w 5 1.1.1.1 80 | grep -q "HTTP/"' < /dev/null 2>&1) || tcp_exit=$?
  if [[ $tcp_exit -eq 0 ]]; then
    pass "singbox_tun_sidecar: TCP works through sidecar"
  else
    fail "singbox_tun_sidecar: TCP works through sidecar" "$tcp_output"
  fi

  local udp_output udp_exit=0
  udp_output=$(run_with_timeout 20 "$ALCA_BIN" run nslookup example.com 1.1.1.1 < /dev/null 2>&1) || udp_exit=$?
  if [[ $udp_exit -eq 0 ]]; then
    pass "singbox_tun_sidecar: UDP DNS works through sidecar"
  else
    fail "singbox_tun_sidecar: UDP DNS works through sidecar" "$udp_output"
  fi

  local down_output down_exit=0
  down_output=$(run_with_timeout 30 "$ALCA_BIN" down 2>&1) || down_exit=$?
  if [[ $down_exit -ne 0 ]]; then
    fail "singbox_tun_sidecar: alca down" "failed: $down_output"
    singbox__cleanup_sidecar
    teardown_test_dir
    return
  fi

  if docker inspect "$SINGBOX__SIDECAR_NAME" >/dev/null 2>&1; then
    fail "singbox_tun_sidecar: sidecar removed by pre_down hook" "container still exists"
    singbox__cleanup_sidecar
  else
    pass "singbox_tun_sidecar: sidecar removed by pre_down hook"
  fi

  teardown_test_dir
}
