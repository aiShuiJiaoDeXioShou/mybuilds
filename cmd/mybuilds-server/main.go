package main

import (
	"os"

	"mybuilds/internal/cli/server"
)

func main() {
	if err := server.NewCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
