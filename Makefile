.PHONY: build test clean

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
