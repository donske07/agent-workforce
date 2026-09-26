package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donske07/agent-workforce/internal/product"
)

func useTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewCommandLaunchesCoordinatorForgeAndForwardsArgs(t *testing.T) {
	useTempHome(t)
	var gotName string
	var gotArgs []string
	previous := coordinatorForgeRunner
	coordinatorForgeRunner = func(name string, args []string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() { coordinatorForgeRunner = previous })

	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--forge-bin", "/custom/forge", "-p", "Review this repo"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotName != "/custom/forge" {
		t.Fatalf("unexpected forge binary: got %s", gotName)
	}
	wantArgs := []string{"--agent", product.CoordinatorAgentID, "-p", "Review this repo"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected forge args: got %#v want %#v", gotArgs, wantArgs)
	}
}

func TestNewCommandUsesForgeBinEnvAndStopsParsingAfterDoubleDash(t *testing.T) {
	useTempHome(t)
	t.Setenv("FORGE_BIN", "/env/forge")
	var gotName string
	var gotArgs []string
	previous := coordinatorForgeRunner
	coordinatorForgeRunner = func(name string, args []string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() { coordinatorForgeRunner = previous })

	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--", "--forge-bin", "literal", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotName != "/env/forge" {
		t.Fatalf("unexpected forge binary: got %s", gotName)
	}
	wantArgs := []string{"--agent", product.CoordinatorAgentID, "--forge-bin", "literal", "--help"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected forge args: got %#v want %#v", gotArgs, wantArgs)
	}
}

func TestNewCommandRequiresForgeBinValue(t *testing.T) {
	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--forge-bin"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing forge-bin value to fail")
	}
	if !strings.Contains(err.Error(), "--forge-bin requires a value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewCommandReturnsForgeRunnerError(t *testing.T) {
	useTempHome(t)
	wantErr := errors.New("forge failed")
	previous := coordinatorForgeRunner
	coordinatorForgeRunner = func(name string, args []string) error {
		return wantErr
	}
	t.Cleanup(func() { coordinatorForgeRunner = previous })

	cmd := newCommand(&CommonOptions{})
	cmd.SetArgs([]string{"--forge-bin", "/custom/forge"})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected runner error %v, got %v", wantErr, err)
	}
}
