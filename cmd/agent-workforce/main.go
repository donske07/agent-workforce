package main

import (
	"os"

	"github.com/donske07/agent-workforce/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
