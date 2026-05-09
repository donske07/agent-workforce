package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-workforce/agent-workforce/internal/product"
	"github.com/spf13/cobra"
)

type request struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

const (
	mcpChildDispatchEnv     = "AGENT_WORKFORCE_MCP_CHILD"
	finalAnswerStartMarker  = "AGENT_WORKFORCE_FINAL_START"
	finalAnswerEndMarker    = "AGENT_WORKFORCE_FINAL_END"
	maxFailureOutputChars   = 2000
	maxForgeOutputChunkSize = 8000
)

const (
	forgeRunStartedEvent  = "forge_run_started"
	forgeOutputEvent      = "forge_output"
	forgeProgressEvent    = "forge_progress"
	forgeRunFinishedEvent = "forge_run_finished"
)

var (
	ansiCSIRegexp                = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
	ansiOSCRegexp                = regexp.MustCompile("\x1b\\][^\x07]*(\x07|\x1b\\\\)")
	forgeRunSerial               uint64
	forgeLifecycleLineRegexp     = regexp.MustCompile(`^● \[[0-9]{2}:[0-9]{2}:[0-9]{2}\] (Initialize|Finished)\b`)
	forgeProgressInterruptRegexp = regexp.MustCompile(`^\S+\s+.+?\s+[0-9]+(?::[0-9]+)?[smh]?\s+· Ctrl\+C to interrupt$`)
)

type officeNotifier func(agentID, state, title, officeURL string) bool

type officeEventPoster func(event map[string]any, officeURL string) bool

type forgeOutputHandler func(stream, chunk string)

type forgeRunner func(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult

type forgeResult struct {
	Output   string
	Stderr   string
	ExitCode int
	Err      error
}

type serverState struct {
	notifyOffice    officeNotifier
	postOfficeEvent officeEventPoster
	runForge        forgeRunner
	idleDelay       time.Duration
	agent           *product.Agent
}

func newServerState(agent *product.Agent) *serverState {
	return &serverState{
		notifyOffice:    NotifyOffice,
		postOfficeEvent: PostOfficeEvent,
		runForge:        defaultForgeRunner,
		idleDelay:       product.AgentIdleDelay,
		agent:           agent,
	}
}

var workforceAgentToolByName = func() map[string]product.Agent {
	tools := map[string]product.Agent{}
	for _, agent := range product.SpecialistAgents() {
		tools[product.AgentToolName(agent.ID)] = agent
	}
	return tools
}()

func NotifyOffice(agentID, state, title, officeURL string) bool {
	payload := map[string]string{"agent_id": agentID, "state": state, "title": title}
	return postOfficeJSON(officeURL, "/agent-state", payload)
}

func PostOfficeEvent(event map[string]any, officeURL string) bool {
	return postOfficeJSON(officeURL, "/forge-event", event)
}

func postOfficeJSON(officeURL, path string, payload any) bool {
	if officeURL == "" {
		officeURL = product.DefaultOfficeURL()
	}
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(stringsTrimRightSlash(officeURL)+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *serverState) notify(agentID, state, title, officeURL string) bool {
	notify := s.notifyOffice
	if notify == nil {
		notify = NotifyOffice
	}
	return notify(agentID, state, title, officeURL)
}

func (s *serverState) postForgeEvent(event map[string]any, officeURL string) bool {
	post := s.postOfficeEvent
	if post == nil {
		post = PostOfficeEvent
	}
	return post(event, officeURL)
}

func (s *serverState) run(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
	run := s.runForge
	if run == nil {
		run = defaultForgeRunner
	}
	return run(agent, prompt, onOutput)
}

func (s *serverState) notifyIdleAfterDelay(agentID, title, officeURL string) bool {
	if s.idleDelay > 0 {
		time.Sleep(s.idleDelay)
	}
	return s.notify(agentID, product.OfficeStateIdle, title, officeURL)
}

func defaultForgeRunner(agent product.Agent, prompt string, onOutput forgeOutputHandler) forgeResult {
	forgeBin, err := resolveForgeBin()
	if err != nil {
		return forgeResult{ExitCode: -1, Err: err}
	}
	cmd := exec.Command(forgeBin, "--agent", agent.ID, "-p", prompt)
	env := os.Environ()
	env = withEnv(env, mcpChildDispatchEnv, "1")
	env = withEnv(env, "NO_COLOR", "1")
	env = withEnv(env, "CLICOLOR", "0")
	env = withEnv(env, "TERM", "dumb")
	cmd.Env = env

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return forgeResult{ExitCode: -1, Err: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return forgeResult{ExitCode: -1, Err: err}
	}

	if err := cmd.Start(); err != nil {
		return forgeResult{ExitCode: -1, Err: err}
	}

	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go captureForgeStream(stdoutPipe, &stdout, "stdout", onOutput, &wg)
	go captureForgeStream(stderrPipe, &stderr, "stderr", onOutput, &wg)
	wg.Wait()

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return forgeResult{Output: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode, Err: err}
}

func captureForgeStream(reader io.Reader, buffer *bytes.Buffer, stream string, onOutput forgeOutputHandler, wg *sync.WaitGroup) {
	defer wg.Done()
	chunk := make([]byte, 4096)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			text := string(chunk[:n])
			_, _ = buffer.WriteString(text)
			if onOutput != nil {
				onOutput(stream, text)
			}
		}
		if err != nil {
			return
		}
	}
}

