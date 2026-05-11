# Update Runtime and Agent Instructions for Minimal MCP Responses

## Objective

Align Agent Workforce MCP runtime behavior, bundled agent instructions, bundled delegation-skill instructions, and tests with the new minimal MCP response contract.

This handoff is **not instruction-only**. The runtime response envelope is the source of truth, and the bundled instructions should describe the behavior implemented by `internal/mcp/server.go`.

Successful MCP responses return only:

- `content`: the specialist's compact final answer text, wrapped as MCP text content.
- `isError: false`: explicit success status.

Successful MCP responses do not include:

- `structuredContent`
- audit trail markdown
- duplicated metadata
- raw MCP JSON
- stdout/stderr wrappers
- subprocess details
- progress logs
- full command transcripts

Failed MCP responses return only:

- `content`: compact diagnostic or specialist failure status text, wrapped as MCP text content.
- `isError: true`: explicit failure status.

Failed MCP responses should not expose raw JSON payloads, subprocess wrappers, full stderr/stdout, progress logs, or audit tables by default.

Audit/completion metadata is still recorded in Pixel Agent Office logs when the MCP agent completes. The existing `forge_run_finished` event already includes specialist identity, completion status, exit code, elapsed time, elapsed milliseconds, token usage, sequence, and timestamp. The runtime should preserve those existing event fields, but should not duplicate that metadata in the MCP response envelope.

## Scope

### In scope

- Runtime MCP response behavior in `internal/mcp/server.go`.
- Runtime response-contract tests in `internal/mcp/server_test.go`.
- Bundled coordinator instructions in `internal/assets/files/forge/agents/coordinator.md`.
- Bundled delegation skill instructions in `internal/assets/files/forge/skills/delegate-to-workforce-experts/SKILL.md`.
- Bundled specialist final-response instructions in all programmer agent files.
- Product/instruction tests that assert old audit-trail wording.

### Out of scope

- Removing Pixel Agent Office event logging.
- Removing token usage, elapsed time, exit status, or agent identity from Office completion events.
- Asking specialists to manually construct MCP JSON envelopes.
- Rendering token usage or audit details in the Pixel Agent Office frontend unless existing tests require it; the required behavior is that completion events carry the metadata.

## Relevant Current Locations

Runtime response behavior:

- Successful response envelope: `internal/mcp/server.go:774-783`
- Audit markdown appended to visible content: `internal/mcp/server.go:786-794`
- Audit markdown rendering helpers: `internal/mcp/server.go:797-873`
- Failure JSON text wrapper: `internal/mcp/server.go:875-878`
- Specialist payload assembly and Office completion event: `internal/mcp/server.go:308-439`
- Audit-trail data helper: `internal/mcp/server.go:642-656`

Office frontend behavior:

- Pixel Agent Office currently stores `exit_code` from `forge_run_finished`: `internal/assets/files/frontend/app.js:466-481`
- Rendering token usage or audit details in the frontend is out of scope unless existing tests require it. The required behavior is that completion events carry the metadata.

Runtime and product tests:

- Successful response currently asserts `structuredContent`: `internal/mcp/server_test.go:153`
- Visible content currently asserts `## Audit Trail`: `internal/mcp/server_test.go:164-167`, `internal/mcp/server_test.go:361-364`, `internal/mcp/server_test.go:468-471`
- Success helper currently decodes `structuredContent` or JSON text: `internal/mcp/server_test.go:812-823`
- Coordinator instruction test currently asserts audit-trail preservation: `internal/product/product_test.go:128-130`

Bundled instructions:

- Coordinator response hygiene: `internal/assets/files/forge/agents/coordinator.md:186-198`
- Delegation skill response hygiene and audit section: `internal/assets/files/forge/skills/delegate-to-workforce-experts/SKILL.md:95-123`
- Specialist final-response rules: all programmer agent files under `internal/assets/files/forge/agents/*programmer.md`

Current programmer specialist files:

- `internal/assets/files/forge/agents/backend-programmer.md`
- `internal/assets/files/forge/agents/frontend-programmer.md`
- `internal/assets/files/forge/agents/qa-programmer.md`
- `internal/assets/files/forge/agents/data-programmer.md`
- `internal/assets/files/forge/agents/policy-programmer.md`
- `internal/assets/files/forge/agents/workflow-programmer.md`
- `internal/assets/files/forge/agents/ai-model-programmer.md`
- `internal/assets/files/forge/agents/privacy-security-programmer.md`
- `internal/assets/files/forge/agents/mcp-agent-platform-programmer.md`

