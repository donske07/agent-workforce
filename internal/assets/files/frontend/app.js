const agents = (window.agentWorkforceAgents || []).map((agent) => ({
  id: agent.id,
  name: agent.title,
  role: agent.role,
  color: agent.color
}));

const appShell = document.querySelector("#appShell");
const office = document.querySelector("#office");
const agentButtons = document.querySelector("#agentButtons");
const template = document.querySelector("#agentTemplate");
const officeStatus = document.querySelector("#officeStatus");
const clearButton = document.querySelector("#clearButton");
const demoButton = document.querySelector("#demoButton");
const connectionLight = document.querySelector("#connectionLight");
const connectionStatus = document.querySelector("#connectionStatus");
const monitorPanel = document.querySelector("#monitorPanel");
const outputPanel = document.querySelector("#outputPanel");
const monitorToggle = document.querySelector("#monitorToggle");
const outputToggle = document.querySelector("#outputToggle");
const hideMonitorButton = document.querySelector("#hideMonitorButton");
const hideOutputButton = document.querySelector("#hideOutputButton");
const focusButton = document.querySelector("#focusButton");
const clearOutputButton = document.querySelector("#clearOutputButton");
const latestOutputButton = document.querySelector("#latestOutputButton");
const runList = document.querySelector("#runList");
const terminalOutput = document.querySelector("#terminalOutput");
const latestScrollThreshold = 24;
const promptRedaction = "<prompt redacted>";
const thinkingAgents = new Set();
const bubbleMessages = new Map();
const runRecords = new Map();
let activeRunId = "";
let unreadOutputCount = 0;
let runningOutputCount = 0;
let monitorCollapsed = true;
let outputCollapsed = true;
let latestRunId = "";
let terminalFollowingLatest = true;
let forceActiveRunScroll = false;
let demoTimer = null;

function renderAgents() {
  agents.forEach((agent) => {
    const card = template.content.firstElementChild.cloneNode(true);
    card.dataset.agentId = agent.id;
    card.style.setProperty("--agent-color", agent.color);
    card.querySelector("[data-agent-name]").textContent = agent.name;
    card.querySelector("[data-agent-role]").textContent = agent.role;
    card.querySelector("[data-bubble-text]").textContent = "Thinking...";
    card.setAttribute("aria-label", `${agent.name}, ${agent.role}`);
    office.appendChild(card);

    const button = document.createElement("button");
    button.type = "button";
    button.className = "agent-button";
    button.textContent = `Preview ${agent.name}`;
    button.addEventListener("click", () => previewThinking(agent.id));
    agentButtons.appendChild(button);
  });
}

function setAgentThinking(agentId, isThinking, message = "") {
  const agent = agents.find((candidate) => candidate.id === agentId);
  const card = document.querySelector(`[data-agent-id="${CSS.escape(agentId)}"]`);

  if (!agent || !card) {
    return;
  }

  if (message) {
    bubbleMessages.set(agentId, message);
  } else if (!isThinking) {
    bubbleMessages.delete(agentId);
  }

  card.classList.toggle("is-thinking", isThinking);
  card.querySelector("[data-bubble-text]").textContent = bubbleMessages.get(agentId) || "Thinking...";

  if (isThinking) {
    thinkingAgents.add(agentId);
    officeStatus.textContent = message || `Coordinator called ${agent.name}.`;
    return;
  }

  thinkingAgents.delete(agentId);
  officeStatus.textContent = thinkingAgents.size > 0
    ? `${thinkingAgents.size} expert agent${thinkingAgents.size === 1 ? " is" : "s are"} still thinking.`
    : "Ready for the next MCP agent call.";
}

function setMonitorCollapsed(value) {
  monitorCollapsed = value;
  monitorPanel.hidden = value;
  appShell.classList.toggle("is-monitor-collapsed", value);
  monitorToggle.setAttribute("aria-expanded", String(!value));
  monitorToggle.textContent = value ? "Show Monitor" : "Hide Monitor";
}

