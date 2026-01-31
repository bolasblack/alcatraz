.PHONY: build test clean docs docs-markdown docs-man docs-html docs-serve vendor vendor-clean vendor-hash-update schema lint

LINT_DIR := out_lint
OUT_DIR := out
BIN_DIR := $(OUT_DIR)/bin

build: schema $(BIN_DIR)/alca

$(BIN_DIR)/alca: $(shell find . -name '*.go' -type f)
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/alca ./cmd/alca

test:
	go test -coverprofile=out_coverage ./...
	go tool cover -html=out_coverage -o out_coverage.html

clean:
	rm -rf $(OUT_DIR) $(LINT_DIR)

# ========= Linting =========
CUSTOM_GCL := $(LINT_DIR)/custom-gcl

$(CUSTOM_GCL): $(shell find tools/fslint -name '*.go' -type f) config/golangci-lint-custom.yml
	@mkdir -p $(LINT_DIR)
	@ln -sf config/golangci-lint-custom.yml .custom-gcl.yml
	golangci-lint custom --destination=$(LINT_DIR) --name=custom-gcl
	@rm -f .custom-gcl.yml

lint: $(CUSTOM_GCL)
	$(CUSTOM_GCL) run ./...

# ========= Vendor management =========
vendor:
	go mod tidy
	go mod vendor
	go mod verify
	go mod download

vendor-clean:
	rm -rf vendor
	go clean -modcache

# Update vendorHash in flake.nix after go.mod changes
vendor-hash-update: vendor
	@echo "Calculating new vendorHash..."
	@OLD_HASH=$$(grep 'vendorHash' flake.nix | sed 's/.*"\(.*\)".*/\1/'); \
	sed -i.bak 's|vendorHash = ".*"|vendorHash = ""|' flake.nix; \
	NEW_HASH=$$(nix build 2>&1 | grep "got:" | awk '{print $$2}'); \
	if [ -n "$$NEW_HASH" ]; then \
		sed -i.bak "s|vendorHash = \"\"|vendorHash = \"$$NEW_HASH\"|" flake.nix; \
		rm -f flake.nix.bak; \
		echo "Updated vendorHash: $$NEW_HASH"; \
	else \
		mv flake.nix.bak flake.nix; \
		echo "Failed to get new hash, restored original"; \
		exit 1; \
	fi

# ========= Documentation generation =========
docs: docs-markdown docs-man

docs-markdown:
	go run ./cmd/gendocs markdown

docs-man:
	go run ./cmd/gendocs man

# ========= Hugo HTML documentation =========
HUGO_BOOK_VERSION ?= v13
HUGO_THEME_DIR := .hugo/themes/hugo-book

$(HUGO_THEME_DIR):
	@mkdir -p "$(HUGO_THEME_DIR)"
	git clone --depth 1 --branch $(HUGO_BOOK_VERSION) https://github.com/alex-shpak/hugo-book.git $(HUGO_THEME_DIR)
	@rm -rf $(HUGO_THEME_DIR)/.git

docs-html: $(HUGO_THEME_DIR)
	hugo --minify

docs-serve: $(HUGO_THEME_DIR)
	hugo server --buildDrafts

# ========= JSON Schema generation =========
schema:
	go run ./cmd/genschema alca-config.schema.json