## Contract Boundaries

Keep these two layers separate:

1. **Specialist final answer text**
   - Produced by the specialist agent.
   - Compact, user-facing, and suitable for coordinator synthesis.
   - Becomes `content[0].text` after MCP wrapping.
   - Must not include raw command transcripts, progress logs, internal reasoning, or manually constructed MCP JSON.

2. **MCP response envelope**
   - Produced by the MCP server runtime.
   - Successful envelope: `content` plus `isError: false`.
   - Failed envelope: `content` plus `isError: true`.
   - Must not include `structuredContent`, audit tables, raw JSON payloads, duplicated metadata, subprocess details, or stdout/stderr wrappers by default.

Specialists should produce only compact final-answer text. The MCP server wraps that text in the MCP response envelope.

## Failure Response Format

Use a predictable, human-readable failure text shape so tests and coordinator synthesis do not depend on raw JSON strings.

For failures after a specialist and task are known:

```text
STATUS: failed
AGENT: frontend_programmer
TASK: <task if available>

SUMMARY:
<compact error message>

FOLLOW_UP:
<retry guidance if available>
```

For validation failures before an agent or task exists:

```text
STATUS: failed
SUMMARY:
unknown tool: <name>
```

Keep failure text compact and actionable. Do not serialize the internal payload as JSON into `content[0].text`.

## Specialist Response Shape

The specialist's final response should be a compact status block when applicable. This text becomes `content[0].text` in the MCP response.

Fields are optional when they do not apply. Irrelevant fields may be omitted or marked `not applicable`; specialists should not fabricate files, commands, tests, risks, or follow-up items.

### Successful specialist final answer

```text
STATUS: completed
AGENT: backend_programmer
TASK: Add retry handling to metrics worker.

SUMMARY:
Added exponential backoff retry handling for transient queue failures and covered the behaviour with unit tests.

FILES:
src/workers/metrics-consumer.ts: wired retry policy into worker execution path
src/workers/retry-policy.ts: added reusable retry helper
tests/workers/metrics-consumer.test.ts: added retry success and max failure tests

COMMANDS:
npm test -- metrics-consumer: passed
npm run typecheck: passed

TESTS: passed
TYPECHECK_LINT: passed

RISKS:
retry limits are currently hard coded

FOLLOW_UP:
consider moving retry settings to environment config

HUMAN_REVIEW: yes
REASON: change affects worker reliability and failure handling
```

### Minimal successful MCP response envelope

```json
{
  "content": [
    {
      "type": "text",
      "text": "STATUS: completed\nAGENT: backend_programmer\nTASK: Add retry handling to metrics worker.\n\nSUMMARY:\nAdded exponential backoff retry handling for transient queue failures and covered the behaviour with unit tests.\n\nFILES:\nsrc/workers/metrics-consumer.ts: wired retry policy into worker execution path\nsrc/workers/retry-policy.ts: added reusable retry helper\ntests/workers/metrics-consumer.test.ts: added retry success and max failure tests\n\nCOMMANDS:\nnpm test -- metrics-consumer: passed\nnpm run typecheck: passed\n\nTESTS: passed\nTYPECHECK_LINT: passed\n\nRISKS:\nretry limits are currently hard coded\n\nFOLLOW_UP:\nconsider moving retry settings to environment config\n\nHUMAN_REVIEW: yes\nREASON: change affects worker reliability and failure handling"
    }
  ],
  "isError": false
}
```

### Failed specialist final answer

For failures, the specialist text should still be compact and actionable:

```text
STATUS: failed
AGENT: backend_programmer
TASK: Add retry handling to metrics worker.

SUMMARY:
Unable to complete the retry implementation because the metrics worker test suite failed during setup.

FILES:
src/workers/metrics-consumer.ts: inspected
tests/workers/metrics-consumer.test.ts: inspected

COMMANDS:
npm test -- metrics-consumer: failed

TESTS: failed
TYPECHECK_LINT: not run

RISKS:
retry behaviour was not changed because the failure occurred before implementation could be safely validated

FOLLOW_UP:
fix the test environment setup error, then retry the implementation

HUMAN_REVIEW: yes
REASON: implementation did not complete and reliability behaviour remains unchanged
```

