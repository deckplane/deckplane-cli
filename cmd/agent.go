package cmd

import (
	"fmt"
	"os"

	"github.com/deckplane/deckplane-cli/internal/agent"
	"github.com/yigit433/kommando/v3"
)

const defaultAgentDataDir = "/var/lib/deckplane-agent"

func agentCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "agent",
		Description: "Manage Deckplane agents",
		SubCommands: []*kommando.Command{
			agentInstallCmd(),
			agentUpdateCmd(),
			agentUninstallCmd(),
		},
	}
}

func agentInstallCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "install",
		Description: "Install the Deckplane agent on this Docker host",
		Flags: []kommando.Flag{
			{Name: "server-url", Type: kommando.FlagString, Description: "Deckplane control plane URL (e.g. https://deckplane.company.com)"},
			{Name: "token", Short: 't', Type: kommando.FlagString, Description: "bootstrap token from the control plane UI"},
			{Name: "name", Short: 'n', Type: kommando.FlagString, Description: "agent display name (default: hostname)"},
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultAgentDataDir, Description: "directory for compose.yml, .env and state volume"},
			{Name: "version", Type: kommando.FlagString, Default: "latest", Description: "agent image tag to pin (e.g. v0.2.0)"},
		},
		Execute: func(ctx *kommando.Context) error {
			serverURL, _ := ctx.String("server-url")
			if serverURL == "" {
				return fmt.Errorf("--server-url is required\nUsage: deckplane agent install --server-url <url> --token <token>")
			}
			token, _ := ctx.String("token")
			if token == "" {
				return fmt.Errorf("--token is required\nUsage: deckplane agent install --server-url <url> --token <token>")
			}
			name, _ := ctx.String("name")
			if name == "" {
				hostname, err := os.Hostname()
				if err != nil {
					return fmt.Errorf("could not determine hostname — use --name to set an agent name: %w", err)
				}
				name = hostname
			}
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultAgentDataDir
			}
			ver, _ := ctx.String("version")
			return agent.Install(agent.InstallOpts{
				ServerURL: serverURL,
				Token:     token,
				Name:      name,
				DataDir:   dataDir,
				Version:   ver,
				Output:    ctx.Output(),
			})
		},
	}
}

func agentUpdateCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "update",
		Description: "Pull the latest agent image and restart",
		Flags: []kommando.Flag{
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultAgentDataDir, Description: "install directory produced by `agent install`"},
		},
		Execute: func(ctx *kommando.Context) error {
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultAgentDataDir
			}
			return agent.Update(agent.UpdateOpts{
				DataDir: dataDir,
				Output:  ctx.Output(),
			})
		},
	}
}

func agentUninstallCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "uninstall",
		Description: "Stop the agent (keeps the state volume by default so the agent token is preserved)",
		Flags: []kommando.Flag{
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultAgentDataDir, Description: "install directory to tear down"},
			{Name: "remove-data", Type: kommando.FlagBool, Description: "also delete the state volume and the install directory"},
		},
		Execute: func(ctx *kommando.Context) error {
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultAgentDataDir
			}
			removeData, _ := ctx.Bool("remove-data")
			return agent.Uninstall(agent.UninstallOpts{
				DataDir:    dataDir,
				RemoveData: removeData,
				Output:     ctx.Output(),
			})
		},
	}
}
