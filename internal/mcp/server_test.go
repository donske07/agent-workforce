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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/donske07/agent-workforce/internal/assets"
	"github.com/donske07/agent-workforce/internal/product"
)

func TestScopedToolDefinitionsExposeOnlySelectedSpecialist(t *testing.T) {
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
	if seen[product.NotifyToolName] {
		t.Fatalf("scoped MCP should not expose notify tool %s", product.NotifyToolName)
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
	if len(tools) != 1 {
		t.Fatalf("unexpected scoped tool count: got %d want 1", len(tools))
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
	if seen[product.NotifyToolName] {
		t.Fatalf("unscoped MCP server should not expose notify tool %s", product.NotifyToolName)
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
	frontendWrapper := product.ForgeMCPToolName(product.AgentMCPServerName("frontend-programmer"), product.AgentToolName("frontend-programmer"))
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
	visibleContent := resultText(t, callResult, false)
	if !strings.Contains(visibleContent, "Frontend Programmer") || !strings.Contains(visibleContent, "Vue.js") {
		t.Fatalf("frontend specialist response missing expected content: %q", visibleContent)
	}
	assertNoAuditTrail(t, visibleContent)

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
	var postedEvents []map[string]any
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
		postOfficeEvent: func(event map[string]any, officeURL string) bool {
			postedEvents = append(postedEvents, event)
			return true
		},
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
			ranAgent = agent
			ranPrompt = prompt
			if len(calls) != 1 || calls[0].state != product.OfficeStateThinking {
				return forgeResult{ExitCode: 1, Err: errors.New("forge ran before thinking notification")}
			}
			onOutput("stdout", "\x1b[32mstreamed hello\x1b[0m")
			onOutput("stderr", "\r\x1b[2Kwarning")
			onOutput("stderr", "\r\x1b[2K⠋ Forging 2:01m · Ctrl+C to interrupt")
			onOutput("stdout", finalAnswerStartMarker+"\nvisible final text\n"+finalAnswerEndMarker+"\n● [10:29:09] Finished abc\n")
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

	contentText := resultText(t, result, false)
	if contentText != "Hello specialist" {
		t.Fatalf("unexpected visible content: %q", contentText)
	}
	assertNoAuditTrail(t, contentText)
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
	if len(postedEvents) != 6 {
		t.Fatalf("expected start, three output chunks, progress, and finish events, got %#v", postedEvents)
	}
	if postedEvents[0]["type"] != forgeRunStartedEvent || postedEvents[5]["type"] != forgeRunFinishedEvent {
		t.Fatalf("unexpected forge lifecycle events: %#v", postedEvents)
	}
	finishEvent := postedEvents[5]
	if finishEvent["agent_id"] != "frontend-programmer" || finishEvent["title"] != "Frontend Programmer" || finishEvent["ok"] != true || finishEvent["exit_code"] != 0 {
		t.Fatalf("finish event missing specialist completion metadata: %#v", finishEvent)
	}
	if elapsed, ok := finishEvent["elapsed"].(string); !ok || elapsed == "" {
		t.Fatalf("finish event missing elapsed string: %#v", finishEvent)
	}
	if elapsedMS, ok := finishEvent["elapsed_ms"].(int64); !ok || elapsedMS < 0 {
		t.Fatalf("finish event missing elapsed_ms: %#v", finishEvent)
	}
	tokenUsage, ok := finishEvent["token_usage"].(map[string]any)
	if !ok {
		t.Fatalf("finish event missing token usage metadata: %#v", finishEvent)
	}
	if tokenUsage["estimated"] != true || tokenUsage["source"] != "estimated" {
		t.Fatalf("expected estimated token usage, got %#v", tokenUsage)
	}
	totalTokens, ok := int64FromAny(tokenUsage["total_tokens"])
	if !ok || totalTokens <= 0 {
		t.Fatalf("expected positive total token usage, got %#v", tokenUsage["total_tokens"])
	}
	if sequence, ok := finishEvent["sequence"].(int64); !ok || sequence <= 0 {
		t.Fatalf("finish event missing sequence: %#v", finishEvent)
	}
	if timestamp, ok := finishEvent["timestamp"].(string); !ok || timestamp == "" {
		t.Fatalf("finish event missing timestamp: %#v", finishEvent)
	}
	command, ok := postedEvents[0]["command"].(string)
	if !ok || !strings.HasPrefix(command, "forge --agent frontend-programmer -p ") || !strings.Contains(command, "Task:\\nsay hi") {
		t.Fatalf("unexpected visible command event: %#v", postedEvents[0])
	}
	if strings.Contains(strings.ToLower(command), "redact") {
		t.Fatalf("start event command should display delegated prompt, got %q", command)
	}
	quotedPrompt := strings.TrimPrefix(command, "forge --agent frontend-programmer -p ")
	unquotedPrompt, err := strconv.Unquote(quotedPrompt)
	if err != nil {
		t.Fatalf("command prompt should be shell-quoted with strconv.Quote-compatible syntax, got %q: %v", quotedPrompt, err)
	}
	if unquotedPrompt != ranPrompt {
		t.Fatalf("command prompt should match delegated forge prompt:\ngot  %q\nwant %q", unquotedPrompt, ranPrompt)
	}
	if postedEvents[0]["task"] != "say hi" {
		t.Fatalf("start event should include task summary, got %#v", postedEvents[0])
	}
	prompt, ok := postedEvents[0]["prompt"].(string)
	if !ok || prompt != ranPrompt || !strings.Contains(prompt, "Task:\nsay hi") {
		t.Fatalf("start event should include full visible prompt, got %#v want %q", postedEvents[0], ranPrompt)
	}
	if strings.Contains(strings.ToLower(prompt), "redact") {
		t.Fatalf("start event prompt should not be redacted: %q", prompt)
	}
	if postedEvents[1]["type"] != forgeOutputEvent || postedEvents[1]["stream"] != "stdout" || postedEvents[1]["chunk"] != "streamed hello" {
		t.Fatalf("unexpected stdout forge event: %#v", postedEvents[1])
	}
	if postedEvents[2]["type"] != forgeOutputEvent || postedEvents[2]["stream"] != "stderr" || postedEvents[2]["chunk"] != "\nwarning" {
		t.Fatalf("unexpected stderr forge event: %#v", postedEvents[2])
	}
	if postedEvents[3]["type"] != forgeProgressEvent || postedEvents[3]["message"] != "Forging 2:01m" {
		t.Fatalf("unexpected progress forge event: %#v", postedEvents[3])
	}
	if postedEvents[4]["type"] != forgeOutputEvent || postedEvents[4]["chunk"] != "visible final text\n" {
		t.Fatalf("final markers or lifecycle lines leaked into transcript event: %#v", postedEvents[4])
	}
}

func TestSpecialistToolCallReturnsFinalAnswerOnly(t *testing.T) {
	state := &serverState{
		idleDelay:    0,
		notifyOffice: func(agentID, state, title, officeURL string) bool { return true },
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
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
	contentText := resultText(t, result, false)
	for _, forbidden := range []string{"Considering boilerplate", "I'm thinking", "npm install", "vite v8", "trailing transcript"} {
		if strings.Contains(contentText, forbidden) {
			t.Fatalf("output leaked transcript fragment %q:\n%s", forbidden, contentText)
		}
	}
	if !strings.Contains(contentText, "## Vue Boilerplate Created") || !strings.Contains(contentText, "npm run build passed") {
		t.Fatalf("output missing final answer content:\n%s", contentText)
	}
	assertNoAuditTrail(t, contentText)
}

func TestSpecialistToolCallUsesSuccessFallbackForEmptyOutput(t *testing.T) {
	state := &serverState{
		idleDelay:    0,
		notifyOffice: func(agentID, state, title, officeURL string) bool { return true },
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
			return forgeResult{Output: "", ExitCode: 0}
		},
	}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("backend-programmer"), Arguments: map[string]any{"task": "do work"}})
	contentText := resultText(t, result, false)
	if contentText != "Specialist completed successfully." {
		t.Fatalf("unexpected fallback content: %q", contentText)
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
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
			return forgeResult{Output: finalAnswerStartMarker + "\npartial final status\n" + finalAnswerEndMarker, Stderr: "bad things", ExitCode: 7, Err: errors.New("exit status 7")}
		},
	}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("backend-programmer"), Arguments: map[string]any{"task": "do work"}})
	text := failureResultText(t, result)
	for _, want := range []string{"STATUS: failed", "AGENT: backend_programmer", "TASK: do work", "SUMMARY:", "exit status 7", "bad things", "partial final status", "FOLLOW_UP:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("failure response missing %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "bad things") > strings.Index(text, "partial final status") {
		t.Fatalf("failure response should prefer stderr before final output:\n%s", text)
	}
	assertFailureDoesNotExposeRawPayload(t, text)
}

