package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donske07/agent-workforce/internal/product"
)

func TestWriteAndRemoveMCPConfig(t *testing.T) {
	home := useTempHome(t)
	opts := CommonOptions{OfficeURL: "http://127.0.0.1:9999", ForgeBin: "/custom/forge"}

	path, err := writeMCPConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".agent-workforce", "mcp", product.PackageName+".json"); path != want {
		t.Fatalf("unexpected MCP config path: got %s want %s", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.MCPServers[product.AgentMCPServerName(product.CoordinatorAgentID)]; ok {
		t.Fatalf("coordinator must not be generated as a specialist MCP server")
	}
	if len(payload.MCPServers) != len(product.SpecialistAgents()) {
		t.Fatalf("unexpected MCP server count: got %d want %d", len(payload.MCPServers), len(product.SpecialistAgents()))
	}
	for _, agent := range product.SpecialistAgents() {
		name := product.AgentMCPServerName(agent.ID)
		server, ok := payload.MCPServers[name]
		if !ok {
			t.Fatalf("missing MCP server %s", name)
		}
		if server.Command != product.MCPCommand {
			t.Fatalf("unexpected MCP command for %s: got %s want %s", name, server.Command, product.MCPCommand)
		}
		wantArgs := []string{"--agent", agent.ID}
		if strings.Join(server.Args, " ") != strings.Join(wantArgs, " ") {
			t.Fatalf("unexpected args for %s: got %#v want %#v", name, server.Args, wantArgs)
		}
		if server.Env["AGENT_WORKFORCE_HOME"] != filepath.Join(home, ".agent-workforce") {
			t.Fatalf("unexpected home env for %s: got %s", name, server.Env["AGENT_WORKFORCE_HOME"])
		}
		if server.Env["AGENT_WORKFORCE_OFFICE_URL"] != opts.OfficeURL {
			t.Fatalf("unexpected office URL for %s: got %s want %s", name, server.Env["AGENT_WORKFORCE_OFFICE_URL"], opts.OfficeURL)
		}
		if server.Env["AGENT_WORKFORCE_FORGE_BIN"] != opts.ForgeBin {
			t.Fatalf("unexpected forge binary for %s: got %s want %s", name, server.Env["AGENT_WORKFORCE_FORGE_BIN"], opts.ForgeBin)
		}
	}
	if err := removeMCPConfig(); err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Fatalf("MCP config was not removed: %s", path)
	}
}