### Minimal failed MCP response envelope

```json
{
  "content": [
    {
      "type": "text",
      "text": "STATUS: failed\nAGENT: backend_programmer\nTASK: Add retry handling to metrics worker.\n\nSUMMARY:\nUnable to complete the retry implementation because the metrics worker test suite failed during setup.\n\nFILES:\nsrc/workers/metrics-consumer.ts: inspected\ntests/workers/metrics-consumer.test.ts: inspected\n\nCOMMANDS:\nnpm test -- metrics-consumer: failed\n\nTESTS: failed\nTYPECHECK_LINT: not run\n\nRISKS:\nretry behaviour was not changed because the failure occurred before implementation could be safely validated\n\nFOLLOW_UP:\nfix the test environment setup error, then retry the implementation\n\nHUMAN_REVIEW: yes\nREASON: implementation did not complete and reliability behaviour remains unchanged"
    }
  ],
  "isError": true
}
```

## Implementation Plan

- [x] Task 1. Update successful MCP runtime responses in `internal/mcp/server.go`.
  - Rationale: The runtime currently returns `structuredContent` and visible audit markdown for successful specialist calls. Change `specialistSuccessResult` so successful responses contain only `content` and `isError: false`.
  - Requirements:
    - Use the extracted specialist final answer as the response text.
    - Preserve the existing fallback text for empty successful output, such as `Specialist completed successfully.`
    - Do not append `## Audit Trail` to visible response content.
    - Do not include `structuredContent`, `audit_trail`, token usage, elapsed time, exit code, or duplicated payload metadata in the MCP response envelope.

- [x] Task 2. Update failed MCP runtime responses in `internal/mcp/server.go`.
  - Rationale: Failure paths currently serialize payload JSON into `content[0].text` and do not set `isError: true`.
  - Requirements:
    - Return compact, human-readable diagnostic text in `content[0].text`.
    - Include `isError: true`.
    - Use the failure response format in this handoff for known-specialist failures and validation failures before an agent or task exists.
    - Avoid raw JSON payloads, stdout/stderr wrappers, subprocess details, full command output, and audit tables.
    - Cover unknown tool, nested dispatch refusal, missing task, and Forge execution failure paths.
    - Preserve enough diagnostic content for retry or final synthesis.

- [x] Task 3. Verify and preserve Pixel Agent Office audit/completion metadata in runtime events.
  - Rationale: Minimal MCP responses should not remove operational observability.
  - Requirements:
    - Keep Office start/progress/output/finished event posting intact.
    - Verify that the existing `forge_run_finished` event remains the operational audit source. It already includes specialist identity, status, exit code, elapsed time, elapsed milliseconds, token usage, sequence, and timestamp.
    - Do not rely on MCP response `structuredContent` as the only source of completion metadata.
    - Rendering token usage or audit details in the Pixel Agent Office frontend is out of scope unless existing tests require it. The required behavior is that completion events carry the metadata.

- [x] Task 4. Decide whether to keep or remove audit-trail generation helpers in `internal/mcp/server.go`.
  - Rationale: After MCP responses stop returning audit tables and `structuredContent`, `specialistAuditTrail`, `auditTrailMarkdown`, and related markdown/table helpers may become dead code.
  - Requirements:
    - Remove `specialistAuditTrail` and audit markdown helper functions if they are no longer used after the response payload is simplified.
    - If an `audit_trail` field is intentionally kept for a non-response destination, keep only the compact data helper needed for that destination and remove markdown rendering helpers.
    - Do not remove `estimateForgeTokenUsage` if it is still needed for `forge_run_finished` event metadata.

- [x] Task 5. Update `internal/mcp/server_test.go` for the new response contract.
  - Rationale: Existing tests assert the old behavior, including `structuredContent` and visible audit markdown.
  - Requirements:
    - Replace assertions that successful responses contain `structuredContent`.
    - Assert successful responses include `isError: false`.
    - Assert successful responses do not include `structuredContent`.
    - Assert the response key casing is exactly `isError`, not `is_error`.
    - Assert visible successful content contains the specialist final answer only, not `## Audit Trail`.
    - Assert failure responses include `isError: true` and compact, human-readable diagnostic text matching the failure response format in this handoff.
    - Update or replace helpers that currently decode success payloads from `structuredContent` or JSON text.
    - Keep separate assertions that Office completion events still contain audit/completion metadata.

