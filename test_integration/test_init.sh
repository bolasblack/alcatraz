# Group: Init (no container runtime needed)
# Sourced by run.sh — no shebang needed.

# test_init_template: --template flag creates config non-interactively
test_init_template() {
  setup_test_dir

  # alca init --template alpine should create .alca.toml without prompts
  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" init --template alpine 2>&1)
  assert_file_exists ".alca.toml" "init_template: config file created"
  assert_file_contains ".alca.toml" "alpine" "init_template: config contains alpine image"

  teardown_test_dir
}

# test_init_template_unknown: unknown template produces clear error
test_init_template_unknown() {
  setup_test_dir

  local output
  output=$(run_with_timeout 30 "$ALCA_BIN" init --template bogus 2>&1 || true)
  assert_stdout_contains "$output" "unknown template" "init_template_unknown: error for invalid template"

  # Config file should NOT be created
  if [[ ! -f ".alca.toml" ]]; then
    pass "init_template_unknown: no config created"
  else
    fail "init_template_unknown: no config created" ".alca.toml was created"
  fi

  teardown_test_dir
}