func resolveForgeBin() (string, error) {
	if value := os.Getenv("AGENT_WORKFORCE_FORGE_BIN"); value != "" {
		return value, nil
	}
	if value := os.Getenv("FORGE_BIN"); value != "" {
		return value, nil
	}
	return exec.LookPath("forge")
}

func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

func buildForgePrompt(task, context, expectedOutput string) string {
	var prompt strings.Builder
	prompt.WriteString("Task:\n")
	prompt.WriteString(task)
	if context != "" {
		prompt.WriteString("\n\nContext:\n")
		prompt.WriteString(context)
	}
	if expectedOutput != "" {
		prompt.WriteString("\n\nExpected output:\n")
		prompt.WriteString(expectedOutput)
	}
	prompt.WriteString("\n\nFinal response contract:\n")
	prompt.WriteString("Return only a concise final user-facing answer in the final block. Do not include internal reasoning, todo logs, tool transcripts, or full command output in that block. Summarize changed files, verification, and blockers when relevant.\n")
	prompt.WriteString("End your response with this exact marker block, with each marker on its own line:\n")
	prompt.WriteString(finalAnswerStartMarker)
	prompt.WriteString("\n<final answer only>\n")
	prompt.WriteString(finalAnswerEndMarker)
	return prompt.String()
}

func findSpecialistAgent(id string) (product.Agent, bool) {
	for _, agent := range product.SpecialistAgents() {
		if agent.ID == id {
			return agent, true
		}
	}
	return product.Agent{}, false
}

func toolDefinitionForAgent(agent product.Agent) map[string]any {
	return map[string]any{
		"name":        product.AgentToolName(agent.ID),
		"description": agent.Description,
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":            map[string]any{"type": "string", "description": "The task or message for this specialist agent."},
				"context":         map[string]any{"type": "string", "description": "Optional project context, constraints, or relevant file references."},
				"expected_output": map[string]any{"type": "string", "description": "Optional requested response format."},
			},
			"required": []string{"task"},
		},
	}
}

func notifyToolDefinition() map[string]any {
	return map[string]any{
		"name":        product.NotifyToolName,
		"description": "Update the Pixel Agent Office state for a specialist agent.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_id": map[string]any{"type": "string"},
				"state":    map[string]any{"type": "string", "enum": []string{product.OfficeStateThinking, product.OfficeStateIdle}},
				"title":    map[string]any{"type": "string"},
			},
			"required": []string{"agent_id", "state"},
		},
	}
}

