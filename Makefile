IMAGE_SOURCE ?= https://github.com/isometry/gh-promotion-app

VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
IMAGE_TAG_BASE ?= ghcr.io/isometry/gh-promotion-app

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: build
build: fmt vet
	go build -o gh-promotion-app .

.PHONY: test
test: fmt vet
	go test -v ./...

.PHONY: run
run: fmt vet
	go run .

.PHONY: ko-build
ko-build: ## Build the manager image using ko.
	KO_DOCKER_REPO=$(IMAGE_TAG_BASE) \
	ko build --bare --platform="$(PLATFORMS)" --image-label org.opencontainers.image.source="$(IMAGE_SOURCE)" --tags "latest,$(VERSION)" --push .
