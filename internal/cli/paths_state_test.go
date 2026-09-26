package cli

import (
	"path/filepath"
	"testing"
)

func TestLoadAgentStateSupportsLegacySchema(t *testing.T) {
	useTempHome(t)
	writeTestFile(t, agentStatePath(), `{"agents":{"frontend-programmer":{"active":false},"backend-programmer":{"active":true}},"schemaVersion":1}`)

	state, err := loadAgentState()
	if err != nil {
		t.Fatal(err)
	}
	if state["frontend-programmer"] {
		t.Fatal("expected frontend-programmer to be inactive")
	}
	if !state["backend-programmer"] {
		t.Fatal("expected backend-programmer to be active")
	}
}

func TestLoadAgentStateReturnsError_whenDocumentIsNull(t *testing.T) {
	// Given
	useTempHome(t)
	writeTestFile(t, agentStatePath(), "null")

	// When
	state, err := loadAgentState()

	// Then
	if err == nil {
		t.Fatalf("expected null state document to fail, got %#v", state)
	}
}

func TestResolveForgeEditorIgnoresKeysThatOnlyStartWithEditor(t *testing.T) {
	// Given
	config := t.TempDir()
	t.Setenv("FORGE_EDITOR", "")
	t.Setenv("EDITOR", "/usr/bin/true")
	writeTestFile(t, filepath.Join(config, "forge.toml"), "editor_theme = \"dark\"\n")

	// When
	editor, err := resolveForgeEditor(config)

	// Then
	if err != nil {
		t.Fatal(err)
	}
	if editor != "/usr/bin/true" {
		t.Fatalf("unexpected editor: got %q want %q", editor, "/usr/bin/true")
	}
}
