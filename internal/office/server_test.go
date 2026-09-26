package office

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donske07/agent-workforce/internal/product"
)

func TestAgentsJSUsesProductRegistry(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/agents.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(string(data))
	body = strings.TrimPrefix(body, "window.agentWorkforceAgents = ")
	body = strings.TrimSuffix(body, ";")

	var agents []product.Agent
	if err := json.Unmarshal([]byte(body), &agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != len(product.Agents) {
		t.Fatalf("unexpected agent count: got %d want %d", len(agents), len(product.Agents))
	}
	for i, agent := range agents {
		if agent != product.Agents[i] {
			t.Fatalf("agent %d mismatch: got %#v want %#v", i, agent, product.Agents[i])
		}
	}
}

func TestAgentStateRejectsInvalidState(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	resp, err := http.Post(server.URL+"/agent-state", "application/json", strings.NewReader("{\"agent_id\":\"frontend-programmer\",\"state\":\"busy\"}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOfficeRejectsCrossOriginEventReads(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://attacker.example")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusForbidden)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Fatalf("cross-origin response must not opt into CORS, got %q", origin)
	}
}

func TestOfficeRejectsCrossOriginSimplePost(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/forge-event", strings.NewReader(`{"type":"forge_output","run_id":"run-1","agent_id":"frontend-programmer","stream":"stdout","chunk":"forged"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "https://attacker.example")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestOfficeRejectsSameOriginRequest_whenHostIsNotLoopback(t *testing.T) {
	// Given
	req := httptest.NewRequest(http.MethodGet, "http://attacker.example/health", nil)
	req.Host = "attacker.example"
	req.Header.Set("Origin", "http://attacker.example")
	recorder := httptest.NewRecorder()

	// When
	NewHandler().ServeHTTP(recorder, req)

	// Then
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: got %d want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestForgeEventAcceptsValidOutputEvent(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	body := "{\"type\":\"forge_output\",\"run_id\":\"run-1\",\"agent_id\":\"frontend-programmer\",\"stream\":\"stdout\",\"sequence\":1,\"chunk\":\"hello\"}"
	resp, err := http.Post(server.URL+"/forge-event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestFrontendAppDisplaysForgeRunPrompt(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{"<prompt redacted>", "promptRedaction", "redactCommandPrompt", "prompt_redacted"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("frontend app should not redact delegated forge prompts; found %q", forbidden)
		}
	}
	for _, want := range []string{"event?.prompt", "run?.prompt", "normalizeTerminalText(run.prompt)", "renderRunDetails(run)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("frontend app missing visible prompt display logic %q", want)
		}
	}
}

func TestForgeEventRejectsInvalidOutputEvent(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	body := "{\"type\":\"forge_output\",\"run_id\":\"run-1\",\"agent_id\":\"frontend-programmer\",\"stream\":\"debug\",\"chunk\":\"hello\"}"
	resp, err := http.Post(server.URL+"/forge-event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestForgeEventAcceptsValidProgressEvent(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	body := "{\"type\":\"forge_progress\",\"run_id\":\"run-1\",\"agent_id\":\"frontend-programmer\",\"sequence\":2,\"message\":\"Forging 2:01m\"}"
	resp, err := http.Post(server.URL+"/forge-event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestForgeEventRejectsInvalidProgressEvent(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	body := "{\"type\":\"forge_progress\",\"run_id\":\"run-1\",\"agent_id\":\"frontend-programmer\"}"
	resp, err := http.Post(server.URL+"/forge-event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestFrontendDefaultsMonitorAndOutputCollapsed(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"is-monitor-collapsed", "is-output-collapsed", "id=\"monitorPanel\"", "id=\"outputPanel\"", "hidden", "id=\"latestOutputButton\"", "Go to latest", "id=\"officeStatus\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("frontend default HTML missing %q", want)
		}
	}
	for _, removed := range []string{"Forge Agent Office", "Forge MCP Monitor", "Default Forge", "Forge Output", "No Forge output captured yet", "Captured Forge runs", "forgeStatus", "forge-console"} {
		if strings.Contains(body, removed) {
			t.Fatalf("frontend default HTML still contains removed label %q", removed)
		}
	}
	if strings.Contains(strings.ToLower(body), "forge") {
		t.Fatalf("frontend default HTML should not contain forge branding: %s", body)
	}
}