- [x] Task 6. Update the coordinator response hygiene section in `internal/assets/files/forge/agents/coordinator.md:186-198`.
  - Rationale: Remove the requirement to include audit trails from successful specialist MCP results. Replace it with guidance that audit/completion metadata is recorded in Pixel Agent Office logs and should only be surfaced when the user explicitly asks for operational diagnostics.

- [x] Task 7. Replace the audit-trail requirement in `internal/assets/files/forge/skills/delegate-to-workforce-experts/SKILL.md:95-123`.
  - Rationale: The delegation skill currently tells agents to preserve audit trails from MCP results, which conflicts with the minimal MCP response contract.

- [x] Task 8. Add a `Minimal MCP Response Contract` section to the delegation skill.
  - Rationale: The skill should explicitly state that successful MCP responses contain only `content` and `isError: false`, with no `structuredContent` and no audit trail.
  - Requirement: Also state that failed MCP responses contain `content` and `isError: true`.

- [x] Task 9. Add a `Specialist Response Shape` section to the delegation skill.
  - Rationale: The coordinator and default agents should know that `content[0].text` contains compact final-answer text produced by the specialist.
  - Requirement: Make clear that specialists produce text only and must not manually construct MCP JSON envelopes.

- [x] Task 10. Add Pixel Agent Office audit guidance to the delegation skill.
  - Rationale: Agents should know that audit/completion metadata still exists, but it is captured in Pixel Agent Office logs instead of returned in the MCP response.

- [x] Task 11. Update coordinator instructions to treat the MCP result as specialist final-answer text only.
  - Rationale: The coordinator should synthesize from the compact status block and should not look for structured payloads, audit markdown, raw MCP JSON, or subprocess details.

- [x] Task 12. Update all programmer specialist files to request compact final-answer/status-block responses.
  - Rationale: Because the MCP response will contain only text, specialist final answers should be predictable, compact, and easy for the coordinator to synthesize.
  - Files:
    - `internal/assets/files/forge/agents/backend-programmer.md`
    - `internal/assets/files/forge/agents/frontend-programmer.md`
    - `internal/assets/files/forge/agents/qa-programmer.md`
    - `internal/assets/files/forge/agents/data-programmer.md`
    - `internal/assets/files/forge/agents/policy-programmer.md`
    - `internal/assets/files/forge/agents/workflow-programmer.md`
    - `internal/assets/files/forge/agents/ai-model-programmer.md`
    - `internal/assets/files/forge/agents/privacy-security-programmer.md`
    - `internal/assets/files/forge/agents/mcp-agent-platform-programmer.md`

- [x] Task 13. Use flexible wording for optional status-block fields.
  - Rationale: Not every task has changed files, commands, tests, risks, or follow-up items. The instruction should allow irrelevant fields to be omitted or marked as `not applicable`.

- [x] Task 14. Remove any instruction that says final user answers must include audit trails by default.
  - Rationale: Audit trails now belong in Pixel Agent Office logs, not coordinator-facing MCP response text.

- [x] Task 15. Keep the existing prohibition on raw MCP JSON, subprocess details, stdout/stderr wrappers, progress logs, todo logs, full command output, and raw command transcripts.
  - Rationale: This remains aligned with the token-reduction goal and existing response hygiene rules.

- [~] Task 16. Update product and instruction tests that assert old wording.
  - Rationale: Product tests currently enforce the old audit-trail instruction.
  - Requirements:
    - Update `internal/product/product_test.go:128-130` so it no longer requires coordinator audit-trail preservation.
    - Add or update assertions that coordinator instructions describe minimal specialist MCP results and Pixel Agent Office audit metadata.

- [x] Task 17. Search for stale response-contract wording after edits.
  - Rationale: Old guidance may remain in tests or bundled instructions.
  - Search terms:
    - `Include the audit trail`
    - `Audit Trail`
    - `structuredContent`
    - `audit_trail`
    - `content[0].text`
    - `stdout/stderr wrappers`
  - Requirement: Keep references only where they are explicitly testing forbidden old behavior or documenting what must not be returned.

