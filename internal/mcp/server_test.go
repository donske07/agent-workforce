package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-workforce/agent-workforce/internal/assets"
	"github.com/agent-workforce/agent-workforce/internal/product"
)

func TestScopedToolDefinitionsExposeOnlySelectedSpecialistAndNotify(t *testing.T) {
	frontend, ok := findSpecialistAgent("frontend-programmer")
	if !ok {
		t.Fatal("missing frontend specialist")
	}
	state := newServerState(&frontend)
	defs := state.toolDefinitions()
	tools, ok := defs["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tools payload type: %T", defs["tools"])
	}

	seen := map[string]bool{}
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		seen[name] = true
	}
	if !seen[product.NotifyToolName] {
		t.Fatalf("missing notify tool %s", product.NotifyToolName)
	}
	if !seen[product.AgentToolName(frontend.ID)] {
		t.Fatalf("missing scoped agent tool %s", product.AgentToolName(frontend.ID))
	}
	for _, agent := range product.SpecialistAgents() {
		name := product.AgentToolName(agent.ID)
		if agent.ID != frontend.ID && seen[name] {
			t.Fatalf("scoped MCP exposed non-scoped agent tool %s", name)
		}
	}
	if seen[product.AgentToolName(product.CoordinatorAgentID)] {
		t.Fatalf("coordinator must not be exposed as an MCP specialist tool")
	}
	if len(tools) != 2 {
		t.Fatalf("unexpected scoped tool count: got %d want 2", len(tools))
	}
}

func TestUnscopedToolDefinitionsRemainAvailableForCompatibility(t *testing.T) {
	defs := newServerState(nil).toolDefinitions()
	tools, ok := defs["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tools payload type: %T", defs["tools"])
	}

	seen := map[string]bool{}
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		seen[name] = true
	}
	for _, agent := range product.SpecialistAgents() {
		name := product.AgentToolName(agent.ID)
		if !seen[name] {
			t.Fatalf("missing agent tool %s", name)
		}
	}
	if seen[product.AgentToolName(product.CoordinatorAgentID)] {
		t.Fatalf("coordinator must not be exposed as an MCP specialist tool")
	}
}

