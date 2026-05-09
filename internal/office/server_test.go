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

	resp, err := http.Post(server.URL+"/agent-state", "application/json", strings.NewReader(`{"agent_id":"frontend-programmer","state":"busy"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
