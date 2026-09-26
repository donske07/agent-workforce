package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donske07/agent-workforce/internal/product"
)

func TestInventoryDoesNotCleanupLegacyArtifacts(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	legacyAgent := legacyAgents[0]
	legacyAgentPath := filepath.Join(agentDir(config), legacyAgent+".md")
	legacySkillPath := filepath.Join(skillDir(config), legacySkills[0], "SKILL.md")
	writeTestFile(t, legacyAgentPath, "legacy agent")
	writeTestFile(t, legacySkillPath, "legacy skill")
	if err := saveAgentState(map[string]bool{legacyAgent: false}); err != nil {
		t.Fatal(err)
	}

	agents, err := inventory(config)
	if err != nil {
		t.Fatal(err)
	}
	_ = agents

	if !exists(legacyAgentPath) {
		t.Fatalf("inventory removed legacy agent %s", legacyAgentPath)
	}
	if !exists(legacySkillPath) {
		t.Fatalf("inventory removed legacy skill %s", legacySkillPath)
	}
	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state[legacyAgent]; !ok {
		t.Fatalf("inventory removed legacy state for %s", legacyAgent)
	}
}

func TestSyncAgentsCleansLegacyAndInstallsBundledAgents(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	legacyAgent := legacyAgents[0]
	legacyAgentPath := filepath.Join(agentDir(config), legacyAgent+".md")
	legacySkillPath := filepath.Join(skillDir(config), legacySkills[0], "SKILL.md")
	writeTestFile(t, legacyAgentPath, "legacy agent")
	writeTestFile(t, legacySkillPath, "legacy skill")
	if err := saveAgentState(map[string]bool{legacyAgent: false}); err != nil {
		t.Fatal(err)
	}

	actions, err := syncAgents(config, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) < len(bundledAgents) {
		t.Fatalf("expected at least %d sync actions, got %d", len(bundledAgents), len(actions))
	}
	if exists(legacyAgentPath) {
		t.Fatalf("sync did not remove legacy agent %s", legacyAgentPath)
	}
	if exists(legacySkillPath) {
		t.Fatalf("sync did not remove legacy skill %s", legacySkillPath)
	}
	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state[legacyAgent]; ok {
		t.Fatalf("sync did not remove legacy state for %s", legacyAgent)
	}
	for _, id := range product.AgentIDs() {
		if !exists(filepath.Join(agentDir(config), id+".md")) {
			t.Fatalf("sync did not install bundled agent %s", id)
		}
	}
}

func TestSyncAgentFilesPrefersBundledOverrides(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	id := bundledAgents[0]
	overrideContent := "---\nid: " + id + "\ntitle: Override\ndescription: Override agent.\n---\n\nOVERRIDE_MARKER\n"
	writeTestFile(t, customAgentPath(id), overrideContent)

	if _, err := syncAgentFiles(config, false); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(agentDir(config), id+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), "OVERRIDE_MARKER") {
		t.Fatalf("bundled override was not synced into active agent file")
	}
}
