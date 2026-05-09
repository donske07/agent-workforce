# Build and install Agent Workforce command binaries.

BINDIR ?= $(HOME)/.local/bin
GO ?= go
COMMANDS := agent-workforce agent-workforce-mcp
BINARIES := $(addprefix $(BINDIR)/,$(COMMANDS))

.PHONY: all build install test clean print-path

all: build

build: $(BINARIES)

$(BINDIR):
	mkdir -p "$(BINDIR)"

$(BINDIR)/agent-workforce: cmd/agent-workforce/main.go $(shell find internal -type f -name '*.go') go.mod go.sum | $(BINDIR)
	$(GO) build -o "$@" ./cmd/agent-workforce

$(BINDIR)/agent-workforce-mcp: cmd/agent-workforce-mcp/main.go $(shell find internal -type f -name '*.go') go.mod go.sum | $(BINDIR)
	$(GO) build -o "$@" ./cmd/agent-workforce-mcp

install: test build
	@echo "Installed binaries to $(BINDIR): $(COMMANDS)"

test:
	$(GO) test ./...

clean:
	rm -f $(BINARIES)

print-path:
	@echo 'Add this to your shell startup file if agent-workforce is not found globally:'
	@echo 'export PATH="$(BINDIR):$$PATH"'
