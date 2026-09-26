package cli

import (
	"testing"

	"github.com/donske07/agent-workforce/internal/product"
)

func TestRootCommandExposesProductVersion(t *testing.T) {
	// Given
	cmd := rootCommand(&CommonOptions{OfficeURL: product.DefaultOfficeURL()})

	// When
	version := cmd.Version

	// Then
	if version != product.Version {
		t.Fatalf("unexpected version: got %q want %q", version, product.Version)
	}
}
