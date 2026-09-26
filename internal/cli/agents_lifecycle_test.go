package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestActivateRejectsUnknownAgentID(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	opts := &CommonOptions{ForgeConfig: config}
	cmd := agentsActivateCommand(opts, true)
	cmd.SetArgs([]string{"missing-agent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown agent activation to fail")
	}
	if !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("expected agent not found error, got %v", err)
	}
	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state["missing-agent"]; ok {
		t.Fatal("unknown agent was written to state")
	}
}

func TestAgentsDeleteRejectsPathLikeAgentID(t *testing.T) {
	home := useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	victim := filepath.Join(home, ".agent-workforce", "victim.md")
	writeTestFile(t, victim, "keep me")

	cmd := agentsDeleteCommand(&CommonOptions{ForgeConfig: config})
	cmd.SetArgs([]string{"../../victim", "--yes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected path-like agent id to fail")
	}
	if !strings.Contains(err.Error(), "invalid agent id") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists(victim) {
		t.Fatalf("delete escaped the custom agent directory and removed %s", victim)
	}
}

func TestAgentsDeleteReturnsError_whenCustomAgentCannotBeRemoved(t *testing.T) {
	// Given
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	nonEmptyDir := customAgentPath("custom-reviewer")
	writeTestFile(t, filepath.Join(nonEmptyDir, "child"), "keep me")
	cmd := agentsDeleteCommand(&CommonOptions{ForgeConfig: config})
	cmd.SetArgs([]string{"custom-reviewer", "--yes"})

	// When
	err := cmd.Execute()

	// Then
	if err == nil {
		t.Fatal("expected delete failure to be returned")
	}
}
