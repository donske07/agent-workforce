package main

import (
	"os"

	"github.com/agent-workforce/agent-workforce/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
