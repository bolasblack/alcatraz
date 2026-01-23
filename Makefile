.PHONY: build test clean docs docs-markdown docs-man vendor update-vendor-hash

OUT_DIR := out
BIN_DIR := $(OUT_DIR)/bin

build: $(BIN_DIR)/alca

$(BIN_DIR)/alca: $(shell find . -name '*.go' -type f)
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/alca ./cmd/alca

test:
	go test ./...

clean:
	rm -rf $(OUT_DIR)

# Vendor management
vendor:
	go mod tidy
	go mod vendor
	go mod verify

# Update vendorHash in flake.nix after go.mod changes
update-vendor-hash: vendor
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

# Documentation generation
docs: docs-markdown docs-man

docs-markdown:
	go run ./cmd/gendocs markdown

docs-man:
	go run ./cmd/gendocs man
