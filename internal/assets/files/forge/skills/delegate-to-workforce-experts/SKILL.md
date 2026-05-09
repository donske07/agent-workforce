---
name: delegate-to-workforce-experts
description: Use when a default Forge agent or Agent Workforce Coordinator receives software-system work that should be routed to Agent Workforce programmer specialists through MCP tools, including backend, frontend, AI model, workflow, policy, data, privacy/security, QA, or MCP agent platform implementation. This skill is for Coordinator/default orchestration only and must not be used by MCP-launched programmer specialists.
---

# Delegate to Workforce Experts

Use this skill when a request benefits from a specialist programmer agent for software-system design, implementation, testing, policy, data, AI model, frontend, backend, workflow, privacy/security, QA, or MCP platform work.

## Scope

This skill is for default Forge agents and the Agent Workforce Coordinator only.

MCP-launched programmer specialists must not load or use this skill. If you are running as a specialist agent launched by Agent Workforce MCP, complete your assigned task directly and do not call Agent Workforce MCP tools or re-delegate through this skill.

## Required Behavior

When the request maps to a workforce specialist, call the corresponding Agent Workforce MCP expert tool before producing the final answer. Each programmer specialist is expected to be exposed by its own MCP server registration. Prefer Forge-visible server-qualified wrapper names such as `mcp_agent_workforce_mcp_frontend_programmer_tool_agent_workforce_frontend_programmer`, or raw `agent_workforce_*` names when those are what Forge exposes, rather than invoking MCP commands manually.

Specialist delegation is mandatory for requests that ask to create, scaffold, implement, edit, fix, refactor, test, build, configure, install dependencies, run validation, or inspect domain-specific code. Do not answer these requests with only commands or general instructions when a matching Agent Workforce MCP specialist tool is available.

For frontend application or UI creation requests, call the Frontend Programmer MCP tool, preferring Forge's server-qualified wrapper `mcp_agent_workforce_mcp_frontend_programmer_tool_agent_workforce_frontend_programmer` when visible, or raw `agent_workforce_frontend_programmer` when raw MCP tool names are visible.

Do not use native Forge agent delegation for Agent Workforce specialists. Do not invoke `agent-workforce-mcp` through shell, Python, Go, subprocesses, or manual JSON-RPC.

Send the MCP call with:

- `task`: the concrete specialist task to perform.
- `context`: relevant user constraints, repository context, file references, decisions already made, and known risks.
- `expected_output`: the specific response shape needed for final synthesis.

Treat the MCP result as the specialist's final user-facing answer. Use it as source material, then synthesize the final user-facing answer. The MCP server strips raw Forge execution streams from successful specialist results, so do not ask specialists to return or reproduce raw transcripts.

Do not answer from general knowledge when a relevant specialist exists.

If a user asks for code or project creation and details are missing, choose safe conventional defaults when the request is low-risk and include those defaults in the specialist `context` instead of blocking.

Do not expose raw MCP transcripts, intermediate status, subprocess details, tool output, todo/progress logs, command transcripts, or full build output to the user. Return only the cleaned final result.

## MCP Naming Convention

Each specialist is expected to be discoverable as an individual MCP server registration named:

```text
agent-workforce-mcp-<agent-id>
```

Examples:

- `frontend-programmer` is served by `agent-workforce-mcp-frontend-programmer`
- `backend-programmer` is served by `agent-workforce-mcp-backend-programmer`
- `qa-programmer` is served by `agent-workforce-mcp-qa-programmer`

Derive MCP expert tool names from Agent Workforce programmer agent IDs instead of relying on a hardcoded routing table:

1. Start with the programmer agent ID.
2. Prefix with `agent_workforce_`.
3. Replace hyphens with underscores.

Examples:

- `frontend-programmer` -> `agent_workforce_frontend_programmer`
- `backend-programmer` -> `agent_workforce_backend_programmer`
- `qa-programmer` -> `agent_workforce_qa_programmer`

Forge may expose MCP tools with a server-qualified wrapper name instead of the raw MCP tool name. Prefer server-qualified wrappers when visible because generated Agent Workforce configs register one MCP server per specialist. The wrapper shape is:

```text
mcp_<mcp_server_name_with_underscores>_tool_<agent_workforce_tool_name>
```

Examples:

- `agent-workforce-mcp-frontend-programmer` + `agent_workforce_frontend_programmer` -> `mcp_agent_workforce_mcp_frontend_programmer_tool_agent_workforce_frontend_programmer`
- `agent-workforce-mcp-backend-programmer` + `agent_workforce_backend_programmer` -> `mcp_agent_workforce_mcp_backend_programmer_tool_agent_workforce_backend_programmer`

Legacy aggregate wrapper names such as `mcp_agent_workforce_tool_agent_workforce_frontend_programmer` only work when a shared server named `agent-workforce` is registered. Generated configs use `agent-workforce-mcp-<agent-id>` server names, so do not choose aggregate wrappers when a server-qualified wrapper is available.

