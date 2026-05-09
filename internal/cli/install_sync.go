package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agent-workforce/agent-workforce/internal/assets"
	"github.com/agent-workforce/agent-workforce/internal/product"
)

func installAssets(opts CommonOptions) (*Manifest, error) {
	config := forgeConfigDir(opts.ForgeConfig)
	agentsDir := agentDir(config)
	skillsDir := skillDir(config)
	cleanup, err := cleanupLegacy(agentsDir, skillsDir, opts.DryRun)
	if err != nil {
		return nil, err
	}
	if _, err := syncAgentFiles(config, opts.DryRun); err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := installSkills(skillsDir); err != nil {
			return nil, err
		}
	}
	manifest := &Manifest{SchemaVersion: 1, Package: PackageName, Version: Version, InstalledAt: now(), UpdatedAt: now(), LegacyCleanup: cleanup}
	manifest.Forge = map[string]any{"configDir": config, "agentsDir": agentsDir, "skillsDir": skillsDir}
	manifest.MCP = map[string]any{"serverNamePrefix": product.MCPCommand, "serverCount": len(product.SpecialistAgents()), "configWritten": false, "command": MCPCommand}
	agents, err := inventory(config)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		active := a.Active
		sha, err := fileSHA(a.Path)
		if err != nil {
			return nil, err
		}
		manifest.Agents = append(manifest.Agents, ManifestRecord{ID: a.ID, Path: a.Path, Source: a.Source, Active: &active, SHA256: sha})
	}
	for _, skill := range bundledSkills {
		p := filepath.Join(skillsDir, skill, "SKILL.md")
		sha := ""
		if !opts.DryRun {
			var err error
			sha, err = fileSHA(p)
			if err != nil {
				return nil, err
			}
		}
		manifest.Skills = append(manifest.Skills, ManifestRecord{ID: skill, Path: p, SHA256: sha})
	}
	if opts.DryRun {
		return manifest, nil
	}
	if !opts.SkipMCPImport {
		mcpConfigPath, err := writeMCPConfig(opts)
		if err != nil {
			return nil, err
		}
		manifest.MCP["configWritten"] = true
		manifest.MCP["configPath"] = mcpConfigPath
	}
	if err := saveManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func syncAgents(config string, dryRun bool) ([]map[string]any, error) {
	agentsDir := agentDir(config)
	cleanup, err := cleanupLegacy(agentsDir, skillDir(config), dryRun)
	if err != nil {
		return cleanup, err
	}
	actions, err := syncAgentFiles(config, dryRun)
	return append(cleanup, actions...), err
}

