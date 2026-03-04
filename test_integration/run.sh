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

# Prerequisites
if [[ ! -x "$ALCA_BIN" ]]; then
  echo "ERROR: alca binary not found at $ALCA_BIN"
  echo "Build first: make build"
  exit 1
fi

ALCA_BIN="$(cd "$(dirname "$ALCA_BIN")" && pwd)/$(basename "$ALCA_BIN")"
echo "Using alca binary: $ALCA_BIN"
echo ""

# Group 1: Config (no container runtime needed)
echo "=== Group 1: Config ==="
test_config_validation

# Groups 2-9: require container runtime (Docker or Podman)
if container_runtime_available; then
  echo ""
  echo "Container runtime: $CONTAINER_RUNTIME"
  echo ""
  echo "=== Group 2: Lifecycle ==="
  test_lifecycle_basic

  echo ""
  echo "=== Group 3: Status Variations ==="
  test_status_not_running

  echo ""
  echo "=== Group 4: Config Drift Detection ==="
  test_config_drift

  echo ""
  echo "=== Group 5: Enter Command ==="
  test_run_enter_command

  echo ""
  echo "=== Group 6: Mounts ==="
  test_mount_persistence
  test_workdir_exclude

  echo ""
  echo "=== Group 7: Network ==="
  test_network_allow_all
  test_network_isolation

  echo ""
  echo "=== Group 8: Port Mapping ==="
  test_ports_mapping

  echo ""
  echo "=== Group 9: Cleanup ==="
  test_cleanup_no_orphans
else
  echo ""
  echo "  SKIP: No container runtime (Docker/Podman) available — skipping Groups 2-9"
fi

# Summary
echo ""
echo "=== Summary ==="
echo "Passed: $PASSED  Failed: $FAILED"
if [[ $FAILED -gt 0 ]]; then
  for name in "${FAIL_NAMES[@]}"; do
    echo "  - $name"
  done
  exit 1
fi