func TestSpecialistToolCallRejectsMissingTask(t *testing.T) {
	state := &serverState{runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
		t.Fatal("forge should not run without a task")
		return forgeResult{}
	}}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("frontend-programmer"), Arguments: map[string]any{"task": "  "}})
	text := failureResultText(t, result)
	for _, want := range []string{"STATUS: failed", "AGENT: frontend_programmer", "SUMMARY:", "task is required", "FOLLOW_UP:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing task failure response missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "TASK:") {
		t.Fatalf("missing task failure should not fabricate a task line:\n%s", text)
	}
	assertFailureDoesNotExposeRawPayload(t, text)
}

func TestScopedSpecialistToolCallRejectsOtherAgentTool(t *testing.T) {
	frontend, ok := findSpecialistAgent("frontend-programmer")
	if !ok {
		t.Fatal("missing frontend specialist")
	}
	state := &serverState{
		agent: &frontend,
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
			t.Fatal("forge should not run for an out-of-scope tool")
			return forgeResult{}
		},
	}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("backend-programmer"), Arguments: map[string]any{"task": "do backend work"}})
	text := failureResultText(t, result)
	for _, want := range []string{"STATUS: failed", "SUMMARY:", "unknown tool"} {
		if !strings.Contains(text, want) {
			t.Fatalf("scoped unknown tool failure response missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "AGENT:") || strings.Contains(text, "TASK:") {
		t.Fatalf("validation failure should not fabricate agent or task metadata:\n%s", text)
	}
	assertFailureDoesNotExposeRawPayload(t, text)
}

func TestFindSpecialistAgentRejectsCoordinator(t *testing.T) {
	if _, ok := findSpecialistAgent(product.CoordinatorAgentID); ok {
		t.Fatal("coordinator must not be a scoped specialist agent")
	}
}

func TestSpecialistToolCallRejectsNestedDispatch(t *testing.T) {
	t.Setenv(mcpChildDispatchEnv, "1")
	state := &serverState{runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
		t.Fatal("forge should not run during nested dispatch")
		return forgeResult{}
	}}

	result := state.handleToolCall(callParams{Name: product.AgentToolName("frontend-programmer"), Arguments: map[string]any{"task": "say hi"}})
	text := failureResultText(t, result)
	for _, want := range []string{"STATUS: failed", "AGENT: frontend_programmer", "TASK: say hi", "SUMMARY:", "nested", "FOLLOW_UP:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("nested dispatch failure response missing %q:\n%s", want, text)
		}
	}
	assertFailureDoesNotExposeRawPayload(t, text)
}

