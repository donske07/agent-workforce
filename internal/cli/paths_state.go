package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/donske07/agent-workforce/internal/assets"
	"github.com/donske07/agent-workforce/internal/product"
)

func writeMCPConfig(opts CommonOptions) (string, error) {
	dir := filepath.Join(stateDir(), "mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	servers := map[string]any{}
	for _, agent := range product.SpecialistAgents() {
		env := map[string]string{
			"AGENT_WORKFORCE_HOME":       stateDir(),
			"AGENT_WORKFORCE_OFFICE_URL": opts.OfficeURL,
		}
		if opts.ForgeBin != "" {
			env["AGENT_WORKFORCE_FORGE_BIN"] = opts.ForgeBin
		}
		servers[product.AgentMCPServerName(agent.ID)] = map[string]any{
			"command": MCPCommand,
			"args":    []string{"--agent", agent.ID},
			"env":     env,
		}
	}
	payload := map[string]any{"mcpServers": servers}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	path := mcpConfigPath()
	return path, os.WriteFile(path, data, 0o644)
}

func removeMCPConfig() error {
	if err := os.Remove(mcpConfigPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func check(name string, ok bool, detail string) map[string]any {
	return map[string]any{"name": name, "ok": ok, "detail": detail}
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func stateDir() string { home, _ := os.UserHomeDir(); return filepath.Join(home, ".agent-workforce") }

func manifestPath() string { return filepath.Join(stateDir(), "manifest.json") }

func mcpConfigPath() string { return filepath.Join(stateDir(), "mcp", PackageName+".json") }

func agentStatePath() string { return filepath.Join(stateDir(), "agents.json") }

func customAgentsDir() string { return filepath.Join(stateDir(), "custom", "agents") }

func customAgentPath(id string) string { return filepath.Join(customAgentsDir(), id+".md") }

func forgeConfigDir(flag string) string {
	if flag != "" {
		return flag
	}
	for index, arg := range os.Args {
		if config, ok := strings.CutPrefix(arg, "--forge-config="); ok {
			return config
		}
		if arg == "--forge-config" && index+1 < len(os.Args) {
			return os.Args[index+1]
		}
	}
	if env := os.Getenv("FORGE_CONFIG"); env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "forge")
}

func agentDir(config string) string { return filepath.Join(config, "agents") }

func skillDir(config string) string { return filepath.Join(config, "skills") }

func disabledAgentDir(agentsDir string) string {
	return filepath.Join(agentsDir, DisabledAgentsDirName)
}

func isBundled(id string) bool {
	return slices.Contains(bundledAgents, id)
}

func isLegacy(id string) bool {
	return slices.Contains(legacyAgents, id)
}

func titleFromID(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func isTTY() bool {
	st, err := os.Stdin.Stat()
	return err == nil && (st.Mode()&os.ModeCharDevice) != 0
}

func printJSON(v any) error {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
	return nil
}

func confirm(prompt string, def bool) (bool, error) {
	if !isTTY() {
		return false, errors.New("confirmation requires a TTY; rerun with --yes or --json")
	}
	choice := "[y/N]"
	if def {
		choice = "[Y/n]"
	}
	fmt.Printf("%s %s ", prompt, choice)
	s, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		if errors.Is(err, io.EOF) {
			return false, errors.New("confirmation input unavailable; rerun with --yes or --json")
		}
		return def, nil
	}
	if s == "y" || s == "yes" {
		return true, nil
	}
	if s == "n" || s == "no" {
		return false, nil
	}
	return false, fmt.Errorf("invalid confirmation response %q", s)
}

func resolveForgeBin(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("FORGE_BIN"); env != "" {
		return env, nil
	}
	return exec.LookPath("forge")
}

func fileSHA(path string) (string, error) {
	var data []byte
	var err error
	if strings.HasPrefix(path, "files/") {
		data, err = fs.ReadFile(assets.Files, path)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func loadManifest() (*Manifest, error) {
	data, err := os.ReadFile(manifestPath())
	if err != nil {
		return nil, err
	}
	var m Manifest
	return &m, json.Unmarshal(data, &m)
}

func saveManifest(m *Manifest) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(), data, 0o644)
}

func loadAgentState() (map[string]bool, error) {
	data, err := os.ReadFile(agentStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	out := map[string]bool{}
	if err := json.Unmarshal(data, &out); err == nil {
		if out == nil {
			return nil, fmt.Errorf("load agent state %s: state must be a JSON object", agentStatePath())
		}
		return out, nil
	}
	legacy := struct {
		Agents map[string]struct {
			Active bool `json:"active"`
		} `json:"agents"`
	}{}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("load agent state %s: %w", agentStatePath(), err)
	}
	for id, agent := range legacy.Agents {
		out[id] = agent.Active
	}
	return out, nil
}

func saveAgentState(state map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(agentStatePath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(agentStatePath(), data, 0o644)
}