- [x] Task 18. Run full validation.
  - Rationale: This change affects runtime contract, embedded product assets, and tests.
  - Commands:
    - `gofmt` on modified Go files, if any.
    - `go test ./...`

## Recommended Instruction Text

### Coordinator replacement guidance

Replace the audit-specific instruction currently at `internal/assets/files/forge/agents/coordinator.md:196` with:

```md
- Treat each successful specialist MCP result as the specialist's compact final-answer text. Do not expect `structuredContent`, audit trail markdown, raw MCP JSON, subprocess details, stdout/stderr wrappers, or duplicated metadata in the MCP response. Agent Workforce records specialist completion metadata in Pixel Agent Office logs instead of returning it to the coordinator.
```

### Skill minimal response contract section

Add or replace the audit section in `internal/assets/files/forge/skills/delegate-to-workforce-experts/SKILL.md:107-123` with:

```md
## Minimal MCP Response Contract

Successful Agent Workforce MCP specialist responses contain only:

- `content`: the specialist's compact final-answer text.
- `isError: false`: explicit success status.

Successful responses do not include `structuredContent`, audit trails, raw MCP JSON, subprocess details, stdout/stderr wrappers, progress logs, full command transcripts, or duplicated metadata.

Failed Agent Workforce MCP specialist responses contain only:

- `content`: compact diagnostic or specialist failure status text.
- `isError: true`: explicit failure status.

The response key is exactly `isError`, not `is_error`. Failed responses should be compact, human-readable, and actionable for retry or final synthesis. They should not expose raw JSON payloads, full stdout/stderr, subprocess wrappers, progress logs, or audit tables by default.
```

### Skill specialist response shape section

````md
## Specialist Response Shape

Specialists produce compact final-answer text. The MCP server wraps that text in the MCP response envelope; specialists must not manually emit MCP JSON.

When applicable, specialists should format final answers as a compact status block:

```text
STATUS: completed
AGENT: <specialist_id_with_underscores>
TASK: <short task summary>

SUMMARY:
<what was completed or found>

FILES:
<path>: <short change summary>

COMMANDS:
<command>: <passed/failed/not run>

TESTS: <passed/failed/not run/not applicable>
TYPECHECK_LINT: <passed/failed/not run/not applicable>

RISKS:
<remaining risk or none>

FOLLOW_UP:
<recommended next step or none>

HUMAN_REVIEW: <yes/no>
REASON: <why human review is or is not needed>
```

Fields that do not apply may be omitted or marked `not applicable`. Do not fabricate files, commands, tests, risks, or follow-up items.

Do not include raw command transcripts, stdout/stderr wrappers, progress logs, internal reasoning, manually constructed MCP JSON, or full build output.
````

### Skill audit/completion metadata section

```md
## Audit and Completion Metadata

Agent Workforce records specialist completion metadata in Pixel Agent Office logs when the MCP agent completes. This includes consulted specialist identity, completion status, token usage, response time, elapsed milliseconds, exit status, and timestamps where available.

Do not include audit trails in final user responses unless the user explicitly asks for operational diagnostics.
```

### Specialist agent rule addition

Add this near the existing final-response guidance in each specialist agent file, such as `internal/assets/files/forge/agents/backend-programmer.md:37-44`:

```md
- Format the final response as compact final-answer text when applicable, using fields such as STATUS, AGENT, TASK, SUMMARY, FILES, COMMANDS, TESTS, TYPECHECK_LINT, RISKS, FOLLOW_UP, HUMAN_REVIEW, and REASON. Do not manually construct MCP JSON; the MCP server wraps this text in the response envelope.
```

## Verification Criteria

