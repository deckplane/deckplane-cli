package cmd

import (
	"os"

	"github.com/yigit433/kommando/v3"
)

const version = "0.1.0"

// NewApp creates and configures the root deckplane CLI application.
func NewApp() *kommando.App {
	app := kommando.New("deckplane",
		kommando.WithDescription("Deckplane CLI — manage your Deckplane resources from the terminal"),
		kommando.WithOutput(os.Stdout),
		kommando.WithGlobalFlags(
			kommando.Flag{Name: "verbose", Short: 'v', Type: kommando.FlagBool, Description: "enable verbose output"},
		),
	)

	app.AddCommand(versionCmd())
	app.AddCommand(initCmd())
	app.AddCommand(serverCmd())
	app.AddCommand(agentCmd())

	return app
}
