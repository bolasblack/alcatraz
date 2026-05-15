# Group 8: Cleanup Command (requires container runtime)
# Sourced by run.sh — no shebang needed.

test_cleanup_no_orphans() {
  setup_test_dir
  write_lifecycle_config

  run_with_timeout 120 "$ALCA_BIN" up -q 2>&1 || { fail "cleanup_no_orphans: alca up" "setup failed"; teardown_test_dir; return; }

  # Cleanup should find no orphans — project dir exists and state is valid
  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" cleanup 2>&1 || true)
  if echo "$output" | grep -qi "orphan"; then
    fail "cleanup_no_orphans: no orphans reported" "output: $output"
  else
    pass "cleanup_no_orphans: no orphans reported"
  fi

  teardown_test_dir
}

# cleanup__nft_dir: echoes the per-platform nft rule directory.
# Empty if not resolvable.
cleanup__nft_dir() {
  case "$(uname -s)" in
    Darwin) printf '%s' "$HOME/.alcatraz/files/alcatraz_nft" ;;
    Linux)  printf '%s' "/etc/nftables.d/alcatraz" ;;
  esac
}

# cleanup__list_nft_files: lists .nft files for a given project dir.
# Uses sudo on Linux since the directory is under /etc.
cleanup__list_nft_files() {
  local dir=$1
  [[ -z "$dir" ]] && return 0
  if [[ "$(uname -s)" == "Linux" ]]; then
    sudo ls "$dir" 2>/dev/null | grep '\.nft$' || true
  else
    ls "$dir" 2>/dev/null | grep '\.nft$' || true
  fi
}

# test_cleanup_nft_file_removed_when_container_gone: exercises the fix for
# docs_internal/udp-proxy-sidecar-tun-plan.md Phase 2 — if the container was
# removed out-of-band (manual `docker rm`, crash, etc.), `alca down` still
# unwinds the per-project .nft file so its DNAT/TPROXY rules don't hijack
# whoever inherits the container's IP next.
test_cleanup_nft_file_removed_when_container_gone() {
  setup_test_dir

  local nft_dir
  nft_dir=$(cleanup__nft_dir)
  if [[ -z "$nft_dir" ]]; then
    skip "cleanup_nft_file: unsupported platform"
    teardown_test_dir
    return
  fi

  # Use a specific lan-access destination (not "*") so the firewall actually
  # writes a .nft file — wildcard LAN access is a no-op on the rule side.
  cat > .alca.toml <<'TOML'
image = "alpine:3.21"
runtime = "docker"

[network]
lan-access = ["1.2.3.4/32"]

[commands]
up = "true"
enter = "true"
TOML

  if ! run_with_timeout 120 "$ALCA_BIN" up -q 2>&1; then
    fail "cleanup_nft_file: alca up" "alca up failed"
    teardown_test_dir
    return
  fi

  # Snapshot the project's nft file name, derived from the project dir path.
  local expected_file
  expected_file=$(printf '%s' "$CURRENT_TEST_DIR" | tr '/' '-').nft

  local before_files
  before_files=$(cleanup__list_nft_files "$nft_dir")
  if ! printf '%s\n' "$before_files" | grep -Fq -- "$expected_file"; then
    fail "cleanup_nft_file: rule file created on up" "missing $expected_file under $nft_dir; have: $before_files"
    teardown_test_dir
    return
  fi
  pass "cleanup_nft_file: rule file created on up"

  # Remove the container out-of-band so `alca down` has no container to inspect.
  local container_name
  container_name=$(docker ps --filter "label=alca.project.path=$CURRENT_TEST_DIR" --format '{{.Names}}' | head -n1)
  if [[ -z "$container_name" ]]; then
    fail "cleanup_nft_file: locate container" "no container for $CURRENT_TEST_DIR"
    teardown_test_dir
    return
  fi
  docker rm -f "$container_name" >/dev/null 2>&1 || true

  if ! run_with_timeout 30 "$ALCA_BIN" down 2>&1; then
    fail "cleanup_nft_file: alca down" "alca down failed"
    teardown_test_dir
    return
  fi

  local after_files
  after_files=$(cleanup__list_nft_files "$nft_dir")
  if printf '%s\n' "$after_files" | grep -Fq -- "$expected_file"; then
    fail "cleanup_nft_file: rule file removed on down" "$expected_file still under $nft_dir after down; have: $after_files"
  else
    pass "cleanup_nft_file: rule file removed on down even with container gone"
  fi

  teardown_test_dir
}