function setOutputCollapsed(value) {
  outputCollapsed = value;
  outputPanel.hidden = value;
  appShell.classList.toggle("is-output-collapsed", value);
  outputToggle.setAttribute("aria-expanded", String(!value));
  if (!value) {
    unreadOutputCount = 0;
    window.requestAnimationFrame(() => {
      if (terminalFollowingLatest) {
        scrollTerminalToLatest(true);
      } else {
        updateLatestButton();
      }
    });
  } else {
    updateLatestButton();
  }
  updateOutputBadge();
}

function focusPixels() {
  setMonitorCollapsed(true);
  setOutputCollapsed(true);
}

function updateOutputBadge() {
  const parts = [];
  if (runningOutputCount > 0) {
    parts.push("running");
  }
  if (unreadOutputCount > 0) {
    parts.push(String(unreadOutputCount));
  }
  if (parts.length === 0) {
    parts.push(String(runRecords.size));
  }
  outputToggle.textContent = `${outputCollapsed ? "Show" : "Hide"} Output • ${parts.join(" • ")}`;
}

function isViewingLatestRun() {
  return !latestRunId || activeRunId === latestRunId;
}

function isTerminalAtLatest() {
  if (terminalOutput.scrollHeight <= terminalOutput.clientHeight) {
    return true;
  }

  return terminalOutput.scrollHeight - terminalOutput.scrollTop - terminalOutput.clientHeight <= latestScrollThreshold;
}

function updateLatestButton() {
  const hasActiveRun = Boolean(runRecords.get(activeRunId));
  const hasScrollableOutput = terminalOutput.scrollHeight > terminalOutput.clientHeight + latestScrollThreshold;
  const shouldShow = !outputCollapsed && hasActiveRun && (!isViewingLatestRun() || (hasScrollableOutput && !isTerminalAtLatest()));
  latestOutputButton.hidden = !shouldShow;
  latestOutputButton.setAttribute("aria-hidden", String(!shouldShow));
}

function scrollTerminalToLatest(enableFollowing = true) {
  terminalOutput.scrollTop = terminalOutput.scrollHeight;
  terminalFollowingLatest = enableFollowing && isViewingLatestRun();
  updateLatestButton();
}

function goToLatestOutput() {
  if (latestRunId) {
    activeRunId = latestRunId;
  }
  terminalFollowingLatest = true;
  forceActiveRunScroll = true;
  renderOutput();
}

function previewThinking(agentId, duration = 3200) {
  setAgentThinking(agentId, true);
  window.setTimeout(() => setAgentThinking(agentId, false), duration);
}

function clearThinking() {
  thinkingAgents.clear();
  bubbleMessages.clear();
  document.querySelectorAll("[data-agent-card]").forEach((card) => {
    card.classList.remove("is-thinking");
    card.querySelector("[data-bubble-text]").textContent = "Thinking...";
  });
  officeStatus.textContent = "Ready for the next MCP agent call.";

  if (demoTimer) {
    window.clearInterval(demoTimer);
    demoTimer = null;
    demoButton.textContent = "Run delegation demo";
  }
}

function runDemo() {
  if (demoTimer) {
    clearThinking();
    return;
  }

  if (agents.length === 0) {
    officeStatus.textContent = "No agents are available for the delegation demo.";
    return;
  }

  let index = 0;
  demoButton.textContent = "Stop demo";
  previewThinking(agents[index].id, 2600);

  demoTimer = window.setInterval(() => {
    index += 1;

    if (index >= agents.length) {
      window.clearInterval(demoTimer);
      demoTimer = null;
      demoButton.textContent = "Run delegation demo";
      return;
    }

    previewThinking(agents[index].id, 2600);
  }, 900);
}

function commandAgentId(command) {
  const match = String(command || "").match(/(?:^|\s)--agent(?:=|\s+)(?:"([^"]+)"|'([^']+)'|([^\s]+))/);
  return match ? (match[1] || match[2] || match[3] || "") : "";
}

function redactedAgentCommand(agentId = "") {
  return `forge --agent ${agentId || "<agent>"} -p ${promptRedaction}`;
}

