BINARY=clab-tui
GOBUILD=go build
GOTEST=go test
GOMOD=$(GOBUILD) ./...
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
VERSION_LDFLAGS=-X main.version=$(VERSION)
STATIC_CGO_LDFLAGS=$(shell pkg-config --static --libs libpcap libcap 2>/dev/null)
STATIC_LDFLAGS=$(VERSION_LDFLAGS) -linkmode external -extldflags "-static"

.PHONY: all build build-static build-dynamic test test-unit test-ui test-tmux integration-base-prereqs integration-prereqs integration-scale-prereqs integration-kind-prereqs test-integration test-integration-full test-integration-smoke test-integration-scale test-integration-kinds test-integration-all test-ebpf test-ebpf-focused test-all update-golden clean lint fmt run vet run-testdata run-demo

all: fmt lint test build

build:
	$(MAKE) build-static

# Release build: statically links Go, libc, libpcap, and its static system
# dependencies. Requires libpcap-dev, libcap-dev, libsystemd-dev, libdbus-1-dev,
# libibverbs-dev, and libnl-3-dev on Debian/Ubuntu systems.
build-static:
	@test -n "$(STATIC_CGO_LDFLAGS)" || (echo "error: pkg-config static libpcap/libcap dependencies are missing" >&2; exit 1)
	CGO_ENABLED=1 CGO_LDFLAGS="$(STATIC_CGO_LDFLAGS)" $(GOBUILD) -trimpath -tags netgo,osusergo -ldflags '$(STATIC_LDFLAGS)' -o $(BINARY) ./cmd/$(BINARY)/

# Development fallback when static system libraries are unavailable.
build-dynamic:
	$(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o $(BINARY) ./cmd/$(BINARY)/

test:
	$(GOTEST) ./...

test-unit:
	$(GOTEST) ./...

test-ui:
	$(GOTEST) ./internal/tui/... -count=1

test-tmux:
	$(GOTEST) ./internal/tui -run 'TestTmux' -count=1 -timeout 120s

integration-base-prereqs:
	@set -eu; \
		test "$$(id -u)" = 0 || { echo "error: integration tests require root; use sudo or a privileged runner" >&2; exit 1; }; \
		command -v docker >/dev/null || { echo "error: integration tests require docker in PATH" >&2; exit 1; }; \
		docker info >/dev/null 2>&1 || { echo "error: integration tests require a running Docker daemon" >&2; exit 1; }; \
		clab_bin="$${CLAB_BIN:-containerlab}"; command -v "$$clab_bin" >/dev/null || { echo "error: integration tests require containerlab binary '$$clab_bin' in PATH" >&2; exit 1; }

integration-prereqs: integration-base-prereqs
	@set -eu; \
		docker image inspect publicmirror.azurecr.io/debian:bookworm >/dev/null 2>&1 || { echo "error: integration tests require image publicmirror.azurecr.io/debian:bookworm; pull it before running this target" >&2; exit 1; }

integration-trace-prereqs: integration-prereqs
	@set -eu; \
		docker image inspect nicolaka/netshoot:v0.13 >/dev/null 2>&1 || { echo "error: trace integration requires image nicolaka/netshoot:v0.13; pull it before running this target" >&2; exit 1; }

test-integration: integration-trace-prereqs
	$(GOTEST) -tags integration ./internal/engine/containerlab -run '^TestIntegration(DeploySmoke|SessionOpens|SessionRawBytesStreams|PacketTraceCapable|TracePath|TracePathBusinessBidirectional)$$' -count=1 -timeout 900s

integration-scale-prereqs: integration-base-prereqs
	@set -eu; \
		host_image="$${CLAB_TUI_SCALE_HOST_IMAGE:-nicolaka/netshoot:v0.13}"; \
		docker image inspect ghcr.io/nokia/srlinux:24.7.1 >/dev/null 2>&1 || { echo "error: scale integration requires image ghcr.io/nokia/srlinux:24.7.1; pull it before running this target" >&2; exit 1; }; \
		docker image inspect "$$host_image" >/dev/null 2>&1 || { echo "error: scale integration requires resolved host image '$$host_image'; pull it before running this target" >&2; exit 1; }

integration-kind-prereqs: integration-base-prereqs
	@set -eu; \
		docker image inspect alpine:3.20 >/dev/null 2>&1 || { echo "error: kind integration requires image alpine:3.20; pull it before running this target" >&2; exit 1; }; \
		docker image inspect ghcr.io/nokia/srlinux:24.7.1 >/dev/null 2>&1 || { echo "error: kind integration requires image ghcr.io/nokia/srlinux:24.7.1; pull it before running this target" >&2; exit 1; }

test-integration-full: integration-trace-prereqs integration-scale-prereqs integration-kind-prereqs
	$(GOTEST) -tags integration ./... -count=1 -timeout 2400s

test-integration-smoke: integration-prereqs
	$(GOTEST) -tags integration ./internal/engine/containerlab -run '^TestIntegration(DeploySmoke|SessionOpens|SessionRawBytesStreams)$$' -count=1 -timeout 600s

test-integration-scale: integration-scale-prereqs
	$(GOTEST) -tags integration ./internal/engine/containerlab -run '^TestIntegrationClos10(Deploy|Lifecycle|StatsAndNetem|Trace)$$' -count=1 -timeout 1800s

test-integration-kinds: integration-kind-prereqs
	# Go still owns manifest validation and optional-kind skip semantics.
	$(GOTEST) -tags integration ./internal/engine/containerlab -run TestIntegrationKindMatrix -count=1 -timeout 600s

test-ebpf: integration-trace-prereqs integration-scale-prereqs integration-kind-prereqs
	$(GOTEST) -tags integration ./internal/engine/containerlab/... -count=1 -timeout 2400s

test-ebpf-focused: integration-prereqs
	$(GOTEST) -tags integration ./internal/engine/containerlab -run '^TestIntegration(PacketTraceCapable|TracePath|TracePathBusinessBidirectional)$$' -count=1 -timeout 600s

# Default aggregate: unit/UI/tmux/vet only. Integration targets are explicit
# because they require root, Docker, containerlab, and external images.
test-all: test-unit test-ui test-tmux vet

# Explicit privileged integration aggregate; use this instead of test-all when
# Docker/containerlab, root, and the required kernel/images are available.
test-integration-all: test-integration test-integration-scale test-integration-kinds

update-golden:
	UPDATE_GOLDEN=1 $(GOTEST) ./internal/tui -run TestTopologyGolden -count=1

test-v:
	$(GOTEST) -v ./...

clean:
	rm -f $(BINARY)
	rm -rf dist/

lint: vet
	staticcheck ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

# Run against testdata labs so there is always something to see.
run: build
	sudo ./$(BINARY) --dir testdata/simple

# Run against the 20-node demo lab.
run-big: build
	./$(BINARY) --dir testdata/big

# Run in the current directory.
run-cwd: build
	./$(BINARY)
