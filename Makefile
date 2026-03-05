CONFIG_DIR   ?= configs
CONFIG_FILE  ?=
COMMIT_HASH  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
REPO         ?= un-seen/cli-app

LDFLAGS       = -X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT_HASH)

.PHONY: generate build build-one build-all checksums-one install-script-one release-one release-all clean

## generate: Run code generation for a given config
generate:
ifndef CONFIG_FILE
	$(error CONFIG_FILE is required, e.g. make generate CONFIG_FILE=configs/hai.yaml)
endif
	go run ./tools/generate -config=$(CONFIG_FILE) -output-dir=generated

## build: Single-platform dev build for a given config
build:
ifndef CONFIG_FILE
	$(error CONFIG_FILE is required, e.g. make build CONFIG_FILE=configs/hai.yaml)
endif
	$(eval SLUG := $(basename $(notdir $(CONFIG_FILE))))
	$(eval BINARY_NAME := $(shell grep 'binary_name:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	$(eval VERSION := $(shell grep 'version:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	go run ./tools/generate -config=$(CONFIG_FILE) -output-dir=generated
	go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o $(BINARY_NAME) .

## build-one: Generate + cross-compile one config into dist/<slug>/
build-one:
ifndef CONFIG_FILE
	$(error CONFIG_FILE is required, e.g. make build-one CONFIG_FILE=configs/hai.yaml)
endif
	$(eval SLUG := $(basename $(notdir $(CONFIG_FILE))))
	$(eval BINARY_NAME := $(shell grep 'binary_name:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	$(eval VERSION := $(shell grep 'version:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	@echo "==> Building $(SLUG) (binary=$(BINARY_NAME), version=$(VERSION))"
	go run ./tools/generate -config=$(CONFIG_FILE) -output-dir=generated
	@mkdir -p dist/$(SLUG)
	GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(SLUG)/$(SLUG)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(SLUG)/$(SLUG)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(SLUG)/$(SLUG)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(SLUG)/$(SLUG)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS) -s -w" -trimpath -o dist/$(SLUG)/$(SLUG)-windows-amd64.exe .
	@$(MAKE) checksums-one CONFIG_FILE=$(CONFIG_FILE)
	@$(MAKE) install-script-one CONFIG_FILE=$(CONFIG_FILE)
	@echo "==> Done: dist/$(SLUG)/"

## build-all: Build all configs in configs/
build-all:
	@for f in $(CONFIG_DIR)/*.yaml; do \
		echo ""; \
		$(MAKE) build-one CONFIG_FILE=$$f; \
	done

## checksums-one: SHA256 checksums for one config's binaries
checksums-one:
ifndef CONFIG_FILE
	$(error CONFIG_FILE is required)
endif
	$(eval SLUG := $(basename $(notdir $(CONFIG_FILE))))
	cd dist/$(SLUG) && shasum -a 256 $(SLUG)-* > checksums.txt

## install-script-one: Generate baked install.sh from template
install-script-one:
ifndef CONFIG_FILE
	$(error CONFIG_FILE is required)
endif
	$(eval SLUG := $(basename $(notdir $(CONFIG_FILE))))
	$(eval BINARY_NAME := $(shell grep 'binary_name:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	$(eval TOKEN_ENV_VAR := $(shell grep 'env_var:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	$(eval DOWNLOAD_BASE := https://github.com/$(REPO)/releases/download/$(SLUG)-latest)
	sed \
		-e 's|__CONFIG_ID__|$(SLUG)|g' \
		-e 's|__BINARY_NAME__|$(BINARY_NAME)|g' \
		-e 's|__TOKEN_ENV_VAR__|$(TOKEN_ENV_VAR)|g' \
		-e 's|__DOWNLOAD_BASE__|$(DOWNLOAD_BASE)|g' \
		scripts/install.sh.template > dist/$(SLUG)/install.sh
	chmod +x dist/$(SLUG)/install.sh

## release-one: Create/update GitHub Release with tag <slug>-latest
release-one:
ifndef CONFIG_FILE
	$(error CONFIG_FILE is required, e.g. make release-one CONFIG_FILE=configs/hai.yaml)
endif
	$(eval SLUG := $(basename $(notdir $(CONFIG_FILE))))
	$(eval BINARY_NAME := $(shell grep 'binary_name:' $(CONFIG_FILE) | head -1 | awk '{print $$2}'))
	$(eval TAG := $(SLUG)-latest)
	@echo "==> Releasing $(SLUG) as $(TAG)"
	-gh release delete $(TAG) --yes 2>/dev/null
	-git tag -d $(TAG) 2>/dev/null
	-git push origin :refs/tags/$(TAG) 2>/dev/null
	gh release create $(TAG) dist/$(SLUG)/* --title "$(BINARY_NAME) latest" --notes "Latest build of $(BINARY_NAME)"
	@echo "==> Released: https://github.com/$(REPO)/releases/tag/$(TAG)"

## release-all: Release all configs
release-all:
	@for f in $(CONFIG_DIR)/*.yaml; do \
		$(MAKE) release-one CONFIG_FILE=$$f; \
	done

## clean: Remove build artifacts
clean:
	rm -rf dist/ generated/ .cache/
