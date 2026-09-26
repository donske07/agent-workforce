package product

import (
	"fmt"
	"strings"
	"time"
)

const (
	PackageName           = "agent-workforce"
	DefaultOfficeHost     = "127.0.0.1"
	DefaultOfficePort     = 8765
	MCPCommand            = "agent-workforce-mcp"
	NotifyToolName        = "agent_workforce_notify"
	OfficeStateThinking   = "thinking"
	OfficeStateIdle       = "idle"
	DisabledAgentsDirName = ".agent-workforce-disabled"
	AgentIdleDelay        = 3 * time.Second
	CoordinatorAgentID    = "coordinator"
)

var Version = "0.1.0"

type Agent struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Role        string `json:"role"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

var Agents = []Agent{
	{ID: "coordinator", Title: "Coordinator", Role: "Routes work through MCP specialists", Color: "#818cf8", Description: "Coordinates Agent Workforce specialist delegation through MCP tools and synthesizes specialist outputs."},
	{ID: "mcp-agent-platform-programmer", Title: "MCP Agent Platform Programmer", Role: "Builds MCP platform integrations", Color: "#c084fc", Description: "Designs and implements Agent Workforce platform code, MCP wrappers, CLIs, agent routing contracts, Office integration, and validation tooling."},
	{ID: "backend-programmer", Title: "Backend Programmer", Role: "Backend systems and tests", Color: "#38bdf8", Description: "Designs and implements backend services, APIs, persistence, jobs, integrations, reliability behavior, data contracts, and backend tests."},
	{ID: "frontend-programmer", Title: "Frontend Programmer", Role: "Frontend UX, UI, and tests", Color: "#4ade80", Description: "Designs and implements frontend UX, components, state management, interactions, accessibility, responsive behavior, and frontend tests."},
	{ID: "ai-model-programmer", Title: "AI Model Programmer", Role: "AI model integrations", Color: "#f472b6", Description: "Designs and implements AI provider integrations, model selection, prompt/result contracts, confidence handling, fallback behavior, evaluation, and observability."},
	{ID: "workflow-programmer", Title: "Workflow Programmer", Role: "Workflow orchestration", Color: "#fb7185", Description: "Designs and implements workflow orchestration, state transitions, integrations, retries, lifecycle handling, result aggregation, and verification."},
	{ID: "policy-programmer", Title: "Policy Programmer", Role: "Policy and decision logic", Color: "#facc15", Description: "Designs and implements rules, thresholds, decision logic, explainability, governance constraints, safe defaults, and policy validation."},
	{ID: "data-programmer", Title: "Data Programmer", Role: "Data schemas and storage", Color: "#f97316", Description: "Designs and implements data schemas, storage models, migrations, indexing, reporting models, taxonomy structures, and cross-system data contracts."},
	{ID: "privacy-security-programmer", Title: "Privacy Security Programmer", Role: "Privacy and security controls", Color: "#2dd4bf", Description: "Designs and implements privacy, security, retention, access control, auditability, safe logging, secret handling, and security validation."},
	{ID: "qa-programmer", Title: "QA Programmer", Role: "Tests and quality gates", Color: "#f0abfc", Description: "Designs and implements test strategy, automated tests, fixtures, validation scripts, regression checks, CI quality gates, and release verification."},
}

func DefaultOfficeURL() string {
	return fmt.Sprintf("http://%s:%d", DefaultOfficeHost, DefaultOfficePort)
}

func AgentIDs() []string {
	ids := make([]string, 0, len(Agents))
	for _, agent := range Agents {
		ids = append(ids, agent.ID)
	}
	return ids
}

func SpecialistAgents() []Agent {
	agents := make([]Agent, 0, len(Agents))
	for _, agent := range Agents {
		if agent.ID != CoordinatorAgentID {
			agents = append(agents, agent)
		}
	}
	return agents
}

func AgentByID(id string) (Agent, bool) {
	for _, agent := range Agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return Agent{}, false
}

func AgentToolName(id string) string {
	return "agent_workforce_" + strings.ReplaceAll(id, "-", "_")
}

func AgentMCPServerName(id string) string {
	return MCPCommand + "-" + id
}

func ForgeMCPToolName(serverName, toolName string) string {
	return "mcp_" + strings.ReplaceAll(serverName, "-", "_") + "_tool_" + toolName
}
