const agents = (window.agentWorkforceAgents || []).map((agent) => ({
  id: agent.id,
  name: agent.title,
  role: agent.role,
  color: agent.color
}));

const office = document.querySelector("#office");
const agentButtons = document.querySelector("#agentButtons");
const template = document.querySelector("#agentTemplate");
const forgeStatus = document.querySelector("#forgeStatus");
const clearButton = document.querySelector("#clearButton");
const demoButton = document.querySelector("#demoButton");
const connectionLight = document.querySelector("#connectionLight");
const connectionStatus = document.querySelector("#connectionStatus");
const thinkingAgents = new Set();
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

function setAgentThinking(agentId, isThinking) {
  const agent = agents.find((candidate) => candidate.id === agentId);
  const card = document.querySelector(`[data-agent-id="${agentId}"]`);

  if (!agent || !card) {
    return;
  }

  card.classList.toggle("is-thinking", isThinking);
  card.querySelector("[data-bubble-text]").textContent = "Thinking...";

  if (isThinking) {
    thinkingAgents.add(agentId);
    forgeStatus.textContent = `Default Forge called ${agent.name}.`;
    return;
  }

  thinkingAgents.delete(agentId);
  forgeStatus.textContent = thinkingAgents.size > 0
    ? `${thinkingAgents.size} expert agent${thinkingAgents.size === 1 ? " is" : "s are"} still thinking.`
    : "Ready for the next MCP agent call.";
}

function previewThinking(agentId, duration = 3200) {
  setAgentThinking(agentId, true);
  window.setTimeout(() => setAgentThinking(agentId, false), duration);
}

function clearThinking() {
  thinkingAgents.clear();
  document.querySelectorAll("[data-agent-card]").forEach((card) => {
    card.classList.remove("is-thinking");
  });
  forgeStatus.textContent = "Ready for the next MCP agent call.";

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
    const event = JSON.parse(message.data);

    if (event.type === "connected") {
      return;
    }

    if (event.type === "agent_state") {
      setAgentThinking(event.agent_id, event.state === "thinking");
    }
  });
}

renderAgents();
connectEventStream();
clearButton.addEventListener("click", clearThinking);
demoButton.addEventListener("click", runDemo);

window.forgeOffice = {
  setAgentThinking,
  previewThinking,
  clear: clearThinking,
  agents
};
