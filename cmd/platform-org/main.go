package main

import (
	"os"

	"github.com/ffreis/platform-org/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
