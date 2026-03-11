package cmd

import (
	"fmt"
	"os"

	"github.com/deckplane/deckplane-cli/internal/agent"
	"github.com/yigit433/kommando/v3"
)

func agentCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "agent",
		Description: "Manage DeckPlane agents",
		SubCommands: []*kommando.Command{
			agentInstallCmd(),
		},
	}
}

func agentInstallCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "install",
		Description: "Install DeckPlane agent on the current Docker host",
		Flags: []kommando.Flag{
			{Name: "server-url", Type: kommando.FlagString, Description: "Control Plane URL (e.g., https://deckplane.company.com)"},
			{Name: "token", Short: 't', Type: kommando.FlagString, Description: "bootstrap token from Control Plane UI"},
			{Name: "name", Short: 'n', Type: kommando.FlagString, Description: "agent name (default: hostname)"},
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: "/opt/deckplane-agent", Description: "agent data directory"},
		},
		Execute: runAgentInstall,
	}
}

func runAgentInstall(ctx *kommando.Context) error {
	serverURL, ok := ctx.String("server-url")
	if !ok || serverURL == "" {
		return fmt.Errorf("--server-url is required\nUsage: deckplane agent install --server-url <url> --token <token>")
	}

	token, ok := ctx.String("token")
	if !ok || token == "" {
		return fmt.Errorf("--token is required\nUsage: deckplane agent install --server-url <url> --token <token>")
	}

	name, _ := ctx.String("name")
	if name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to determine hostname, use --name to specify an agent name: %w", err)
		}
		name = hostname
	}

	dataDir, _ := ctx.String("data-dir")
	if dataDir == "" {
		dataDir = "/opt/deckplane-agent"
	}

	return agent.Install(agent.InstallOpts{
		ServerURL: serverURL,
		Token:     token,
		Name:      name,
		DataDir:   dataDir,
		Output:    ctx.Output(),
	})
}