func TestClassifyForgeStreamEventsSplitsProgressFromTranscript(t *testing.T) {
	events := classifyForgeStreamEvents(strings.Join([]string{
		"\x1b[32m⠏\x1b[0m Forging 2:01m · Ctrl+C to interrupt",
		finalAnswerStartMarker,
		"## Implementation Summary",
		"",
		"Created the app.",
		finalAnswerEndMarker,
		"● [15:04:17] Finished abc",
		"real stderr warning",
	}, "\n"), maxForgeOutputChunkSize)

	if len(events) != 3 {
		t.Fatalf("unexpected event count: %#v", events)
	}
	if events[0].Kind != forgeStreamEventProgress || events[0].Text != "Forging 2:01m" {
		t.Fatalf("unexpected progress event: %#v", events[0])
	}
	if events[1].Kind != forgeStreamEventOutput || events[1].Text != "## Implementation Summary\n\nCreated the app.\n" {
		t.Fatalf("unexpected summary output event: %#v", events[1])
	}
	if events[2].Kind != forgeStreamEventOutput || events[2].Text != "real stderr warning" {
		t.Fatalf("unexpected warning output event: %#v", events[2])
	}
}

func TestClassifyForgeStreamEventsHandlesResearchingProgressAndNonLifecycleBullets(t *testing.T) {
	events := classifyForgeStreamEvents(strings.Join([]string{
		"\x1b[32m⠋\x1b[0m Migrating credentials 00s · Ctrl+C to interrupt",
		"● [15:15:59] Initialize ce55f2e8-3023-4612-8de1-cc17769dc712",
		"⠹ Researching 03s · Ctrl+C to interrupt",
		"● [15:16:07] Execute [/bin/zsh] ls -la",
		"total 0",
		"⠇ Researching 16s · Ctrl+C to interrupt",
		"● [15:16:15] Update Todos 4 item(s)",
		"  󰄗 Scaffold a non-destructive Vue 3 + Vite JavaScript app in vue-app",
	}, "\n"), maxForgeOutputChunkSize)

	if len(events) != 5 {
		t.Fatalf("unexpected event count: %#v", events)
	}
	want := []forgeStreamEvent{
		{Kind: forgeStreamEventProgress, Text: "Migrating credentials 00s"},
		{Kind: forgeStreamEventProgress, Text: "Researching 03s"},
		{Kind: forgeStreamEventOutput, Text: "● [15:16:07] Execute [/bin/zsh] ls -la\ntotal 0\n"},
		{Kind: forgeStreamEventProgress, Text: "Researching 16s"},
		{Kind: forgeStreamEventOutput, Text: "● [15:16:15] Update Todos 4 item(s)\n  󰄗 Scaffold a non-destructive Vue 3 + Vite JavaScript app in vue-app"},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("unexpected events:\ngot  %#v\nwant %#v", events, want)
	}
}