func TestCoordinatorFrontendDelegationWorkflowEndToEnd(t *testing.T) {
	coordinator, err := assets.Files.ReadFile("files/forge/agents/coordinator.md")
	if err != nil {
		t.Fatal(err)
	}
	frontendWrapper := product.ForgeMCPToolName(product.PackageName, product.AgentToolName("frontend-programmer"))
	coordinatorTools := parseTestAgentTools(t, string(coordinator))
	if !hasString(coordinatorTools, frontendWrapper) {
		t.Fatalf("coordinator is missing Forge-visible frontend MCP wrapper %s", frontendWrapper)
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "forge-log.json")
	forgePath := filepath.Join(dir, "forge")
	script := `#!/bin/sh
python3 - "$@" <<'PY'
import json, os, sys
with open(os.environ["FORGE_LOG"], "w") as f:
    json.dump({"args": sys.argv[1:], "child": os.environ.get("AGENT_WORKFORCE_MCP_CHILD")}, f)
print("specialist progress that should be stripped")
print("AGENT_WORKFORCE_FINAL_START")
print("Frontend Programmer response: Vue.js is the frontend framework in question; check the official Vue release channel or npm for the latest exact version before pinning dependencies.")
print("AGENT_WORKFORCE_FINAL_END")
PY
`
	if err := os.WriteFile(forgePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_WORKFORCE_FORGE_BIN", forgePath)
	t.Setenv("FORGE_LOG", logPath)

	state := &serverState{
		idleDelay:    0,
		notifyOffice: func(agentID, state, title, officeURL string) bool { return true },
		runForge:     defaultForgeRunner,
	}

	listResponse, ok := state.handleRequest(request{JSONRPC: "2.0", ID: 1, Method: "tools/list"})
	if !ok {
		t.Fatal("tools/list did not produce a response")
	}
	listResult := listResponse["result"].(map[string]any)
	tools := listResult["tools"].([]map[string]any)
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool["name"].(string))
	}
	rawFrontendTool := product.AgentToolName("frontend-programmer")
	if !hasString(toolNames, rawFrontendTool) {
		t.Fatalf("unscoped MCP server is missing raw frontend tool %s; exposed tools: %#v", rawFrontendTool, toolNames)
	}

	userPrompt := "What is the latest version for Vue.js for frontend development?"
	params, err := json.Marshal(callParams{
		Name: rawFrontendTool,
		Arguments: map[string]any{
			"task":            userPrompt,
			"context":         "Coordinator frontend delegation workflow smoke test. Forge-visible wrapper " + frontendWrapper + " maps to raw MCP tool " + rawFrontendTool + ".",
			"expected_output": "Concise Vue.js answer from the frontend specialist.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	callResponse, ok := state.handleRequest(request{JSONRPC: "2.0", ID: 2, Method: "tools/call", Params: params})
	if !ok {
		t.Fatal("tools/call did not produce a response")
	}
	callResult := callResponse["result"].(map[string]any)
	payload := callResult["structuredContent"].(map[string]any)
	if payload["ok"] != true {
		t.Fatalf("expected successful frontend delegation, got %#v", payload)
	}
	if payload["agent_id"] != "frontend-programmer" {
		t.Fatalf("expected frontend-programmer delegation, got %#v", payload["agent_id"])
	}
	output := payload["output"].(string)
	if !strings.Contains(output, "Frontend Programmer") || !strings.Contains(output, "Vue.js") {
		t.Fatalf("frontend specialist response missing expected content: %q", output)
	}
	visibleContent := callResult["content"].([]map[string]string)[0]["text"]
	if visibleContent != output {
		t.Fatalf("visible MCP response should be the specialist final answer, got %q want %q", visibleContent, output)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var forgeCall struct {
		Args  []string `json:"args"`
		Child string   `json:"child"`
	}
	if err := json.Unmarshal(data, &forgeCall); err != nil {
		t.Fatal(err)
	}
	if len(forgeCall.Args) != 4 || forgeCall.Args[0] != "--agent" || forgeCall.Args[1] != "frontend-programmer" || forgeCall.Args[2] != "-p" {
		t.Fatalf("expected MCP dispatch to run forge --agent frontend-programmer -p <prompt>, got %#v", forgeCall.Args)
	}
	if forgeCall.Child != "1" {
		t.Fatalf("expected child dispatch environment marker, got %q", forgeCall.Child)
	}
	for _, want := range []string{userPrompt, "Task:\n", "Expected output:\nConcise Vue.js answer", finalAnswerStartMarker, finalAnswerEndMarker} {
		if !strings.Contains(forgeCall.Args[3], want) {
			t.Fatalf("delegated prompt missing %q:\n%s", want, forgeCall.Args[3])
		}
	}
}

func TestLiveCoordinatorFrontendDelegationSmoke(t *testing.T) {
	if os.Getenv("AGENT_WORKFORCE_LIVE_COORDINATOR_E2E") != "1" {
		t.Skip("set AGENT_WORKFORCE_LIVE_COORDINATOR_E2E=1 to run the live Forge coordinator smoke test")
	}
	forgeBin := os.Getenv("FORGE_BIN")
	if forgeBin == "" {
		var err error
		forgeBin, err = exec.LookPath("forge")
		if err != nil {
			t.Skipf("forge binary not found: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	prompt := "What is the latest version for Vue.js for frontend development? Please delegate to the frontend MCP agent and answer concisely."
	cmd := exec.CommandContext(ctx, forgeBin, "--agent", "coordinator", "-p", prompt)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "CLICOLOR=0", "TERM=dumb")
	output, err := cmd.CombinedOutput()
	text := cleanForgeText(string(output))
	if ctx.Err() != nil {
		t.Fatalf("live coordinator smoke test timed out; output:\n%s", text)
	}
	if err != nil {
		t.Fatalf("live coordinator smoke test failed: %v\n%s", err, text)
	}
	for _, forbidden := range []string{"Tool not found", "unknown tool", "unavailable", "not available"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("live coordinator output indicates MCP delegation failure %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(strings.ToLower(text), "vue") {
		t.Fatalf("live coordinator output did not mention Vue:\n%s", text)
	}
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

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSpecialistToolCallRunsForgeAgentAndNotifiesOfficeLifecycle(t *testing.T) {
	var calls []struct {
		agentID string
		state   string
		title   string
	}
	var ranAgent product.Agent
	var ranPrompt string
	state := &serverState{
		idleDelay: 0,
		notifyOffice: func(agentID, state, title, officeURL string) bool {
			calls = append(calls, struct {
				agentID string
				state   string
				title   string
			}{agentID: agentID, state: state, title: title})
			return true
		},
		runForge: func(agent product.Agent, prompt string) forgeResult {
			ranAgent = agent
			ranPrompt = prompt
			if len(calls) != 1 || calls[0].state != product.OfficeStateThinking {
				return forgeResult{ExitCode: 1, Err: errors.New("forge ran before thinking notification")}
			}
			return forgeResult{Output: "\x1b[36m●\x1b[0m [10:29:05] Initialize abc\n" + finalAnswerStartMarker + "\n\x1b[32mHello specialist\x1b[0m\n" + finalAnswerEndMarker + "\n● [10:29:09] Finished abc\n", Stderr: "\r\x1b[2K\x1b[32m⠋\x1b[0m \x1b[1;32mContemplating\x1b[0m \x1b[37m00s\x1b[0m \x1b[2;37m· Ctrl+C to interrupt\x1b[0m", ExitCode: 0}
		},
	}

	result := state.handleToolCall(callParams{
		Name: product.AgentToolName("frontend-programmer"),
		Arguments: map[string]any{
			"task":            "say hi",
			"context":         "test context",
			"expected_output": "brief acknowledgement",
		},
	})

	payload := decodeTextPayload(t, result)
	if payload["ok"] != true {
		t.Fatalf("expected ok response, got %#v", payload)
	}
	if payload["agent_id"] != "frontend-programmer" {
		t.Fatalf("unexpected agent id: %#v", payload["agent_id"])
	}
	if payload["output"] != "Hello specialist" {
		t.Fatalf("unexpected output: %#v", payload["output"])
	}
	if payload["stderr"] != "" {
		t.Fatalf("successful response should omit noisy stderr, got %#v", payload["stderr"])
	}
	if payload["exit_code"] != 0 {
		t.Fatalf("unexpected exit code: %#v", payload["exit_code"])
	}
	if ranAgent.ID != "frontend-programmer" {
		t.Fatalf("unexpected forge agent: %s", ranAgent.ID)
	}
	for _, want := range []string{"Task:\nsay hi", "Context:\ntest context", "Expected output:\nbrief acknowledgement", finalAnswerStartMarker, finalAnswerEndMarker} {
		if !strings.Contains(ranPrompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, ranPrompt)
		}
	}
	wantCalls := []struct {
		agentID string
		state   string
		title   string
	}{
		{agentID: "frontend-programmer", state: product.OfficeStateThinking, title: "Frontend Programmer"},
		{agentID: "frontend-programmer", state: product.OfficeStateIdle, title: "Frontend Programmer"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected office calls: got %#v want %#v", calls, wantCalls)
	}
}

func TestSpecialistToolCallReturnsFinalAnswerOnly(t *testing.T) {
	state := &serverState{
		idleDelay:    0,
		notifyOffice: func(agentID, state, title, officeURL string) bool { return true },
		runForge: func(agent product.Agent, prompt string) forgeResult {
			return forgeResult{Output: strings.Join([]string{
				"󰄗 Inspect the repository structure and existing project files",
				"Considering boilerplate application setup",
				"I'm thinking about what files a boilerplate application might need.",
				"npm install --save-dev vite @vitejs/plugin-vue",
				"> testing-agent-workforce@0.0.0 build",
				"vite v8.0.11 building client environment for production...",
				finalAnswerStartMarker,
				"## Vue Boilerplate Created",
				"",
				"Scaffolded a minimal Vue 3 + Vite frontend app.",
				"",
				"### Verification",
				"- npm run build passed.",
				finalAnswerEndMarker,
				"more trailing transcript noise",
			}, "\n"), ExitCode: 0}
		},
	}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("frontend-programmer"), Arguments: map[string]any{"task": "create vue app"}})
	payload := decodeTextPayload(t, result)
	output, _ := payload["output"].(string)
	if payload["ok"] != true {
		t.Fatalf("expected ok response, got %#v", payload)
	}
	for _, forbidden := range []string{"Considering boilerplate", "I'm thinking", "npm install", "vite v8", "trailing transcript"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output leaked transcript fragment %q:\n%s", forbidden, output)
		}
	}
	if !strings.Contains(output, "## Vue Boilerplate Created") || !strings.Contains(output, "npm run build passed") {
		t.Fatalf("output missing final answer content:\n%s", output)
	}
	content := result["content"].([]map[string]string)
	if content[0]["text"] != output {
		t.Fatalf("visible MCP text should be final answer only, got %q want %q", content[0]["text"], output)
	}
}

func TestExtractFinalAnswerFallsBackToLastMarkdownSection(t *testing.T) {
	raw := strings.Join([]string{
		"󰄗 Inspect repository",
		"I am considering setup details and should not leak this.",
		"command transcript line",
		"## Vue Boilerplate Created",
		"",
		"Created the app and verified the build.",
	}, "\n")
	got := extractFinalAnswer(cleanForgeText(raw))
	if strings.Contains(got, "considering") || strings.Contains(got, "command transcript") {
		t.Fatalf("fallback leaked transcript:\n%s", got)
	}
	if got != "## Vue Boilerplate Created\n\nCreated the app and verified the build." {
		t.Fatalf("unexpected fallback output:\n%s", got)
	}
}

func TestSpecialistToolCallReturnsForgeFailure(t *testing.T) {
	state := &serverState{
		idleDelay:    0,
		notifyOffice: func(agentID, state, title, officeURL string) bool { return true },
		runForge: func(agent product.Agent, prompt string) forgeResult {
			return forgeResult{Output: "", Stderr: "bad things", ExitCode: 7, Err: errors.New("exit status 7")}
		},
	}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("backend-programmer"), Arguments: map[string]any{"task": "do work"}})
	payload := decodeTextPayload(t, result)
	if payload["ok"] != false {
		t.Fatalf("expected failure response, got %#v", payload)
	}
	if payload["agent_id"] != "backend-programmer" {
		t.Fatalf("unexpected agent id: %#v", payload["agent_id"])
	}
	if payload["stderr"] != "bad things" {
		t.Fatalf("unexpected stderr: %#v", payload["stderr"])
	}
	if payload["exit_code"] != float64(7) {
		t.Fatalf("unexpected exit code: %#v", payload["exit_code"])
	}
	if !strings.Contains(payload["error"].(string), "exit status 7") {
		t.Fatalf("unexpected error: %#v", payload["error"])
	}
}

func TestSpecialistToolCallRejectsMissingTask(t *testing.T) {
	state := &serverState{runForge: func(agent product.Agent, prompt string) forgeResult {
		t.Fatal("forge should not run without a task")
		return forgeResult{}
	}}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("frontend-programmer"), Arguments: map[string]any{"task": "  "}})
	payload := decodeTextPayload(t, result)
	if payload["ok"] != false {
		t.Fatalf("expected failure response, got %#v", payload)
	}
	if payload["error"] != "task is required" {
		t.Fatalf("unexpected error: %#v", payload["error"])
	}
}