function redactCommandPrompt(command, agentId = "") {
  const text = String(command || "").trim();
  const displayAgentId = agentId || commandAgentId(text) || "<agent>";

  if (!text) {
    return redactedAgentCommand(displayAgentId);
  }

  if (text.includes(promptRedaction)) {
    return text;
  }

  const isAgentCommand = /(?:^|\s)(?:\S*\/)?forge(?:\s|$)/.test(text);
  const hasPromptFlag = /(?:^|\s)(?:-p|--prompt)(?:=|\s+)/.test(text);
  if (isAgentCommand && hasPromptFlag) {
    return redactedAgentCommand(displayAgentId);
  }

  return text
    .replace(/(\s(?:-p|--prompt)=)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s]+)/, `$1${promptRedaction}`)
    .replace(/(\s(?:-p|--prompt)\s+)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s]+)/, `$1${promptRedaction}`);
}

function activateRunForEvent(runId) {
  latestRunId = runId;
  if (!activeRunId || terminalFollowingLatest || activeRunId === runId) {
    activeRunId = runId;
  }
  terminalFollowingLatest = activeRunId === runId && isTerminalAtLatest();
}

function ensureRun(event) {
  const runId = String(event.run_id || "");
  if (!runId) {
    return null;
  }

  if (!runRecords.has(runId)) {
    const agentId = event.agent_id || "unknown-agent";
    runRecords.set(runId, {
      id: runId,
      agentId,
      title: event.title || agentId || "Agent Run",
      command: redactCommandPrompt(event.command, agentId),
      status: "running",
      ok: null,
      exitCode: null,
      startedAt: event.timestamp || new Date().toISOString(),
      finishedAt: "",
      chunks: []
    });
  }

  if (!activeRunId) {
    activeRunId = runId;
  }

  return runRecords.get(runId);
}

function handleRunStarted(event) {
  const run = ensureRun(event);
  if (!run) {
    return;
  }

  run.status = "running";
  run.command = redactCommandPrompt(event.command || run.command, run.agentId);
  run.startedAt = event.timestamp || run.startedAt;
  runningOutputCount += 1;
  activateRunForEvent(run.id);
  markOutputActivity();
  renderOutput();
}

function handleRunOutput(event) {
  const run = ensureRun(event);
  if (!run) {
    return;
  }

  run.chunks.push({
    stream: event.stream === "stderr" ? "stderr" : "stdout",
    sequence: Number(event.sequence || 0),
    text: String(event.chunk || "")
  });
  activateRunForEvent(run.id);
  markOutputActivity();
  renderOutput();
}

function handleRunProgress(event) {
  const run = ensureRun(event);
  const message = String(event.message || "").trim();
  if (!run || !message) {
    return;
  }

  run.progress = message;
  setAgentThinking(run.agentId, true, message);
}

function handleRunFinished(event) {
  const run = ensureRun(event);
  if (!run) {
    return;
  }

  run.status = event.ok ? "completed" : "failed";
  run.ok = Boolean(event.ok);
  run.exitCode = Number(event.exit_code ?? 0);
  run.finishedAt = event.timestamp || new Date().toISOString();
  runningOutputCount = Math.max(0, runningOutputCount - 1);
  activateRunForEvent(run.id);
  bubbleMessages.delete(run.agentId);
  setAgentThinking(run.agentId, false);
  markOutputActivity();
  renderOutput();
}

function markOutputActivity() {
  if (outputCollapsed) {
    unreadOutputCount += 1;
  }
  updateOutputBadge();
}

function renderOutput() {
  renderRunList();
  renderActiveRun();
  updateOutputBadge();
}

function renderRunList() {
  runList.textContent = "";
  const runs = Array.from(runRecords.values()).reverse();

  if (runs.length === 0) {
    const empty = document.createElement("p");
    empty.className = "empty-output";
    empty.textContent = "No runs captured yet.";
    runList.appendChild(empty);
    return;
  }

  runs.forEach((run) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "run-button";
    button.classList.toggle("is-active", run.id === activeRunId);
    button.innerHTML = `<strong>${escapeHTML(run.title)}</strong><span>${escapeHTML(run.status)}</span>`;
    button.addEventListener("click", () => {
      activeRunId = run.id;
      terminalFollowingLatest = isViewingLatestRun();
      forceActiveRunScroll = true;
      renderOutput();
    });
    runList.appendChild(button);
  });
}