func (s *serverState) toolDefinitions() map[string]any {
	tools := []map[string]any{notifyToolDefinition()}
	if s.agent != nil {
		tools = append(tools, toolDefinitionForAgent(*s.agent))
	} else {
		for _, agent := range product.SpecialistAgents() {
			tools = append(tools, toolDefinitionForAgent(agent))
		}
	}
	return map[string]any{"tools": tools}
}

func (s *serverState) agentForTool(name string) (product.Agent, bool) {
	if s.agent != nil {
		if name == product.AgentToolName(s.agent.ID) {
			return *s.agent, true
		}
		return product.Agent{}, false
	}
	agent, ok := workforceAgentToolByName[name]
	return agent, ok
}

func (s *serverState) handleToolCall(params callParams) map[string]any {
	args := params.Arguments
	if args == nil {
		args = map[string]any{}
	}
	if params.Name == product.NotifyToolName || params.Name == "" {
		ok := s.notify(toString(args["agent_id"]), toString(args["state"]), toString(args["title"]), os.Getenv("AGENT_WORKFORCE_OFFICE_URL"))
		return textResult(map[string]any{"ok": ok})
	}
	agent, ok := s.agentForTool(params.Name)
	if !ok {
		return textResult(map[string]any{"ok": false, "error": "unknown tool: " + params.Name})
	}
	if os.Getenv(mcpChildDispatchEnv) == "1" {
		return textResult(map[string]any{"ok": false, "agent_id": agent.ID, "title": agent.Title, "error": "refusing nested Agent Workforce MCP specialist dispatch"})
	}
	task := strings.TrimSpace(toString(args["task"]))
	context := strings.TrimSpace(toString(args["context"]))
	expectedOutput := strings.TrimSpace(toString(args["expected_output"]))
	if task == "" {
		return textResult(map[string]any{"ok": false, "agent_id": agent.ID, "title": agent.Title, "error": "task is required"})
	}
	prompt := buildForgePrompt(task, context, expectedOutput)
	officeURL := os.Getenv("AGENT_WORKFORCE_OFFICE_URL")
	notified := s.notify(agent.ID, product.OfficeStateThinking, agent.Title, officeURL)
	runID := newForgeRunID(agent.ID)
	s.postForgeEvent(map[string]any{
		"type":            forgeRunStartedEvent,
		"run_id":          runID,
		"agent_id":        agent.ID,
		"title":           agent.Title,
		"sequence":        0,
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
		"command":         fmt.Sprintf("forge --agent %s -p <prompt redacted>", agent.ID),
		"prompt_redacted": true,
	}, officeURL)
	var sequence int64
	var progressMu sync.Mutex
	lastProgressMessage := ""
	result := s.run(agent, prompt, func(stream, chunk string) {
		for _, event := range classifyForgeStreamEvents(chunk, maxForgeOutputChunkSize) {
			if event.Text == "" {
				continue
			}
			seq := atomic.AddInt64(&sequence, 1)
			if event.Kind == forgeStreamEventProgress {
				progressMu.Lock()
				isDuplicateProgress := event.Text == lastProgressMessage
				if !isDuplicateProgress {
					lastProgressMessage = event.Text
				}
				progressMu.Unlock()
				if isDuplicateProgress {
					continue
				}
				s.postForgeEvent(map[string]any{
					"type":      forgeProgressEvent,
					"run_id":    runID,
					"agent_id":  agent.ID,
					"title":     agent.Title,
					"message":   event.Text,
					"sequence":  seq,
					"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				}, officeURL)
				continue
			}
			s.postForgeEvent(map[string]any{
				"type":      forgeOutputEvent,
				"run_id":    runID,
				"agent_id":  agent.ID,
				"title":     agent.Title,
				"stream":    stream,
				"sequence":  seq,
				"chunk":     event.Text,
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			}, officeURL)
		}
	})
	success := result.Err == nil && result.ExitCode == 0
	s.postForgeEvent(map[string]any{
		"type":      forgeRunFinishedEvent,
		"run_id":    runID,
		"agent_id":  agent.ID,
		"title":     agent.Title,
		"ok":        success,
		"exit_code": result.ExitCode,
		"sequence":  atomic.AddInt64(&sequence, 1),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}, officeURL)
	idleNotified := false
	if notified {
		idleNotified = s.notifyIdleAfterDelay(agent.ID, agent.Title, officeURL)
	}
	cleanedOutput := cleanForgeText(result.Output)
	output := extractFinalAnswer(cleanedOutput)
	stderr := ""
	if !success {
		output = truncateText(output, maxFailureOutputChars)
		stderr = truncateText(cleanForgeText(result.Stderr), maxFailureOutputChars)
	}
	payload := map[string]any{
		"ok":            success,
		"notified":      notified,
		"idle_notified": idleNotified,
		"agent_id":      agent.ID,
		"title":         agent.Title,
		"output":        output,
		"stderr":        stderr,
		"exit_code":     result.ExitCode,
		"idle_after_ms": product.AgentIdleDelay.Milliseconds(),
	}
	if result.Err != nil {
		payload["error"] = result.Err.Error()
	}
	if success {
		return specialistSuccessResult(payload)
	}
	return textResult(payload)
}

func newForgeRunID(agentID string) string {
	serial := atomic.AddUint64(&forgeRunSerial, 1)
	cleanAgentID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, strings.ToLower(agentID))
	return fmt.Sprintf("%s-%d-%d", cleanAgentID, time.Now().UTC().UnixNano(), serial)
}

type forgeStreamEventKind string

const (
	forgeStreamEventOutput   forgeStreamEventKind = "output"
	forgeStreamEventProgress forgeStreamEventKind = "progress"
)

type forgeStreamEvent struct {
	Kind forgeStreamEventKind
	Text string
}

func classifyForgeStreamEvents(value string, chunkLimit int) []forgeStreamEvent {
	value = ansiOSCRegexp.ReplaceAllString(value, "")
	value = ansiCSIRegexp.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = removeControlChars(value)
	if value == "" {
		return nil
	}

	lines := strings.SplitAfter(value, "\n")
	events := make([]forgeStreamEvent, 0, len(lines))
	var output strings.Builder
	flushOutput := func() {
		text := output.String()
		output.Reset()
		for _, chunk := range chunkForgeOutputText(text, chunkLimit) {
			events = append(events, forgeStreamEvent{Kind: forgeStreamEventOutput, Text: chunk})
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			output.WriteString(line)
			continue
		}
		if isFinalAnswerMarker(trimmed) || isForgeLifecycleLine(trimmed) {
			flushOutput()
			continue
		}
		if isForgeProgressLine(trimmed) {
			flushOutput()
			if message := normalizeForgeProgressMessage(trimmed); message != "" {
				events = append(events, forgeStreamEvent{Kind: forgeStreamEventProgress, Text: message})
			}
			continue
		}
		output.WriteString(line)
	}
	flushOutput()
	return events
}

func sanitizeForgeOutputChunks(value string, chunkLimit int) []string {
	value = ansiOSCRegexp.ReplaceAllString(value, "")
	value = ansiCSIRegexp.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = removeControlChars(value)
	return chunkForgeOutputText(value, chunkLimit)
}

func chunkForgeOutputText(value string, chunkLimit int) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if chunkLimit <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	chunks := make([]string, 0, (len(runes)/chunkLimit)+1)
	for len(runes) > chunkLimit {
		chunks = append(chunks, string(runes[:chunkLimit]))
		runes = runes[chunkLimit:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

func cleanForgeText(value string) string {
	value = ansiOSCRegexp.ReplaceAllString(value, "")
	value = ansiCSIRegexp.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r", "\n")

	lines := strings.Split(value, "\n")
	kept := make([]string, 0, len(lines))
	previousBlank := false
	for _, line := range lines {
		line = strings.TrimRight(removeControlChars(line), " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !previousBlank && len(kept) > 0 {
				kept = append(kept, "")
				previousBlank = true
			}
			continue
		}
		if isForgeProgressLine(trimmed) {
			continue
		}
		kept = append(kept, line)
		previousBlank = false
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func extractFinalAnswer(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if marked, ok := extractMarkedFinalAnswer(value); ok {
		return marked
	}
	if section := extractLastMarkdownSection(value); section != "" {
		return section
	}
	return value
}

func extractMarkedFinalAnswer(value string) (string, bool) {
	start := strings.LastIndex(value, finalAnswerStartMarker)
	if start < 0 {
		return "", false
	}
	contentStart := start + len(finalAnswerStartMarker)
	end := strings.Index(value[contentStart:], finalAnswerEndMarker)
	if end < 0 {
		marked := strings.TrimSpace(value[contentStart:])
		return marked, marked != ""
	}
	marked := strings.TrimSpace(value[contentStart : contentStart+end])
	return marked, marked != ""
}

func extractLastMarkdownSection(value string) string {
	lines := strings.Split(value, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if isFinalMarkdownHeading(strings.TrimSpace(lines[i])) {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return ""
}

func isFinalMarkdownHeading(line string) bool {
	return strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ")
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "\n\n[output truncated]"
}

func removeControlChars(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return r
		}
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
}

func isFinalAnswerMarker(line string) bool {
	return line == finalAnswerStartMarker || line == finalAnswerEndMarker
}

func isForgeLifecycleLine(line string) bool {
	return forgeLifecycleLineRegexp.MatchString(line)
}

func isForgeProgressLine(line string) bool {
	if strings.Contains(line, "· Ctrl+C to interrupt") {
		return startsWithSpinner(line) || forgeProgressInterruptRegexp.MatchString(line)
	}
	if startsWithSpinner(line) && normalizedSpinnerStatus(line) != "" {
		return true
	}
	return false
}

func normalizeForgeProgressMessage(line string) string {
	line = normalizedSpinnerStatus(line)
	if index := strings.Index(line, "· Ctrl+C to interrupt"); index >= 0 {
		line = line[:index]
	}
	line = strings.Join(strings.Fields(line), " ")
	return truncateText(line, 64)
}

func normalizedSpinnerStatus(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, spinnerPrefix(line)))
	if line == "" {
		return ""
	}
	return line
}

func spinnerPrefix(line string) string {
	for _, prefix := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		if strings.HasPrefix(line, prefix) {
			return prefix
		}
	}
	return ""
}

func startsWithSpinner(line string) bool {
	return spinnerPrefix(line) != ""
}

func specialistSuccessResult(payload map[string]any) map[string]any {
	output := strings.TrimSpace(toString(payload["output"]))
	if output == "" {
		output = "Specialist completed successfully."
		payload["output"] = output
	}
	return map[string]any{
		"content":           []map[string]string{{"type": "text", "text": output}},
		"structuredContent": payload,
	}
}

func textResult(payload map[string]any) map[string]any {
	content, _ := json.Marshal(payload)
	return map[string]any{"content": []map[string]string{{"type": "text", "text": string(content)}}}
}

func initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]string{"name": product.PackageName, "version": product.Version},
	}
}

