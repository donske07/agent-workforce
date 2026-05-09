package office

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-workforce/agent-workforce/internal/product"
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
	for _, want := range []string{"is-monitor-collapsed", "is-output-collapsed", "id=\"monitorPanel\"", "id=\"outputPanel\"", "hidden"} {
		if !strings.Contains(body, want) {
			t.Fatalf("frontend default HTML missing %q", want)
		}
	}
}
