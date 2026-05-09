package main

import (
	"os"

	workforcemcp "github.com/agent-workforce/agent-workforce/internal/mcp"
)

func main() {
	if err := workforcemcp.Execute(); err != nil {
		os.Exit(1)
	}
}
