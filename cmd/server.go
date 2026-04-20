package cmd

import (
	"fmt"

	"github.com/deckplane/deckplane-cli/internal/server"
	"github.com/yigit433/kommando/v3"
)

const defaultServerDataDir = "/var/lib/deckplane"

func serverCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "server",
		Description: "Install, update, and manage the Deckplane control plane on this host",
		SubCommands: []*kommando.Command{
			serverInstallCmd(),
			serverUpdateCmd(),
			serverUninstallCmd(),
		},
	}
}

func serverInstallCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "install",
		Description: "Install the Deckplane control plane",
		Flags: []kommando.Flag{
			{Name: "license", Short: 'l', Type: kommando.FlagString, Description: "license JWT copied from Deckplane Cloud → Licenses"},
			{Name: "cloud-url", Type: kommando.FlagString, Description: "Deckplane Cloud base URL (default: https://cloud.deckplane.io)"},
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultServerDataDir, Description: "directory for compose.yml, .env and postgres volume"},
			{Name: "port", Short: 'p', Type: kommando.FlagInt, Default: "3000", Description: "port to expose the control plane on"},
		},
		Execute: func(ctx *kommando.Context) error {
			license, _ := ctx.String("license")
			if license == "" {
				return fmt.Errorf("--license is required\nUsage: deckplane server install --license <jwt>")
			}
			cloudURL, _ := ctx.String("cloud-url")
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultServerDataDir
			}
			port, _ := ctx.Int("port")
			if port == 0 {
				port = 3000
			}
			return server.Install(server.InstallOpts{
				LicenseKey: license,
				CloudURL:   cloudURL,
				DataDir:    dataDir,
				Port:       int(port),
				Output:     ctx.Output(),
			})
		},
	}
}

func serverUpdateCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "update",
		Description: "Pull the latest control plane image and restart",
		Flags: []kommando.Flag{
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultServerDataDir, Description: "install directory produced by `server install`"},
		},
		Execute: func(ctx *kommando.Context) error {
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultServerDataDir
			}
			return server.Update(server.UpdateOpts{
				DataDir: dataDir,
				Output:  ctx.Output(),
			})
		},
	}
}

func serverUninstallCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "uninstall",
		Description: "Stop the control plane (keeps the postgres volume by default)",
		Flags: []kommando.Flag{
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultServerDataDir, Description: "install directory to tear down"},
			{Name: "remove-data", Type: kommando.FlagBool, Description: "also delete the postgres volume and the install directory"},
		},
		Execute: func(ctx *kommando.Context) error {
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultServerDataDir
			}
			removeData, _ := ctx.Bool("remove-data")
			return server.Uninstall(server.UninstallOpts{
				DataDir:    dataDir,
				RemoveData: removeData,
				Output:     ctx.Output(),
			})
		},
	}
}
