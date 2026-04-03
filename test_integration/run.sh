#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source all components
source "$SCRIPT_DIR/helpers.sh"
source "$SCRIPT_DIR/test_config.sh"
source "$SCRIPT_DIR/test_lifecycle.sh"
source "$SCRIPT_DIR/test_status.sh"
source "$SCRIPT_DIR/test_drift.sh"
source "$SCRIPT_DIR/test_enter.sh"
source "$SCRIPT_DIR/test_mounts.sh"
source "$SCRIPT_DIR/test_network.sh"
source "$SCRIPT_DIR/test_ports.sh"
source "$SCRIPT_DIR/test_cleanup.sh"
source "$SCRIPT_DIR/test_init.sh"
source "$SCRIPT_DIR/test_subdir.sh"
source "$SCRIPT_DIR/test_restart_policy.sh"
source "$SCRIPT_DIR/test_proxy.sh"

# Prerequisites
if [[ ! -x "$ALCA_BIN" ]]; then
  echo "ERROR: alca binary not found at $ALCA_BIN"
  echo "Build first: make build"
  exit 1
fi

ALCA_BIN="$(cd "$(dirname "$ALCA_BIN")" && pwd)/$(basename "$ALCA_BIN")"
echo "Using alca binary: $ALCA_BIN"
echo ""

# ---------------------------------------------------------------------------
# Ensure dockerd is running (CI environments may not have it started)
# ---------------------------------------------------------------------------

DOCKERD_PID=""

ensure_dockerd() {
  if docker info >/dev/null 2>&1; then
    return 0
  fi
  if ! command -v dockerd >/dev/null 2>&1; then
    return 0
  fi
  echo "Starting dockerd..."
  dockerd &>/tmp/dockerd.log &
  DOCKERD_PID=$!
  local i=0
  while [ "$i" -lt 30 ]; do
    if docker info >/dev/null 2>&1; then
      echo "dockerd ready (PID: $DOCKERD_PID)"
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  echo "ERROR: dockerd failed to start within 30s"
  cat /tmp/dockerd.log
  exit 1
}

cleanup_dockerd() {
  if [ -n "$DOCKERD_PID" ]; then
    echo "Stopping dockerd (PID: $DOCKERD_PID)..."
    kill "$DOCKERD_PID" 2>/dev/null || true
    wait "$DOCKERD_PID" 2>/dev/null || true
  fi
}

trap 'cleanup_on_exit; cleanup_network_helper; cleanup_dockerd' EXIT

ensure_dockerd

# ---------------------------------------------------------------------------
# Ensure network helper is installed (required for network isolation tests)
# ---------------------------------------------------------------------------

NETWORK_HELPER_INSTALLED_BY_US=""

ensure_network_helper() {
  if ! container_runtime_available; then
    return 0
  fi
  if "$ALCA_BIN" network-helper status 2>&1 | grep -qF "Installed: Yes"; then
    return 0
  fi
  echo "Installing network helper..."
  sudo "$ALCA_BIN" network-helper install --yes 2>&1 || true
  NETWORK_HELPER_INSTALLED_BY_US=1
}

cleanup_network_helper() {
  if ! container_runtime_available; then
    return 0
  fi
  if [ -n "$NETWORK_HELPER_INSTALLED_BY_US" ]; then
    sudo "$ALCA_BIN" network-helper uninstall --yes 2>/dev/null || true
  fi
}

ensure_network_helper

# ---------------------------------------------------------------------------
# Group filter: TEST_GROUP=12 runs only Group 12, unset runs all.
# ---------------------------------------------------------------------------
should_run_group() {
  [[ -z "${TEST_GROUP:-}" ]] || [[ "${TEST_GROUP}" == "$1" ]]
}

# Group 1: Config (no container runtime needed)
if should_run_group 1; then
  echo "=== Group 1: Config ==="
  test_config_validation
  test_init_template
  test_init_template_unknown
fi

# Groups 2+: require container runtime (Docker or Podman)
if container_runtime_available; then
  if should_run_group 2; then
    echo ""
    echo "=== Group 2: Lifecycle ==="
    test_lifecycle_basic
  fi

  if should_run_group 3; then
    echo ""
    echo "=== Group 3: Status Variations ==="
    test_status_not_running
  fi

  if should_run_group 4; then
    echo ""
    echo "=== Group 4: Config Drift Detection ==="
    test_config_drift
  fi

  if should_run_group 5; then
    echo ""
    echo "=== Group 5: Enter Command ==="
    test_run_enter_command
  fi

  if should_run_group 6; then
    echo ""
    echo "=== Group 6: Mounts ==="
    test_mount_persistence
    test_workdir_exclude
  fi

  if should_run_group 7; then
    echo ""
    echo "=== Group 7: Network ==="
    test_network_allow_all
    test_network_isolation
  fi

  if should_run_group 8; then
    echo ""
    echo "=== Group 8: Port Mapping ==="
    test_ports_mapping
  fi

  if should_run_group 9; then
    echo ""
    echo "=== Group 9: Cleanup ==="
    test_cleanup_no_orphans
  fi

  if should_run_group 10; then
    echo ""
    echo "=== Group 10: Subdirectory Discovery ==="
    test_subdir_status
    test_subdir_run
  fi

  if should_run_group 11; then
    echo ""
    echo "=== Group 11: Restart Policy ==="
    test_restart_policy
  fi

  if should_run_group 12; then
    echo ""
    echo "=== Group 12: Transparent Proxy ==="
    test_proxy_redirect
  fi
elif [[ -z "${TEST_GROUP:-}" ]]; then
  echo ""
  skip "No container runtime (Docker/Podman) available — skipping Groups 2-12"
fi

# Summary
echo ""
echo "=== Summary ==="
echo "Passed: $PASSED  Failed: $FAILED  Skipped: $SKIPPED"
if [[ $FAILED -gt 0 ]]; then
  for name in "${FAIL_NAMES[@]}"; do
    echo "  - $name"
  done
  exit 1
fi
