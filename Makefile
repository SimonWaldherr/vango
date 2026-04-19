GOROOT    := $(shell go env GOROOT)
WASM_EXEC := $(GOROOT)/lib/wasm/wasm_exec.js
OUT_DIR   := dist
VERSION   ?= dev

.PHONY: all build build-wasm test dist clean

## all: build the CLI (default target)
all: build

## build: compile the vango-cli binary
build:
	go build -o vango-cli ./cmd

## build-wasm: compile the WASM module into dist/
build-wasm: _distdir
	GOOS=js GOARCH=wasm go build \
		-ldflags "-X main.wasmVersion=$(VERSION)" \
		-o $(OUT_DIR)/vango.wasm ./cmd/wasm

## test: run all unit and example tests
test:
	go test ./...

## dist: build everything needed for the browser demo into dist/
dist: build-wasm
	cp $(WASM_EXEC) $(OUT_DIR)/wasm_exec.js
	cp demo/index.html $(OUT_DIR)/index.html
	@echo ""
	@echo "✅  dist/ is ready.  Serve it with:"
	@echo "     cd dist && python3 -m http.server 8080"
	@echo "   then open http://localhost:8080"

## clean: remove build artefacts
clean:
	rm -rf $(OUT_DIR) vango-cli

_distdir:
	mkdir -p $(OUT_DIR)
