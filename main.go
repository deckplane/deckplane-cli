package main

import (
	"fmt"
	"os"

	"github.com/deckplane/deckplane-cli/cmd"
)

func main() {
	app := cmd.NewApp()

	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