func TestScopedSpecialistToolCallRejectsOtherAgentTool(t *testing.T) {
	frontend, ok := findSpecialistAgent("frontend-programmer")
	if !ok {
		t.Fatal("missing frontend specialist")
	}
	state := &serverState{
		agent: &frontend,
		runForge: func(agent product.Agent, prompt string) forgeResult {
			t.Fatal("forge should not run for an out-of-scope tool")
			return forgeResult{}
		},
	}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("backend-programmer"), Arguments: map[string]any{"task": "do backend work"}})
	payload := decodeTextPayload(t, result)
	if payload["ok"] != false {
		t.Fatalf("expected failure response, got %#v", payload)
	}
	if !strings.Contains(payload["error"].(string), "unknown tool") {
		t.Fatalf("unexpected error: %#v", payload["error"])
	}
}

func TestFindSpecialistAgentRejectsCoordinator(t *testing.T) {
	if _, ok := findSpecialistAgent(product.CoordinatorAgentID); ok {
		t.Fatal("coordinator must not be a scoped specialist agent")
	}
}

func TestSpecialistToolCallRejectsNestedDispatch(t *testing.T) {
	t.Setenv(mcpChildDispatchEnv, "1")
	state := &serverState{runForge: func(agent product.Agent, prompt string) forgeResult {
		t.Fatal("forge should not run during nested dispatch")
		return forgeResult{}
	}}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("frontend-programmer"), Arguments: map[string]any{"task": "say hi"}})
	payload := decodeTextPayload(t, result)
	if payload["ok"] != false {
		t.Fatalf("expected failure response, got %#v", payload)
	}
	if !strings.Contains(payload["error"].(string), "nested") {
		t.Fatalf("unexpected error: %#v", payload["error"])
	}
}