Use the available MCP tool list as the source of truth when present. If a matching MCP expert tool is unavailable, proceed normally and mention the limitation only when it materially affects the result. Call the visible server-qualified wrapper or raw tool name; do not invoke the MCP server registration name, `agent-workforce-mcp`, shell commands, subprocesses, or manual JSON-RPC. If a legacy aggregate wrapper reports that the tool is unavailable, retry the matching server-qualified specialist wrapper before falling back.

## Expert Routing

| User request area | Workforce specialist |
|---|---|
| Backend APIs, services, jobs, reliability, integrations, persistence, result contracts | Backend Programmer |
| Frontend UX, interaction flow, components, state, accessibility, responsive UI | Frontend Programmer |
| AI provider integration, model selection, prompt/result contracts, confidence, fallback behavior, evaluation | AI Model Programmer |
| End-to-end workflow orchestration, state transitions, retries, lifecycle handling, result aggregation | Workflow Programmer |
| Rules, thresholds, decision logic, governance, explainability, safe defaults | Policy Programmer |
| Data schemas, storage models, taxonomy structures, migrations, indexing, reporting, data contracts | Data Programmer |
| Privacy, security, retention, audit, access control, safe logging, secret handling | Privacy Security Programmer |
| Test strategy, automated tests, fixtures, evaluation criteria, release gates, regression checks | QA Programmer |
| MCP server behavior, wrapper implementation, tool schemas, agent routing, Office integration | MCP Agent Platform Programmer |

## Response Hygiene

Final user responses must:

- Include only the useful final answer.
- Synthesize specialist output instead of pasting it raw.
- Omit raw expert output.
- Omit intermediate reasoning.
- Omit tool status logs.
- Omit specialist progress text.
- Omit todo/progress logs, command transcripts, and full build output unless the user explicitly asks for diagnostic details.
- Omit MCP subprocess details, command arguments, stdout/stderr wrappers, and JSON payloads unless the user asks for implementation details.
- Include the audit trail supplied by each successful specialist result.

## Audit Trail Format

When an expert was consulted, preserve the specialist result's audit trail in the final answer. The Agent Workforce MCP server supplies the consulted MCP agent, token count, and response time.

Use this format:

```md
## Audit Trail

| Consulted MCP agent | Status | Tokens consumed | Response time |
|---|---|---|---|
| <name> | <Completed/Failed> | <token count> | <duration> |
```

Do not include raw tool names, command arguments, or JSON payloads unless the user specifically asks for implementation details.

## Failure Handling

If an MCP expert call fails, returns incomplete output, or reports an execution error:

1. Retry only if the failure is clearly transient or caused by missing context you can provide.
2. If retry is not appropriate, continue with the best available information.
3. Clearly state any material limitation in the final answer without exposing raw MCP protocol details.
4. Do not fabricate specialist conclusions.

If multiple specialists disagree, reconcile the disagreement explicitly in your synthesis and prefer concrete repository evidence over unsupported claims.

## Multi-Domain Requests

If a request spans multiple areas, consult multiple relevant programmer specialists.

For project creation or scaffolding requests, ask follow-up questions when missing details materially affect safe implementation. Important details include target directory, overwrite behavior, package manager, framework variant, language choice, database/persistence, authentication, deployment target, required integrations, and testing expectations. If safe conventional defaults are reasonable, state those defaults in the specialist `context` and delegate.

Examples:

- Frontend Vue boilerplate application: ask required follow-up questions or choose safe defaults, then call `agent_workforce_frontend_programmer` so the Frontend Programmer writes the code and validates the result.
- NestJS backend application: ask required follow-up questions or choose safe defaults, then call `agent_workforce_backend_programmer` so the Backend Programmer writes the code and validates the result.

- API and frontend flow: backend programmer and frontend programmer.
- Data schema and policy thresholds: data programmer and policy programmer.
- Workflow and QA strategy: workflow programmer and QA programmer.
- Privacy-sensitive storage: privacy security programmer and backend programmer.

## Implementation Requests

For implementation tasks:

1. Delegate to the relevant programmer specialist through MCP before producing the final answer.
2. Include enough context for the programmer to design, implement, test, and verify within its domain.
3. If a safe default is needed, state the default in `context` and continue with delegation.
4. Use the returned specialist output to decide whether additional specialists are needed.
5. Return a concise summary with changed files and verification when implementation work was completed.

Do not replace implementation delegation with command-only advice. If the user asks to create a project, scaffold files, modify code, install packages, or run validation, the relevant programmer specialist should perform the work through MCP. Engineer MCP agents are implementers: they can write and edit files, run commands, install/configure dependencies when needed, and validate changes.

## Fallback

If no relevant specialist exists or MCP specialist tools are unavailable, proceed normally as default Forge and mention the limitation only when it materially affects the result.