func syncAgentFiles(config string, dryRun bool) ([]map[string]any, error) {
	agentsDir := agentDir(config)
	disabledDir := disabledAgentDir(agentsDir)
	state, err := loadAgentState()
	if err != nil {
		return nil, err
	}
	if !dryRun {
		if err := os.MkdirAll(agentsDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(disabledDir, 0o755); err != nil {
			return nil, err
		}
	}
	actions := []map[string]any{}
	seen := map[string]bool{}
	for _, id := range bundledAgents {
		active, ok := state[id]
		if !ok {
			active = true
		}
		content, err := bundledAgentContent(id)
		if err != nil {
			return actions, err
		}
		target := filepath.Join(agentsDir, id+".md")
		disabled := filepath.Join(disabledDir, id+".md")
		action, err := syncOne(id, content, target, disabled, active, dryRun)
		if err != nil {
			return actions, err
		}
		actions = append(actions, action)
		seen[id] = true
	}
	customs, err := filepath.Glob(filepath.Join(customAgentsDir(), "*.md"))
	if err != nil {
		return actions, err
	}
	for _, path := range customs {
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		if isLegacy(id) || seen[id] {
			continue
		}
		active, ok := state[id]
		if !ok {
			active = true
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return actions, err
		}
		target := filepath.Join(agentsDir, id+".md")
		disabled := filepath.Join(disabledDir, id+".md")
		action, err := syncOne(id, content, target, disabled, active, dryRun)
		if err != nil {
			return actions, err
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func syncOne(id string, content []byte, activePath, disabledPath string, active, dryRun bool) (map[string]any, error) {
	action := map[string]any{"id": id, "active": active}
	if active {
		action["path"] = activePath
		if !dryRun {
			if err := os.Remove(disabledPath); err != nil && !os.IsNotExist(err) {
				return action, err
			}
			if err := os.MkdirAll(filepath.Dir(activePath), 0o755); err != nil {
				return action, err
			}
			if err := os.WriteFile(activePath, content, 0o644); err != nil {
				return action, err
			}
		}
	} else {
		action["path"] = disabledPath
		if !dryRun {
			if err := os.Remove(activePath); err != nil && !os.IsNotExist(err) {
				return action, err
			}
			if err := os.MkdirAll(filepath.Dir(disabledPath), 0o755); err != nil {
				return action, err
			}
			if err := os.WriteFile(disabledPath, content, 0o644); err != nil {
				return action, err
			}
		}
	}
	return action, nil
}

func installSkills(skillsDir string) error {
	for _, skill := range bundledSkills {
		content, err := fs.ReadFile(assets.Files, "files/forge/skills/"+skill+"/SKILL.md")
		if err != nil {
			return err
		}
		path := filepath.Join(skillsDir, skill, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func inventory(config string) ([]AgentInfo, error) {
	state, err := loadAgentState()
	if err != nil {
		return nil, err
	}
	infos := []AgentInfo{}
	for _, id := range bundledAgents {
		active, ok := state[id]
		if !ok {
			active = true
		}
		path := bundledAgentPath(config, id, active)
		title := titleFromID(id)
		if agent, ok := product.AgentByID(id); ok {
			title = agent.Title
		}
		infos = append(infos, AgentInfo{ID: id, Source: "bundled", Active: active, Deletable: false, Path: path, Title: title})
	}
	customs, err := filepath.Glob(filepath.Join(customAgentsDir(), "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(customs)
	for _, path := range customs {
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		if isBundled(id) || isLegacy(id) {
			continue
		}
		active, ok := state[id]
		if !ok {
			active = true
		}
		infos = append(infos, AgentInfo{ID: id, Source: "custom", Active: active, Deletable: true, Path: path, Title: agentTitle(path, id)})
	}
	return infos, nil
}

func agentTitle(path, id string) string {
	data, err := os.ReadFile(path)
	if err == nil {
		if title := parseFrontmatter(string(data))["title"]; title != "" {
			return title
		}
	}
	return titleFromID(id)
}

func bundledAgentContent(id string) ([]byte, error) {
	path := customAgentPath(id)
	if _, err := os.Stat(path); err == nil {
		return os.ReadFile(path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return fs.ReadFile(assets.Files, "files/forge/agents/"+id+".md")
}

func bundledAgentPath(config, id string, active bool) string {
	installedPath := filepath.Join(agentDir(config), id+".md")
	if !active {
		installedPath = filepath.Join(disabledAgentDir(agentDir(config)), id+".md")
	}
	if _, err := os.Stat(installedPath); err == nil {
		return installedPath
	}
	if overridePath := customAgentPath(id); exists(overridePath) {
		return overridePath
	}
	return "files/forge/agents/" + id + ".md"
}

func agentExists(agents []AgentInfo, id string) bool {
	for _, agent := range agents {
		if agent.ID == id {
			return true
		}
	}
	return false
}

func cleanupLegacy(agentsDir, skillsDir string, dryRun bool) ([]map[string]any, error) {
	actions := []map[string]any{}
	for _, id := range legacyAgents {
		for _, p := range []string{filepath.Join(agentsDir, id+".md"), filepath.Join(disabledAgentDir(agentsDir), id+".md"), customAgentPath(id)} {
			if _, err := os.Stat(p); err == nil {
				actions = append(actions, map[string]any{"remove": p})
				if !dryRun {
					if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
						return actions, err
					}
				}
			} else if !os.IsNotExist(err) {
				return actions, err
			}
		}
	}
	for _, skill := range legacySkills {
		p := filepath.Join(skillsDir, skill)
		if _, err := os.Stat(p); err == nil {
			actions = append(actions, map[string]any{"remove": p})
			if !dryRun {
				if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
					return actions, err
				}
			}
		} else if !os.IsNotExist(err) {
			return actions, err
		}
	}
	state, err := loadAgentState()
	if err != nil {
		return actions, err
	}
	changed := false
	for _, id := range legacyAgents {
		if _, ok := state[id]; ok {
			delete(state, id)
			changed = true
		}
	}
	if changed && !dryRun {
		if err := saveAgentState(state); err != nil {
			return actions, err
		}
	}
	return actions, nil
}
