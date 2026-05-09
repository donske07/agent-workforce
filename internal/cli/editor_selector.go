package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agent-workforce/agent-workforce/internal/assets"
)

func editAgent(config, id string) error {
	editor, err := resolveForgeEditor(config)
	if err != nil {
		return err
	}
	var path string
	if isBundled(id) {
		path = customAgentPath(id)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			content, err := fs.ReadFile(assets.Files, "files/forge/agents/"+id+".md")
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, content, 0o644); err != nil {
				return err
			}
		}
	} else {
		path = customAgentPath(id)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("custom agent not found: %s", id)
		}
	}
	fmt.Println("Opening editor:", editor)
	fmt.Println("Agent file:", path)
	if err := launchEditor(editor, path); err != nil {
		return err
	}
	if errs := validateAgentFile(id, path); len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	_, err = syncAgents(config, false)
	return err
}

func resolveForgeEditor(config string) (string, error) {
	if v := os.Getenv("FORGE_EDITOR"); v != "" {
		return v, nil
	}
	for _, file := range []string{filepath.Join(config, ".forge.toml"), filepath.Join(config, "forge.toml")} {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "editor") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					return strings.Trim(strings.TrimSpace(parts[1]), `"'`), nil
				}
			}
		}
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v, nil
	}
	return "", errors.New("No ForgeCode editor setting found. Configure editor in ForgeCode first.")
}

func launchEditor(editor, path string) error {
	cmd := exec.Command("sh", "-c", editor+" "+shellQuote(path))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

type selectorModel struct {
	title     string
	agents    []AgentInfo
	cursor    int
	selected  int
	manage    bool
	state     map[string]bool
	done      bool
	confirmed bool
}

func (m selectorModel) Init() tea.Cmd { return nil }

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.done = true
			return m, tea.Quit
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.agents)+1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.agents) {
				id := m.agents[m.cursor].ID
				if m.manage {
					m.state[id] = !m.state[id]
				} else {
					if m.selected == m.cursor {
						m.selected = -1
					} else {
						m.selected = m.cursor
					}
				}
			} else if m.cursor == len(m.agents) {
				m.confirmed = true
				m.done = true
				return m, tea.Quit
			} else {
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m selectorModel) View() string {
	var b strings.Builder
	b.WriteString(m.title + "\n↑/↓ move • Enter toggle/select\n\n")
	for i, a := range m.agents {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		checked := " "
		if m.manage {
			if m.state[a.ID] {
				checked = "x"
			}
		} else if m.selected == i {
			checked = "x"
		}
		fmt.Fprintf(&b, "%s [%s] %s\n", cursor, checked, a.ID)
	}
	for i, label := range []string{actionLabel(m.manage), "Exit"} {
		cursor := " "
		if m.cursor == len(m.agents)+i {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s [ %s ]\n", cursor, label)
	}
	return b.String()
}

func actionLabel(manage bool) string {
	if manage {
		return "Confirm changes"
	}
	return "Edit selected"
}

func runManageSelector(agents []AgentInfo) (map[string]bool, bool, error) {
	if !isTTY() {
		return nil, false, errors.New("Interactive agent selection requires a TTY terminal.")
	}
	state := map[string]bool{}
	for _, a := range agents {
		state[a.ID] = a.Active
	}
	model := selectorModel{title: "Agent Activation Manager", agents: agents, manage: true, state: state, selected: -1}
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return nil, false, err
	}
	m := final.(selectorModel)
	return m.state, m.confirmed, nil
}

func runEditSelector(agents []AgentInfo) (string, bool, error) {
	if !isTTY() {
		return "", false, errors.New("Interactive agent selection requires a TTY terminal.")
	}
	model := selectorModel{title: "Agent Editor", agents: agents, manage: false, state: map[string]bool{}, selected: -1}
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return "", false, err
	}
	m := final.(selectorModel)
	if !m.confirmed || m.selected < 0 {
		return "", false, nil
	}
	return agents[m.selected].ID, true, nil
}