- [x] Successful MCP responses include `content`.
- [x] Successful MCP responses include `isError: false`.
- [x] Successful MCP responses use exact key casing `isError`, not `is_error`.
- [x] Successful MCP responses do not include `structuredContent`.
- [x] Successful MCP response text contains the specialist final answer only.
- [x] Successful MCP response text does not append `## Audit Trail`.
- [x] Failed MCP responses include `content`.
- [x] Failed MCP responses include `isError: true`.
- [x] Failed MCP responses use exact key casing `isError`, not `is_error`.
- [x] Failed MCP response text follows the compact, human-readable failure response format in this handoff and is not raw JSON.
- [x] Failed MCP response text does not expose stdout/stderr wrappers, subprocess details, full command output, or audit tables by default.
- [x] Pixel Agent Office `forge_run_finished` events still record specialist identity, status, exit code, elapsed time, elapsed milliseconds, token usage, sequence, and timestamp.
- [x] Pixel Agent Office frontend rendering remains unchanged unless tests require displaying token usage or audit details.
- [x] Unused audit-trail generation and audit markdown helpers are removed, or intentionally retained only for a non-response destination with tests.
- [x] Coordinator instructions no longer say to include audit trails from successful specialist MCP results.
- [x] Delegation skill no longer says successful MCP results include audit trails.
- [x] Delegation skill documents the minimal MCP response contract for success and failure.
- [x] Delegation skill includes the expected specialist status-block response shape.
- [x] Delegation skill states that audit/completion metadata is recorded in Pixel Agent Office logs.
- [x] Specialist agents consistently request compact final-answer/status-block responses.
- [x] Specialist instructions remain flexible for tasks where some status-block fields do not apply.
- [x] No bundled instructions encourage exposing `structuredContent`, raw MCP JSON, stdout/stderr wrappers, command transcripts, progress logs, or full build output.
- [x] `internal/mcp/server_test.go` is updated to assert the new runtime response shape.
- [x] `internal/product/product_test.go` is updated to assert the new coordinator instruction wording.
- [x] `go test ./...` passes.

## Potential Risks and Mitigations

1. **Downstream callers may rely on `structuredContent`**
   - Mitigation: Treat this as a breaking response-shape change. Update internal tests and any internal consumers in the same change. Document that operational metadata moved to Pixel Agent Office logs.

2. **Coordinator may stop surfacing useful provenance**
   - Mitigation: State clearly that provenance is still available in Pixel Agent Office logs and should be surfaced only when the user asks for operational diagnostics.

3. **Runtime behavior and bundled instructions may diverge**
   - Mitigation: Implement runtime changes first, then update instructions and tests to describe the implemented behavior.

4. **Failure responses may lose useful retry diagnostics**
   - Mitigation: Keep compact diagnostic text for failures, including specialist identity and high-level failure reason where safe, but omit raw JSON, full stdout/stderr, and subprocess wrappers.

5. **Specialists may produce overly verbose status blocks**
   - Mitigation: Keep status-block fields compact and retain existing instructions to omit raw command transcripts and full build output.

6. **Specialists may fabricate fields that do not apply**
   - Mitigation: Use `when applicable`, allow irrelevant fields to be omitted or marked `not applicable`, and explicitly forbid fabricated files, commands, tests, risks, or follow-up items.

7. **Tests may still encode the old contract indirectly**
   - Mitigation: Search for stale `structuredContent`, `Audit Trail`, `audit_trail`, and audit-preservation wording after edits.

## Rollback Guidance

If runtime changes break downstream consumers unexpectedly:

1. Restore the previous `specialistSuccessResult` behavior in `internal/mcp/server.go`.
2. Restore old `internal/mcp/server_test.go` assertions for `structuredContent` and visible audit trail only if the runtime contract is intentionally rolled back.
3. Restore old coordinator/delegation-skill audit wording only if the product decision is to keep audit trails in MCP responses.
4. Keep any independent response-hygiene improvements that still match the chosen runtime contract.

Do not leave instructions claiming minimal MCP responses if runtime still returns `structuredContent` or audit markdown.

## Alternative Approaches

1. **Instruction-only update**
   - Lower effort, but unsafe unless runtime contract work is explicitly out of scope. It would leave `internal/mcp/server.go` returning `structuredContent` and audit markdown while instructions claim otherwise.

2. **Runtime-only update**
   - Fixes the protocol shape but leaves coordinator, delegation skill, and specialist instructions teaching stale audit-trail behavior.

3. **Only document the MCP envelope**
   - Lower effort, but specialists may keep returning inconsistent free-form responses.

4. **Only update specialist status-block instructions**
   - Improves specialist output, but coordinator/skill instructions would still incorrectly expect audit trails.

5. **Keep audit trails optional in final user answers**
   - More flexible, but risks continuing to spend coordinator tokens on operational metadata. The recommended approach is to keep audit data in Pixel Agent Office logs by default and expose it only on explicit diagnostic request.
