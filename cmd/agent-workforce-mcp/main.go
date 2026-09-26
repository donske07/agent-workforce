package main

import (
	"os"

	workforcemcp "github.com/donske07/agent-workforce/internal/mcp"
)

func main() {
	if err := workforcemcp.Execute(); err != nil {
		os.Exit(1)
	}
}
