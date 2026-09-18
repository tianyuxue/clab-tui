package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "clab-tui: error: %v\n", err)
		os.Exit(1)
	}
}
