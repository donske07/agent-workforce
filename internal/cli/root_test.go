package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-workforce/agent-workforce/internal/product"
)

func useTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewCommandLaunchesCoordinatorForgeAndForwardsArgs(t *testing.T) {
	useTempHome(t)
	var gotName string
	var gotArgs []string
	previous := coordinatorForgeRunner
	coordinatorForgeRunner = func(name string, args []string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() { coordinatorForgeRunner = previous })

	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--forge-bin", "/custom/forge", "-p", "Review this repo"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotName != "/custom/forge" {
		t.Fatalf("unexpected forge binary: got %s", gotName)
	}
	wantArgs := []string{"--agent", product.CoordinatorAgentID, "-p", "Review this repo"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected forge args: got %#v want %#v", gotArgs, wantArgs)
	}
}

func TestNewCommandUsesForgeBinEnvAndStopsParsingAfterDoubleDash(t *testing.T) {
	useTempHome(t)
	t.Setenv("FORGE_BIN", "/env/forge")
	var gotName string
	var gotArgs []string
	previous := coordinatorForgeRunner
	coordinatorForgeRunner = func(name string, args []string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() { coordinatorForgeRunner = previous })

	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--", "--forge-bin", "literal", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotName != "/env/forge" {
		t.Fatalf("unexpected forge binary: got %s", gotName)
	}
	wantArgs := []string{"--agent", product.CoordinatorAgentID, "--forge-bin", "literal", "--help"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected forge args: got %#v want %#v", gotArgs, wantArgs)
	}
}

func TestNewCommandRequiresForgeBinValue(t *testing.T) {
	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--forge-bin"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing forge-bin value to fail")
	}
	if !strings.Contains(err.Error(), "--forge-bin requires a value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewCommandReturnsForgeRunnerError(t *testing.T) {
	useTempHome(t)
	wantErr := errors.New("forge failed")
	previous := coordinatorForgeRunner
	coordinatorForgeRunner = func(name string, args []string) error {
		return wantErr
	}
	t.Cleanup(func() { coordinatorForgeRunner = previous })

	cmd := newCommand(&CommonOptions{ForgeBin: "/custom/forge"})
	cmd.SetArgs(nil)

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected runner error %v, got %v", wantErr, err)
	}
}

func TestInventoryDoesNotCleanupLegacyArtifacts(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	legacyAgent := legacyAgents[0]
	legacyAgentPath := filepath.Join(agentDir(config), legacyAgent+".md")
	legacySkillPath := filepath.Join(skillDir(config), legacySkills[0], "SKILL.md")
	writeTestFile(t, legacyAgentPath, "legacy agent")
	writeTestFile(t, legacySkillPath, "legacy skill")
	if err := saveAgentState(map[string]bool{legacyAgent: false}); err != nil {
		t.Fatal(err)
	}

	agents, err := inventory(config)
	if err != nil {
		t.Fatal(err)
	}
	_ = agents

	if !exists(legacyAgentPath) {
		t.Fatalf("inventory removed legacy agent %s", legacyAgentPath)
	}
	if !exists(legacySkillPath) {
		t.Fatalf("inventory removed legacy skill %s", legacySkillPath)
	}
	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state[legacyAgent]; !ok {
		t.Fatalf("inventory removed legacy state for %s", legacyAgent)
	}
}

func TestSyncAgentsCleansLegacyAndInstallsBundledAgents(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	legacyAgent := legacyAgents[0]
	legacyAgentPath := filepath.Join(agentDir(config), legacyAgent+".md")
	legacySkillPath := filepath.Join(skillDir(config), legacySkills[0], "SKILL.md")
	writeTestFile(t, legacyAgentPath, "legacy agent")
	writeTestFile(t, legacySkillPath, "legacy skill")
	if err := saveAgentState(map[string]bool{legacyAgent: false}); err != nil {
		t.Fatal(err)
	}

	actions, err := syncAgents(config, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) < len(bundledAgents) {
		t.Fatalf("expected at least %d sync actions, got %d", len(bundledAgents), len(actions))
	}
	if exists(legacyAgentPath) {
		t.Fatalf("sync did not remove legacy agent %s", legacyAgentPath)
	}
	if exists(legacySkillPath) {
		t.Fatalf("sync did not remove legacy skill %s", legacySkillPath)
	}
	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state[legacyAgent]; ok {
		t.Fatalf("sync did not remove legacy state for %s", legacyAgent)
	}
	for _, id := range product.AgentIDs() {
		if !exists(filepath.Join(agentDir(config), id+".md")) {
			t.Fatalf("sync did not install bundled agent %s", id)
		}
	}
}

func TestSyncAgentFilesPrefersBundledOverrides(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	id := bundledAgents[0]
	overrideContent := "---\nid: " + id + "\ntitle: Override\ndescription: Override agent.\n---\n\nOVERRIDE_MARKER\n"
	writeTestFile(t, customAgentPath(id), overrideContent)

	if _, err := syncAgentFiles(config, false); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(agentDir(config), id+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), "OVERRIDE_MARKER") {
		t.Fatalf("bundled override was not synced into active agent file")
	}
}

func TestActivateRejectsUnknownAgentID(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	opts := &CommonOptions{ForgeConfig: config}
	cmd := agentsActivateCommand(opts, true)
	cmd.SetArgs([]string{"missing-agent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown agent activation to fail")
	}
	if !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("expected agent not found error, got %v", err)
	}
	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state["missing-agent"]; ok {
		t.Fatal("unknown agent was written to state")
	}
}

func TestLoadAgentStateSupportsLegacySchema(t *testing.T) {
	useTempHome(t)
	writeTestFile(t, agentStatePath(), `{"agents":{"frontend-programmer":{"active":false},"backend-programmer":{"active":true}},"schemaVersion":1}`)

	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if state["frontend-programmer"] {
		t.Fatal("expected frontend-programmer to be inactive")
	}
	if !state["backend-programmer"] {
		t.Fatal("expected backend-programmer to be active")
	}
}

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
