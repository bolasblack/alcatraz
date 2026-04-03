.PHONY: clean

LINT_DIR := out_lint
OUT_DIR := out
BIN_DIR := $(OUT_DIR)/bin

GO_SRC := $(shell find . -name '*.go' -type f -not -path './.alca.cache/*' -not -path './vendor/*' -not -path './.git/*')
EMBED_SRC := $(shell find . -name '*.sh' -type f -not -path './.alca.cache/*' -not -path './vendor/*' -not -path './.git/*')
BUILD_SRC := $(GO_SRC) $(EMBED_SRC)

clean:
	rm -rf $(OUT_DIR) $(LINT_DIR)

# ========= Builds =========
.PHONY: build build\:all
.PHONY: build\:linux\:amd64 build\:linux\:arm64
.PHONY: build\:linux\:amd64-static build\:linux\:arm64-static
.PHONY: build\:darwin\:amd64 build\:darwin\:arm64

build: build\:all schema docs-man docs-completions

# Build all targets
build\:all: build\:linux\:amd64 build\:linux\:arm64 build\:linux\:amd64-static build\:linux\:arm64-static build\:darwin\:amd64 build\:darwin\:arm64

# Linux glibc builds
build\:linux\:amd64: $(BIN_DIR)/alca-linux-amd64
$(BIN_DIR)/alca-linux-amd64: $(BUILD_SRC)
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/alca

build\:linux\:arm64: $(BIN_DIR)/alca-linux-arm64
$(BIN_DIR)/alca-linux-arm64: $(BUILD_SRC)
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 go build -o $@ ./cmd/alca

# Linux static builds (CGO_ENABLED=0 for pure Go static binary)
build\:linux\:amd64-static: $(BIN_DIR)/alca-linux-amd64-static
$(BIN_DIR)/alca-linux-amd64-static: $(BUILD_SRC)
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/alca

build\:linux\:arm64-static: $(BIN_DIR)/alca-linux-arm64-static
$(BIN_DIR)/alca-linux-arm64-static: $(BUILD_SRC)
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $@ ./cmd/alca

# Darwin builds
build\:darwin\:amd64: $(BIN_DIR)/alca-darwin-amd64
$(BIN_DIR)/alca-darwin-amd64: $(BUILD_SRC)
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=amd64 go build -o $@ ./cmd/alca

build\:darwin\:arm64: $(BIN_DIR)/alca-darwin-arm64
$(BIN_DIR)/alca-darwin-arm64: $(BUILD_SRC)
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 go build -o $@ ./cmd/alca

# ========= Testing =========
.PHONY: test test-integration

test:
	go test -coverprofile=out_coverage ./...
	go tool cover -html=out_coverage -o out_coverage.html

test-integration: build\:$(shell go env GOOS)\:$(shell go env GOARCH)
	ALCA_BIN=$(BIN_DIR)/alca-$(shell go env GOOS)-$(shell go env GOARCH) bash test_integration/run.sh

# ========= Linting =========
.PHONY: lint

CUSTOM_GCL := $(LINT_DIR)/custom-gcl

$(CUSTOM_GCL): $(shell find tools/fslint -name '*.go' -type f) config/golangci-lint-custom.yml
	@mkdir -p $(LINT_DIR)
	@ln -sf config/golangci-lint-custom.yml .custom-gcl.yml
	golangci-lint custom --destination=$(LINT_DIR) --name=custom-gcl
	@rm -f .custom-gcl.yml

lint: $(CUSTOM_GCL)
	GOOS=linux $(CUSTOM_GCL) run ./...
	GOOS=darwin $(CUSTOM_GCL) run ./...

# ========= Vendor management =========
.PHONY: vendor vendor-clean vendor-hash-update

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
.PHONY: docs docs-markdown docs-man docs-completions docs-html docs-serve

docs: docs-markdown docs-man docs-completions

docs-markdown:
	go run ./cmd/gendocs markdown

docs-man:
	go run ./cmd/gendocs man

docs-completions:
	go run ./cmd/gendocs completions

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
.PHONY: schema

schema:
	go run ./cmd/genschema alca-config.schema.json

# ========= Release =========
.PHONY: release-notes release-patch release-minor release-major

DOCS_BASE_URL := https://bolasblack.github.io/alcatraz

# Generate release notes from docs/changelogs/<VERSION>.md
# Strips YAML frontmatter and rewrites relative .md links to absolute docs URLs.
# Usage: make release-notes VERSION=v0.2.0
release-notes:
	@if [ -z "$(VERSION)" ]; then echo "Usage: make release-notes VERSION=v0.2.0"; exit 1; fi
	@mkdir -p $(OUT_DIR)
	@CHANGELOG="docs/changelogs/$(VERSION).md"; \
	if [ ! -f "$$CHANGELOG" ]; then echo "Changelog not found: $$CHANGELOG"; exit 1; fi; \
	sed '/^---$$/,/^---$$/d' "$$CHANGELOG" \
		| sed 's|\.\./\([^)]*\)\.md|$(DOCS_BASE_URL)/\1/|g' \
		| sed 's|/_index/|/|g' \
		> $(OUT_DIR)/release-notes.md
	@echo "Generated $(OUT_DIR)/release-notes.md"

release-patch:
	@if ! command -v svu >/dev/null 2>&1; then echo "svu not found. Install: mise install"; exit 1; fi
	@VERSION=$$(svu patch) && \
		sed -i "s/alcaVersion = \".*\"/alcaVersion = \"$${VERSION#v}\"/" flake.nix && \
		git add flake.nix && \
		git commit -m "Release $$VERSION" && \
		git tag -a $$VERSION -m "Release $$VERSION" && \
		git push origin HEAD $$VERSION

release-minor:
	@if ! command -v svu >/dev/null 2>&1; then echo "svu not found. Install: mise install"; exit 1; fi
	@VERSION=$$(svu minor) && \
		sed -i "s/alcaVersion = \".*\"/alcaVersion = \"$${VERSION#v}\"/" flake.nix && \
		git add flake.nix && \
		git commit -m "Release $$VERSION" && \
		git tag -a $$VERSION -m "Release $$VERSION" && \
		git push origin HEAD $$VERSION

release-major:
	@if ! command -v svu >/dev/null 2>&1; then echo "svu not found. Install: mise install"; exit 1; fi
	@VERSION=$$(svu major) && \
		sed -i "s/alcaVersion = \".*\"/alcaVersion = \"$${VERSION#v}\"/" flake.nix && \
		git add flake.nix && \
		git commit -m "Release $$VERSION" && \
		git tag -a $$VERSION -m "Release $$VERSION" && \
		git push origin HEAD $$VERSION
