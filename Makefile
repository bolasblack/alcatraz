.PHONY: build test clean docs docs-markdown docs-man docs-html docs-serve vendor vendor-clean vendor-hash-update schema lint
.PHONY: build\:linux\:amd64 build\:linux\:arm64
.PHONY: build\:linux\:amd64-static build\:linux\:arm64-static
.PHONY: build\:darwin\:amd64 build\:darwin\:arm64

LINT_DIR := out_lint
OUT_DIR := out
BIN_DIR := $(OUT_DIR)/bin

GO_SRC := $(shell find . -name '*.go' -type f -not -path './.alca.cache/*' -not -path './vendor/*' -not -path './.git/*')
EMBED_SRC := $(shell find . -name '*.sh' -type f -not -path './.alca.cache/*' -not -path './vendor/*' -not -path './.git/*')
BUILD_SRC := $(GO_SRC) $(EMBED_SRC)

# ========= Cross-compilation builds =========
# Build all targets
build: schema build\:linux\:amd64 build\:linux\:arm64 build\:linux\:amd64-static build\:linux\:arm64-static build\:darwin\:amd64 build\:darwin\:arm64

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
	GOOS=linux $(CUSTOM_GCL) run ./...
	GOOS=darwin $(CUSTOM_GCL) run ./...

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