func (s *serverState) handleRequest(req request) (map[string]any, bool) {
	switch req.Method {
	case "initialize":
		return map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": initializeResult()}, true
	case "notifications/initialized":
		return nil, false
	case "tools/list":
		return map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": s.toolDefinitions()}, true
	case "tools/call":
		var params callParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			if req.ID == nil {
				return nil, false
			}
			return map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32602, "message": "Invalid params"}}, true
		}
		result := s.handleToolCall(params)
		return map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}, true
	default:
		if req.ID == nil {
			return nil, false
		}
		return map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "Method not found"}}, true
	}
}

func RunStdio() error {
	return RunStdioForAgent(nil)
}

func RunStdioForAgent(agent *product.Agent) error {
	return runStdio(os.Stdin, os.Stdout, agent)
}

func runStdio(in io.Reader, out io.Writer, agent *product.Agent) error {
	state := newServerState(agent)
	reader := bufio.NewReader(in)
	for {
		message, framed, err := readStdioMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(message)) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(message, &req); err != nil {
			writeStdioPayload(out, framed, map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": -32700, "message": "Invalid JSON"}})
			continue
		}
		response, ok := state.handleRequest(req)
		if ok {
			writeStdioPayload(out, framed, response)
		}
	}
}

func readStdioMessage(reader *bufio.Reader) ([]byte, bool, error) {
	for {
		b, err := reader.Peek(1)
		if err != nil {
			return nil, false, err
		}
		if b[0] != ' ' && b[0] != '\t' && b[0] != '\r' && b[0] != '\n' {
			break
		}
		if _, err := reader.ReadByte(); err != nil {
			return nil, false, err
		}
	}
	if hasContentLengthHeader(reader) {
		return readFramedMessage(reader)
	}
	line, err := reader.ReadBytes('\n')
	if errors.Is(err, io.EOF) && len(line) > 0 {
		return bytes.TrimSpace(line), false, nil
	}
	return bytes.TrimSpace(line), false, err
}

