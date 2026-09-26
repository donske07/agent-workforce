package product

import (
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/donske07/agent-workforce/internal/assets"
)

func TestAgentRegistryHasUniqueIDsAndToolNames(t *testing.T) {
	ids := map[string]bool{}
	tools := map[string]bool{}
	for _, agent := range Agents {
		if agent.ID == "" {
			t.Fatal("agent has empty ID")
		}
		if ids[agent.ID] {
			t.Fatalf("duplicate agent ID %s", agent.ID)
		}
		ids[agent.ID] = true
		if agent.Title == "" || agent.Role == "" || agent.Color == "" || agent.Description == "" {
			t.Fatalf("agent %s has incomplete metadata", agent.ID)
		}
		tool := AgentToolName(agent.ID)
		if tools[tool] {
			t.Fatalf("duplicate tool name %s", tool)
		}
		tools[tool] = true
	}
	if len(AgentIDs()) != len(Agents) {
		t.Fatalf("AgentIDs length mismatch: got %d want %d", len(AgentIDs()), len(Agents))
	}
}

func TestForgeMCPToolName(t *testing.T) {
	server := AgentMCPServerName("frontend-programmer")
	tool := AgentToolName("frontend-programmer")
	got := ForgeMCPToolName(server, tool)
	want := "mcp_agent_workforce_mcp_frontend_programmer_tool_agent_workforce_frontend_programmer"
	if got != want {
		t.Fatalf("unexpected Forge MCP tool name: got %s want %s", got, want)
	}
}

func TestAgentRegistryIsCoordinatorPlusProgrammersOnly(t *testing.T) {
	if len(Agents) != 10 {
		t.Fatalf("unexpected agent count: got %d want 10", len(Agents))
	}
	if Agents[0].ID != CoordinatorAgentID {
		t.Fatalf("coordinator should be first registry entry, got %s", Agents[0].ID)
	}
	for _, agent := range Agents {
		if strings.Contains(agent.ID, "architect") || strings.Contains(strings.ToLower(agent.Title), "architect") {
			t.Fatalf("architect agent remains in registry: %#v", agent)
		}
		if agent.ID != CoordinatorAgentID && !strings.HasSuffix(agent.ID, "-programmer") {
			t.Fatalf("non-coordinator agent must be a programmer: %s", agent.ID)
		}
	}
}

func TestSpecialistAgentsExcludeCoordinator(t *testing.T) {
	specialists := SpecialistAgents()
	if len(specialists) != len(Agents)-1 {
		t.Fatalf("unexpected specialist count: got %d want %d", len(specialists), len(Agents)-1)
	}
	for _, agent := range specialists {
		if agent.ID == CoordinatorAgentID {
			t.Fatal("coordinator must not be exposed as a specialist")
		}
	}
}

func TestAgentByID(t *testing.T) {
	agent, ok := AgentByID("backend-programmer")
	if !ok {
		t.Fatal("backend-programmer not found")
	}
	if agent.Title != "Backend Programmer" {
		t.Fatalf("unexpected title: %s", agent.Title)
	}
	if _, ok := AgentByID("missing-agent"); ok {
		t.Fatal("unexpected match for missing agent")
	}
}

func TestBundledAgentFilesMatchCoordinatorProgrammerArchitecture(t *testing.T) {
	entries, err := fs.ReadDir(assets.Files, "files/forge/agents")
	if err != nil {
		t.Fatal(err)
	}

	registryIDs := map[string]bool{}
	for _, agent := range Agents {
		registryIDs[agent.ID] = true
	}
	fileIDs := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			continue
		}
		data, err := assets.Files.ReadFile(path.Join("files/forge/agents", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		frontmatter := parseTestAgentFrontmatter(t, string(data))
		id := frontmatter["id"]
		if id == "" {
			t.Fatalf("%s is missing id frontmatter", entry.Name())
		}
		if !registryIDs[id] {
			t.Fatalf("bundled agent %s is not in product registry", id)
		}
		if strings.Contains(id, "architect") || strings.Contains(strings.ToLower(frontmatter["title"]), "architect") {
			t.Fatalf("architect bundled agent remains: %s", id)
		}
		assertReasoningConfig(t, id, string(data))
		fileIDs[id] = true

		tools := parseTestAgentTools(t, string(data))
		if id == CoordinatorAgentID {
			assertTools(t, id, tools, coordinatorToolProfile(), []string{"write", "patch", "multi_patch", "remove", "undo", "shell", "task"})
			content := string(data)
			for _, want := range []string{
				"Treat each successful specialist MCP result as the specialist's compact final-answer text",
				"Do not expect `structuredContent`, audit trail markdown, raw MCP JSON, subprocess details, stdout/stderr wrappers",
				"Agent Workforce records specialist completion metadata in Pixel Agent Office logs instead of returning it to the coordinator",
			} {
				if !strings.Contains(content, want) {
					t.Fatalf("coordinator instructions missing minimal MCP response guidance %q", want)
				}
			}
			if strings.Contains(content, "Include the audit trail provided by each successful specialist MCP result") {
				t.Fatal("coordinator instructions must not require specialist audit trail preservation")
			}
			continue
		}
		if !strings.HasSuffix(id, "-programmer") {
			t.Fatalf("non-coordinator bundled agent must be a programmer: %s", id)
		}
		assertTools(t, id, tools, programmerToolProfile(), []string{"followup", "task", "skill", "agent_workforce_*"})
		content := string(data)
		for _, want := range []string{
			"Format the final response as compact final-answer text",
			"Fields that do not apply may be omitted or marked `not applicable`",
			"Do not manually construct MCP JSON",
			"stdout/stderr wrappers",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s instructions missing compact final-answer guidance %q", id, want)
			}
		}
	}

	for id := range registryIDs {
		if !fileIDs[id] {
			t.Fatalf("registry agent %s is missing bundled agent file", id)
		}
	}
}

