package main

import (
	"os"

	"mybuilds/internal/cli/client"
)

func main() {
	if err := client.NewCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
