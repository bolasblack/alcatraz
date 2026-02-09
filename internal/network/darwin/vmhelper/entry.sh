#!/bin/sh
set -eu

NFT_DIR="/files/alcatraz_nft"
PLATFORM="${ALCA_PLATFORM:-}"

log() { echo "[alcatraz-network-helper] $*"; }

# --- Readiness check ---
readiness_check() {
  case "$PLATFORM" in
    orbstack)
      log "Waiting for OrbStack network init..."
      for i in $(seq 1 60); do
        if nsenter -t 1 -m -u -n -i nft list table inet orbstack >/dev/null 2>&1; then
          log "OrbStack network ready"
          return 0
        fi
        sleep 0.5
      done
      log "WARNING: OrbStack network init timeout, proceeding anyway"
      ;;
    docker-desktop)
      log "Checking nftables availability..."
      nsenter -t 1 -m -u -n -i modprobe nf_tables 2>/dev/null || true
      for i in $(seq 1 30); do
        if nsenter -t 1 -m -u -n -i nft list tables >/dev/null 2>&1; then
          log "nftables ready"
          return 0
        fi
        sleep 1
      done
      log "WARNING: nftables not available, proceeding anyway"
      ;;
    *)
      log "Unknown platform '$PLATFORM', skipping readiness check"
      ;;
  esac
}

# --- Load all rule files ---
# Rule files live on a volume mount visible inside this container (/files/...),
# but nsenter -m switches to the host mount namespace where that path doesn't exist.
# We use shell redirection (< file) to open the file from container FS before nsenter,
# then nft reads from /dev/stdin inside the host namespace via inherited fd.
load_rules() {
  # Delete all alca-* tables for clean slate
  nsenter -t 1 -m -u -n -i nft list tables | grep "inet alca-" | while read _ _ table; do
    nsenter -t 1 -m -u -n -i nft delete table inet "$table" 2>/dev/null || true
  done

  # Load all .nft files
  local loaded=0
  if [ -d "$NFT_DIR" ]; then
    for f in "$NFT_DIR"/*.nft; do
      [ -f "$f" ] || continue
      log "Loading $f"
      if nsenter -t 1 -m -u -n -i sh -c 'nft -f /dev/stdin' < "$f"; then
        loaded=$((loaded + 1))
      else
        log "ERROR: Failed to load $f"
      fi
    done
  fi
  log "Loaded $loaded rule file(s)"
}

# --- Reload handler ---
reload() {
  log "Reload triggered"

  # Reload-then-recheck loop: keep reloading until state is stable
  while true; do
    # Record current state (mtime of all .nft files)
    state_before=$(find "$NFT_DIR" -name "*.nft" -type f -exec stat -c '%Y %n' {} \; 2>/dev/null | sort)

    # Apply all rules
    load_rules

    # Check if state changed during reload
    state_after=$(find "$NFT_DIR" -name "*.nft" -type f -exec stat -c '%Y %n' {} \; 2>/dev/null | sort)

    if [ "$state_before" = "$state_after" ]; then
      log "Reload complete (state stable)"
      break
    else
      log "State changed during reload, reloading again"
    fi
  done
}

# --- Signal handler ---
trap 'reload' HUP
trap 'log "Shutting down"; exit 0' TERM INT

# --- Main ---
readiness_check
load_rules

# Watch for changes + periodic fallback
log "Watching $NFT_DIR for changes..."
while true; do
  # Try inotifywait if available, else fall back to sleep
  if command -v inotifywait >/dev/null 2>&1; then
    inotifywait -q -t 30 -e create,modify,delete "$NFT_DIR" 2>/dev/null || true
  else
    sleep 30
  fi
  load_rules
done
