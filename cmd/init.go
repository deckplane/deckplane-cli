package cmd

import (
	"fmt"

	"github.com/yigit433/kommando/v3"
)

func initCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "init",
		Description: "Initialize a new Deckplane project",
		Aliases:     []string{"i"},

		Flags: []kommando.Flag{
			{Name: "template", Short: 't', Type: kommando.FlagString, Default: "default", Description: "project template to use"},
		},
		Execute: func(ctx *kommando.Context) error {
			args := ctx.Args()
			if len(args) == 0 {
				return fmt.Errorf("project name is required\nUsage: deckplane init <project-name>")
			}
			projectName := args[0]
			template, _ := ctx.String("template")

			fmt.Fprintf(ctx.Output(), "Initializing project %q with template %q...\n", projectName, template)
			// TODO: implement project scaffolding
			fmt.Fprintln(ctx.Output(), "Done!")
			return nil
		},
	}
}
