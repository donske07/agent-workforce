package cli

import (
	"path/filepath"
	"testing"
)

func TestUninstallPreservesCustomAgentSource(t *testing.T) {
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	customSource := customAgentPath("custom-reviewer")
	installedProjection := filepath.Join(agentDir(config), "custom-reviewer.md")
	writeTestFile(t, customSource, "custom source")
	writeTestFile(t, installedProjection, "installed projection")
	manifest := &Manifest{
		Agents: []ManifestRecord{{ID: "custom-reviewer", Path: customSource, Source: "custom"}, {ID: "backend-programmer", Path: filepath.Join(agentDir(config), "backend-programmer.md"), Source: "bundled"}},
	}
	writeTestFile(t, manifest.Agents[1].Path, "bundled projection")
	if err := saveManifest(manifest); err != nil {
		t.Fatal(err)
	}

	cmd := uninstallCommand(&CommonOptions{ForgeConfig: config})
	cmd.SetArgs([]string{"--yes", "--skip-mcp-remove"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !exists(customSource) {
		t.Fatalf("uninstall removed user-owned custom source %s", customSource)
	}
	if exists(installedProjection) {
		t.Fatalf("uninstall left custom installed projection %s", installedProjection)
	}
}

func TestUninstallReturnsError_whenManifestIsCorrupt(t *testing.T) {
	// Given
	useTempHome(t)
	writeTestFile(t, manifestPath(), "not-json")
	cmd := uninstallCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--json"})

	// When
	err := cmd.Execute()

	// Then
	if err == nil {
		t.Fatal("expected corrupt manifest to fail uninstall")
	}
}

func TestUninstallReturnsError_whenJSONReportContainsRemovalFailures(t *testing.T) {
	// Given
	useTempHome(t)
	config := filepath.Join(t.TempDir(), "forge")
	nonEmptyDir := filepath.Join(agentDir(config), "backend-programmer.md")
	writeTestFile(t, filepath.Join(nonEmptyDir, "child"), "keep me")
	if err := saveManifest(&Manifest{Agents: []ManifestRecord{{ID: "backend-programmer", Path: nonEmptyDir, Source: "bundled"}}}); err != nil {
		t.Fatal(err)
	}
	cmd := uninstallCommand(&CommonOptions{ForgeConfig: config})
	cmd.SetArgs([]string{"--yes", "--json", "--skip-mcp-remove"})

	// When
	err := cmd.Execute()

	// Then
	if err == nil {
		t.Fatal("expected partial uninstall to return an error in JSON mode")
	}
}

func TestStatusReturnsError_whenManifestIsCorrupt(t *testing.T) {
	// Given
	useTempHome(t)
	writeTestFile(t, manifestPath(), "not-json")
	cmd := statusCommand(&CommonOptions{})

	// When
	err := cmd.Execute()

	// Then
	if err == nil {
		t.Fatal("expected corrupt manifest to fail status")
	}
}
