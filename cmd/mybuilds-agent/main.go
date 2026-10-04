package main

import (
	"mybuilds/internal/cli/agent"
	"os"
)

func main() {
	if err := agent.NewCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
