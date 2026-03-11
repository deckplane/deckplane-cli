package cmd

import (
	"fmt"

	"github.com/yigit433/kommando/v3"
)

func versionCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "version",
		Description: "Print the CLI version",
		Aliases:     []string{"v"},
		Execute: func(ctx *kommando.Context) error {
			fmt.Fprintf(ctx.Output(), "deckplane version %s\n", version)
			return nil
		},
	}
}