func TestCleanForgeTextRemovesAnsiProgressAndLifecycleLines(t *testing.T) {
	raw := "\x1b[36m●\x1b[0m \x1b[2m[10:29:05] \x1b[0m\x1b[2mInitialize abc\x1b[0m\nHello, I received this delegated prompt.\n\x1b[36m●\x1b[0m \x1b[2m[10:29:09] \x1b[0m\x1b[2mFinished\x1b[0m \x1b[2mabc\x1b[0m\n\r\x1b[2K\x1b[32m⠸\x1b[0m \x1b[1;32mContemplating\x1b[0m \x1b[37m02s\x1b[0m \x1b[2;37m· Ctrl+C to interrupt\x1b[0m\r\x1b[2K"
	got := cleanForgeText(raw)
	if got != "Hello, I received this delegated prompt." {
		t.Fatalf("unexpected cleaned text: %q", got)
	}
}

func TestDefaultForgeRunnerUsesForgePromptAndAgent(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "forge-log.json")
	forgePath := filepath.Join(dir, "forge")
	script := `#!/bin/sh
python3 - "$@" <<'PY'
import json, os, sys
path = os.environ["FORGE_LOG"]
with open(path, "w") as f:
    json.dump({"args": sys.argv[1:], "child": os.environ.get("AGENT_WORKFORCE_MCP_CHILD"), "no_color": os.environ.get("NO_COLOR"), "color": os.environ.get("CLICOLOR"), "term": os.environ.get("TERM")}, f)
print("fake forge output")
print("fake forge stderr", file=sys.stderr)
PY
`
	if err := os.WriteFile(forgePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_WORKFORCE_FORGE_BIN", forgePath)
	t.Setenv("FORGE_LOG", logPath)

	result := defaultForgeRunner(product.Agent{ID: "qa-programmer"}, "Task:\nwrite tests")
	if result.Err != nil {
		t.Fatalf("unexpected forge error: %v stderr=%s", result.Err, result.Stderr)
	}
	if !strings.Contains(result.Output, "fake forge output") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if !strings.Contains(result.Stderr, "fake forge stderr") {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Args    []string `json:"args"`
		Child   string   `json:"child"`
		NoColor string   `json:"no_color"`
		Color   string   `json:"color"`
		Term    string   `json:"term"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"--agent", "qa-programmer", "-p", "Task:\nwrite tests"}
	if !reflect.DeepEqual(payload.Args, wantArgs) {
		t.Fatalf("unexpected args: got %#v want %#v", payload.Args, wantArgs)
	}
	if payload.Child != "1" {
		t.Fatalf("expected child dispatch env marker, got %q", payload.Child)
	}
	if payload.NoColor != "1" || payload.Color != "0" || payload.Term != "dumb" {
		t.Fatalf("expected non-interactive color suppression env, got NO_COLOR=%q CLICOLOR=%q TERM=%q", payload.NoColor, payload.Color, payload.Term)
	}
}

func TestScopedRunStdioListsOnlySelectedAgentTool(t *testing.T) {
	frontend, ok := findSpecialistAgent("frontend-programmer")
	if !ok {
		t.Fatal("missing frontend specialist")
	}
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var output bytes.Buffer

	if err := runStdio(input, &output, &frontend); err != nil {
		t.Fatal(err)
	}

	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("failed to decode stdio response %q: %v", output.String(), err)
	}
	seen := map[string]bool{}
	for _, tool := range response.Result.Tools {
		seen[tool.Name] = true
	}
	if !seen[product.NotifyToolName] || !seen[product.AgentToolName("frontend-programmer")] {
		t.Fatalf("missing scoped tools in response: %#v", seen)
	}
	if seen[product.AgentToolName("backend-programmer")] {
		t.Fatalf("scoped stdio exposed backend tool: %#v", seen)
	}
	if len(response.Result.Tools) != 2 {
		t.Fatalf("unexpected scoped stdio tool count: got %d want 2", len(response.Result.Tools))
	}
}

func TestNotifyToolRemainsNotificationOnly(t *testing.T) {
	var notified bool
	state := &serverState{
		notifyOffice: func(agentID, state, title, officeURL string) bool {
			notified = true
			if agentID != "frontend-programmer" || state != product.OfficeStateThinking || title != "Frontend Programmer" {
				t.Fatalf("unexpected notification: %s %s %s", agentID, state, title)
			}
			return true
		},
		runForge: func(agent product.Agent, prompt string) forgeResult {
			t.Fatal("notify tool should not run forge")
			return forgeResult{}
		},
	}

	result := state.handleToolCall(callParams{Name: product.NotifyToolName, Arguments: map[string]any{"agent_id": "frontend-programmer", "state": product.OfficeStateThinking, "title": "Frontend Programmer"}})
	payload := decodeTextPayload(t, result)
	if payload["ok"] != true || !notified {
		t.Fatalf("unexpected notify result: %#v notified=%v", payload, notified)
	}
}

func TestNotifyIdleAfterDelayWaitsBeforeIdle(t *testing.T) {
	state := &serverState{idleDelay: 25 * time.Millisecond, notifyOffice: func(agentID, state, title, officeURL string) bool { return true }}
	start := time.Now()
	if !state.notifyIdleAfterDelay("frontend-programmer", "Frontend Programmer", "") {
		t.Fatal("expected idle notification to succeed")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("idle notification did not wait for delay: %s", elapsed)
	}
}

func decodeTextPayload(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	if structured, ok := result["structuredContent"].(map[string]any); ok {
		return structured
	}
	content := result["content"].([]map[string]string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(content[0]["text"]), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
