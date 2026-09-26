BINDIR ?= $(HOME)/.local/bin
BUILDDIR ?= bin
GO ?= go
COMMANDS := agent-workforce agent-workforce-mcp
BINARIES := $(addprefix $(BUILDDIR)/,$(COMMANDS))

.PHONY: all build install test verify clean print-path

all: build

build: $(BINARIES)

$(BUILDDIR):
	mkdir -p "$(BUILDDIR)"

$(BUILDDIR)/agent-workforce: cmd/agent-workforce/main.go $(shell find internal -type f -name '*.go') go.mod go.sum | $(BUILDDIR)
	$(GO) build -trimpath -o "$@" ./cmd/agent-workforce

$(BUILDDIR)/agent-workforce-mcp: cmd/agent-workforce-mcp/main.go $(shell find internal -type f -name '*.go') go.mod go.sum | $(BUILDDIR)
	$(GO) build -trimpath -o "$@" ./cmd/agent-workforce-mcp

install: test build
	mkdir -p "$(BINDIR)"
	install -m 0755 $(BINARIES) "$(BINDIR)"
	@echo "Installed binaries to $(BINDIR): $(COMMANDS)"

test:
	$(GO) test -race -shuffle=on -count=1 ./...

verify: test
	$(GO) vet ./...
	$(GO) build -trimpath ./cmd/agent-workforce ./cmd/agent-workforce-mcp

clean:
	rm -rf "$(BUILDDIR)"

print-path:
	@echo 'Add this to your shell startup file if agent-workforce is not found globally:'
	@echo 'export PATH="$(BINDIR):$$PATH"'