func TestNormalizeForgeProgressMessageSupportsGenericSpinnerStatuses(t *testing.T) {
	for _, tc := range []struct {
		line string
		want string
	}{
		{line: "⠙ Researching 24s · Ctrl+C to interrupt", want: "Researching 24s"},
		{line: "⠋ Migrating credentials 00s · Ctrl+C to interrupt", want: "Migrating credentials 00s"},
		{line: "⠼ Building project 01m · Ctrl+C to interrupt", want: "Building project 01m"},
	} {
		if !isForgeProgressLine(tc.line) {
			t.Fatalf("expected progress line: %q", tc.line)
		}
		if got := normalizeForgeProgressMessage(tc.line); got != tc.want {
			t.Fatalf("unexpected normalized progress: got %q want %q", got, tc.want)
		}
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

	result := defaultForgeRunner(product.Agent{ID: "qa-programmer"}, "Task:\nwrite tests", nil)
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
	if seen[product.NotifyToolName] {
		t.Fatalf("scoped stdio should not expose notify tool: %#v", seen)
	}
	if !seen[product.AgentToolName("frontend-programmer")] {
		t.Fatalf("missing scoped frontend tool in response: %#v", seen)
	}
	if seen[product.AgentToolName("backend-programmer")] {
		t.Fatalf("scoped stdio exposed backend tool: %#v", seen)
	}
	if len(response.Result.Tools) != 1 {
		t.Fatalf("unexpected scoped stdio tool count: got %d want 1", len(response.Result.Tools))
	}
}

func TestNotifyToolIsNotExposedForToolCalls(t *testing.T) {
	state := &serverState{
		notifyOffice: func(agentID, state, title, officeURL string) bool {
			t.Fatal("notify tool should not be callable")
			return false
		},
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
			t.Fatal("notify tool should not run forge")
			return forgeResult{}
		},
	}

	result := state.handleToolCall(callParams{Name: product.NotifyToolName, Arguments: map[string]any{"agent_id": "frontend-programmer", "state": product.OfficeStateThinking, "title": "Frontend Programmer"}})
	text := failureResultText(t, result)
	if !strings.Contains(text, "unknown tool") {
		t.Fatalf("unexpected notify error: %s", text)
	}
	assertFailureDoesNotExposeRawPayload(t, text)
}

func TestEmptyToolNameIsRejected(t *testing.T) {
	state := &serverState{
		notifyOffice: func(agentID, state, title, officeURL string) bool {
			t.Fatal("empty tool name should not notify")
			return false
		},
		runForge: func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
			t.Fatal("empty tool name should not run forge")
			return forgeResult{}
		},
	}

	result := state.handleToolCall(callParams{Name: "", Arguments: map[string]any{}})
	text := failureResultText(t, result)
	if !strings.Contains(text, "unknown tool") {
		t.Fatalf("unexpected error: %s", text)
	}
	assertFailureDoesNotExposeRawPayload(t, text)
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

func resultText(t *testing.T, result map[string]any, wantIsError bool) string {
	t.Helper()
	if _, ok := result["structuredContent"]; ok {
		t.Fatalf("MCP result must not include structuredContent: %#v", result)
	}
	if _, ok := result["is_error"]; ok {
		t.Fatalf("MCP result must use isError key casing, got is_error: %#v", result)
	}
	isError, ok := result["isError"].(bool)
	if !ok {
		t.Fatalf("MCP result missing isError bool: %#v", result)
	}
	if isError != wantIsError {
		t.Fatalf("unexpected isError value: got %v want %v in %#v", isError, wantIsError, result)
	}
	content, ok := result["content"].([]map[string]string)
	if !ok || len(content) != 1 {
		t.Fatalf("MCP result should contain one text content item, got %#v", result["content"])
	}
	if content[0]["type"] != "text" {
		t.Fatalf("MCP content should be text, got %#v", content[0])
	}
	return content[0]["text"]
}

func failureResultText(t *testing.T, result map[string]any) string {
	t.Helper()
	text := resultText(t, result, true)
	if !strings.Contains(text, "STATUS: failed") || !strings.Contains(text, "SUMMARY:") {
		t.Fatalf("failure result should be compact status text, got:\n%s", text)
	}
	return text
}

func assertNoAuditTrail(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, "## Audit Trail") {
		t.Fatalf("visible MCP text must not include audit trail markdown:\n%s", text)
	}
}

func assertFailureDoesNotExposeRawPayload(t *testing.T, text string) {
	t.Helper()
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.Contains(trimmed, "\"ok\"") || strings.Contains(trimmed, "\"stderr\"") || strings.Contains(trimmed, "\"exit_code\"") {
		t.Fatalf("failure response exposed raw JSON payload details:\n%s", text)
	}
	assertNoAuditTrail(t, text)
}