function renderActiveRun() {
  const run = runRecords.get(activeRunId);

  if (!run) {
    terminalOutput.textContent = "No output captured yet.";
    terminalFollowingLatest = true;
    updateLatestButton();
    return;
  }

  const shouldFollowLatest = forceActiveRunScroll || (isViewingLatestRun() && (terminalFollowingLatest || isTerminalAtLatest()));
  const previousScrollTop = terminalOutput.scrollTop;
  forceActiveRunScroll = false;

  const sortedChunks = [...run.chunks].sort((left, right) => left.sequence - right.sequence);
  const transcript = sortedChunks.map((chunk) => {
    const prefix = chunk.stream === "stderr" ? "[stderr] " : "";
    return `${prefix}${chunk.text}`;
  }).join("");

  terminalOutput.textContent = [
    `${run.title} · ${run.status}`,
    run.command,
    "",
    transcript || "Waiting for output..."
  ].join("\n");

  if (shouldFollowLatest) {
    scrollTerminalToLatest(isViewingLatestRun());
  } else {
    terminalOutput.scrollTop = previousScrollTop;
    terminalFollowingLatest = false;
    updateLatestButton();
  }
}

function clearOutput() {
  runRecords.clear();
  activeRunId = "";
  unreadOutputCount = 0;
  runningOutputCount = 0;
  latestRunId = "";
  terminalFollowingLatest = true;
  forceActiveRunScroll = false;
  renderOutput();
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function connectEventStream() {
  if (!window.EventSource) {
    connectionStatus.textContent = "Event stream unsupported in this browser.";
    return;
  }

  const events = new EventSource("/events");

  events.addEventListener("open", () => {
    connectionLight.classList.add("is-connected");
    connectionStatus.textContent = "Connected. Waiting for MCP agent calls.";
  });

  events.addEventListener("error", () => {
    connectionLight.classList.remove("is-connected");
    connectionStatus.textContent = "Disconnected. Reconnecting...";
  });

  events.addEventListener("message", (message) => {
    let event;

    try {
      event = JSON.parse(message.data);
    } catch (error) {
      connectionStatus.textContent = "Received malformed event data. Waiting for the next update.";
      return;
    }

    if (event.type === "connected") {
      return;
    }

    if (event.type === "agent_state") {
      setAgentThinking(event.agent_id, event.state === "thinking");
      return;
    }

    if (event.type === "forge_run_started") {
      handleRunStarted(event);
      return;
    }

    if (event.type === "forge_output") {
      handleRunOutput(event);
      return;
    }

    if (event.type === "forge_progress") {
      handleRunProgress(event);
      return;
    }

    if (event.type === "forge_run_finished") {
      handleRunFinished(event);
    }
  });
}

renderAgents();
renderOutput();
setMonitorCollapsed(true);
setOutputCollapsed(true);
connectEventStream();
clearButton.addEventListener("click", clearThinking);
demoButton.addEventListener("click", runDemo);
monitorToggle.addEventListener("click", () => setMonitorCollapsed(!monitorCollapsed));
outputToggle.addEventListener("click", () => setOutputCollapsed(!outputCollapsed));
hideMonitorButton.addEventListener("click", () => setMonitorCollapsed(true));
hideOutputButton.addEventListener("click", () => setOutputCollapsed(true));
focusButton.addEventListener("click", focusPixels);
clearOutputButton.addEventListener("click", clearOutput);
latestOutputButton.addEventListener("click", goToLatestOutput);
terminalOutput.addEventListener("scroll", () => {
  terminalFollowingLatest = isViewingLatestRun() && isTerminalAtLatest();
  updateLatestButton();
});

window.pixelOffice = {
  setAgentThinking,
  previewThinking,
  clear: clearThinking,
  clearOutput,
  goToLatestOutput,
  focusPixels,
  agents
};
