BINARY_NAME ?= $(shell grep 'binary_name:' config.yaml | head -1 | awk '{print $$2}')
VERSION     ?= $(shell grep 'version:' config.yaml | head -1 | awk '{print $$2}')
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS      = -X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT_HASH)

.PHONY: generate build build-garble build-all build-all-garble checksums clean

## generate: Run code generation from OpenAPI specs
generate:
	go run ./tools/generate

## build: Generate code and compile the binary
build: generate
	go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o $(BINARY_NAME) .

## build-garble: Build with garble obfuscation
build-garble: generate
	garble -literals -tiny -seed=random build -ldflags="$(LDFLAGS) -s -w" -trimpath -o $(BINARY_NAME) .

## build-all: Cross-compile for all target platforms
build-all: generate
	@mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-windows-amd64.exe .

## build-all-garble: Cross-compile with garble for all target platforms
build-all-garble: generate
	@mkdir -p dist
	GOOS=linux   GOARCH=amd64 garble -literals -tiny -seed=random build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 garble -literals -tiny -seed=random build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 garble -literals -tiny -seed=random build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 garble -literals -tiny -seed=random build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 garble -literals -tiny -seed=random build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(BINARY_NAME)-windows-amd64.exe .

## checksums: Generate SHA256 checksums for all binaries in dist/
checksums:
	cd dist && shasum -a 256 $(BINARY_NAME)-* > checksums.txt

## clean: Remove build artifacts
clean:
	rm -rf dist/ $(BINARY_NAME) .cache/