func TestCoordinatorAllowsForgeVisibleServerQualifiedMCPWorkflow(t *testing.T) {
	data, err := assets.Files.ReadFile("files/forge/agents/coordinator.md")
	if err != nil {
		t.Fatal(err)
	}
	tools := parseTestAgentTools(t, string(data))
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool] = true
	}

	for _, agentID := range coordinatorMCPAgentIDs() {
		tool := ForgeMCPToolName(AgentMCPServerName(agentID), AgentToolName(agentID))
		if !seen[tool] {
			t.Fatalf("coordinator is missing Forge-visible server-qualified MCP tool %s", tool)
		}
	}

	frontendWrapper := ForgeMCPToolName(AgentMCPServerName("frontend-programmer"), AgentToolName("frontend-programmer"))
	if !seen[frontendWrapper] {
		t.Fatalf("coordinator frontend workflow is missing server-qualified MCP wrapper %s", frontendWrapper)
	}

	for _, agent := range SpecialistAgents() {
		for _, tool := range []string{
			ForgeMCPToolName(PackageName, NotifyToolName),
			ForgeMCPToolName(PackageName, AgentToolName(agent.ID)),
			ForgeMCPToolName(AgentMCPServerName(agent.ID), NotifyToolName),
		} {
			if seen[tool] {
				t.Fatalf("coordinator should use explicit server-qualified specialist wrappers only, but found %s", tool)
			}
		}
	}
}

func coordinatorToolProfile() []string {
	tools := []string{"sem_search", "fs_search", "read", "fetch", "followup", "skill", "plan", "todo_write", "todo_read"}
	for _, agentID := range coordinatorMCPAgentIDs() {
		serverName := AgentMCPServerName(agentID)
		tools = append(tools, ForgeMCPToolName(serverName, AgentToolName(agentID)))
	}
	return tools
}

func coordinatorMCPAgentIDs() []string {
	ids := make([]string, 0, len(SpecialistAgents()))
	for _, agent := range SpecialistAgents() {
		ids = append(ids, agent.ID)
	}
	return ids
}

func programmerToolProfile() []string {
	return []string{"read", "write", "fs_search", "sem_search", "remove", "patch", "multi_patch", "undo", "shell", "fetch", "plan", "todo_write", "todo_read"}
}

func assertTools(t *testing.T, id string, got, want, forbidden []string) {
	t.Helper()
	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		t.Fatalf("%s tools mismatch:\ngot  %#v\nwant %#v", id, gotSorted, wantSorted)
	}
	for _, tool := range got {
		for _, bad := range forbidden {
			if tool == bad || (strings.HasSuffix(bad, "*") && strings.HasPrefix(tool, strings.TrimSuffix(bad, "*"))) {
				t.Fatalf("%s has forbidden tool %s", id, tool)
			}
		}
	}
}

func assertReasoningConfig(t *testing.T, id, content string) {
	t.Helper()
	frontmatter := parseTestAgentFrontmatter(t, content)
	if frontmatter["reasoning"] != "" {
		t.Fatalf("%s reasoning frontmatter should be an object, got inline value %q", id, frontmatter["reasoning"])
	}
	want := map[string]string{"enabled": "true", "effort": "high", "exclude": "false", "tool_supported": "false"}
	for key, expected := range want {
		if got := frontmatter[key]; got != expected {
			t.Fatalf("%s reasoning.%s mismatch: got %q want %q", id, key, got, expected)
		}
	}
}

func parseTestAgentFrontmatter(t *testing.T, content string) map[string]string {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatal("agent file missing frontmatter start")
	}
	values := map[string]string{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			return values
		}
		key, value, ok := strings.Cut(line, ":")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	t.Fatal("agent file missing frontmatter end")
	return nil
}

func parseTestAgentTools(t *testing.T, content string) []string {
	t.Helper()
	lines := strings.Split(content, "\n")
	inTools := false
	tools := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "tools:" {
			inTools = true
			continue
		}
		if !inTools {
			continue
		}
		if strings.HasPrefix(line, "  - ") {
			tools = append(tools, strings.TrimSpace(strings.TrimPrefix(line, "  - ")))
			continue
		}
		if trimmed != "" {
			break
		}
	}
	if len(tools) == 0 {
		t.Fatal("agent file missing tools")
	}
	return tools
}
