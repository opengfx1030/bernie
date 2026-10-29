BIN      := bin
DIST     := dist
PKG      := $(shell go list -m)
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X $(PKG)/internal/cli.Version=$(VERSION)
GOFLAGS  := -trimpath
ARCHES   := amd64 arm64

# Release helpers. Archive and package versions have no leading "v"; a build
# that is not on a version tag gets 0.0.0-<describe> so nfpm accepts it.
RELVER   := $(shell echo '$(VERSION)' | sed -E 's/^v//; /^[0-9]+\.[0-9]+\.[0-9]+/!s/^/0.0.0-/')
NFPM     := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.48.0
SHA256   := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)
ENGINE   ?= $(shell command -v docker >/dev/null 2>&1 && echo docker || { command -v podman >/dev/null 2>&1 && echo podman; })
IMAGE    ?= ghcr.io/opengfx1030/bernie

.PHONY: build linux test lint check fmt clean dist packages image sums layers webcheck installtest dashtest

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN)/bernie ./cmd/bernie

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN)/bernie-linux-amd64 ./cmd/bernie

test:
	go test ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

# layers: no package imports a package of a higher layer (docs/architecture.md §2).
layers:
	@scripts/check-layers.sh

# webcheck: the front-end rules of dashboard/AGENTS.md.
webcheck:
	@scripts/check-web.sh

# installtest: the installer against a fake release server (Linux only).
installtest:
	@scripts/test-install.sh

# dashtest: the dashboard service end to end (start, status, stop).
dashtest:
	@scripts/test-dash.sh

check: lint test layers webcheck installtest dashtest

fmt:
	gofmt -w .

# dist: static linux binaries (bernie-linux-<arch>), tar.gz archives
# (bernie_<version>_linux_<arch>.tar.gz) and SHA256SUMS in dist/.
dist:
	rm -rf $(DIST)
	mkdir -p $(DIST)
	@set -e; for arch in $(ARCHES); do \
	  echo "build linux/$$arch"; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(DIST)/bernie-linux-$$arch ./cmd/bernie; \
	  stage=$$(mktemp -d); \
	  cp $(DIST)/bernie-linux-$$arch $$stage/bernie; \
	  cp LICENSE README.md $$stage/; \
	  tar -C $$stage -czf $(DIST)/bernie_$(RELVER)_linux_$$arch.tar.gz bernie LICENSE README.md; \
	  rm -rf $$stage; \
	done
	@$(MAKE) --no-print-directory sums

# packages: .rpm and .deb for each arch via nfpm (pinned). Needs dist/ binaries.
packages:
	@test -f $(DIST)/bernie-linux-amd64 || $(MAKE) --no-print-directory dist
	@set -e; for arch in $(ARCHES); do for fmt in rpm deb; do \
	  NFPM_ARCH=$$arch NFPM_VERSION=$(RELVER) $(NFPM) package -f deploy/packaging/nfpm.yaml -p $$fmt -t $(DIST)/; \
	done; done
	@$(MAKE) --no-print-directory sums

# sums: SHA256SUMS over every file in dist/ (except SHA256SUMS and signatures).
sums:
	cd $(DIST) && find . -maxdepth 1 -type f ! -name 'SHA256SUMS*' ! -name '*.bundle' | sed 's|^\./||' | LC_ALL=C sort | xargs $(SHA256) > SHA256SUMS

# image: build the container image with docker or podman (ENGINE=... to force).
image:
	@test -n "$(ENGINE)" || { echo "image: neither docker nor podman found" >&2; exit 1; }
	$(ENGINE) build -f deploy/docker/Dockerfile --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

clean:
	rm -rf $(BIN) $(DIST)