func hasContentLengthHeader(reader *bufio.Reader) bool {
	prefix := "Content-Length:"
	data, err := reader.Peek(len(prefix))
	return err == nil && strings.EqualFold(string(data), prefix)
}

func readFramedMessage(reader *bufio.Reader) ([]byte, bool, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, true, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			length, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || length < 0 {
				return nil, true, fmt.Errorf("invalid Content-Length header: %q", value)
			}
			contentLength = length
		}
	}
	if contentLength < 0 {
		return nil, true, errors.New("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	return body, true, err
}

func writeStdioPayload(out io.Writer, framed bool, payload any) {
	if framed {
		writeFramed(out, payload)
		return
	}
	writeJSONLine(out, payload)
}

func writeFramed(out io.Writer, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(data))
	_, _ = out.Write(data)
}

func writeJSONLine(out io.Writer, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintln(out, string(data))
}

func Execute() error {
	var notify bool
	var agentID, state, title, officeURL, scopedAgentID string
	cmd := &cobra.Command{
		Use:   product.MCPCommand,
		Short: "Agent Workforce MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if notify {
				if agentID == "" || state == "" {
					return errors.New("--agent-id and --state are required with --notify")
				}
				if state != product.OfficeStateThinking && state != product.OfficeStateIdle {
					return fmt.Errorf("--state must be %s or %s", product.OfficeStateThinking, product.OfficeStateIdle)
				}
				if !NotifyOffice(agentID, state, title, officeURL) {
					return errors.New("office notification failed")
				}
				return nil
			}
			if scopedAgentID == "" {
				return RunStdio()
			}
			scopedAgent, ok := findSpecialistAgent(scopedAgentID)
			if !ok {
				return fmt.Errorf("unknown specialist agent: %s", scopedAgentID)
			}
			return RunStdioForAgent(&scopedAgent)
		},
	}
	cmd.Flags().BoolVar(&notify, "notify", false, "Send one office notification and exit.")
	cmd.Flags().StringVar(&scopedAgentID, "agent", "", "Specialist agent id served by this MCP instance.")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent id to notify.")
	cmd.Flags().StringVar(&state, "state", "", fmt.Sprintf("Agent state: %s or %s.", product.OfficeStateThinking, product.OfficeStateIdle))
	cmd.Flags().StringVar(&title, "title", "", "Optional display title.")
	cmd.Flags().StringVar(&officeURL, "office-url", envDefault("AGENT_WORKFORCE_OFFICE_URL", product.DefaultOfficeURL()), "Pixel office URL.")
	return cmd.Execute()
}

func write(payload any) {
	writeJSONLine(os.Stdout, payload)
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func stringsTrimRightSlash(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}
