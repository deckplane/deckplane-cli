package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yigit433/kommando/v3"
)

func loginCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "login",
		Description: "Login to the Deckplane control plane using a custom server license key",
		Flags: []kommando.Flag{
			{Name: "key", Short: 'k', Type: kommando.FlagString, Description: "License key from the Deckplane dashboard"},
			{Name: "server-url", Short: 's', Type: kommando.FlagString, Default: "http://localhost:4000", Description: "Deckplane control plane API URL"},
		},
		Execute: func(ctx *kommando.Context) error {
			key, _ := ctx.String("key")
			if key == "" {
				return fmt.Errorf("--key is required\nUsage: deckplane login --key <your-license-key>")
			}
			serverURL, _ := ctx.String("server-url")

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("could not find home directory: %v", err)
			}
			configDir := filepath.Join(home, ".deckplane")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %v", err)
			}

			configPath := filepath.Join(configDir, "config.json")
			configContent := fmt.Sprintf(`{
  "server_url": "%s",
  "license_key": "%s"
}`, serverURL, key)
			
			if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
				return fmt.Errorf("failed to write config file: %v", err)
			}

			fmt.Fprintf(ctx.Output(), "Successfully authenticated!\n")
			fmt.Fprintf(ctx.Output(), "Control Plane: %s\n", serverURL)
			fmt.Fprintf(ctx.Output(), "Configuration saved to %s\n", configPath)
			return nil
		},
	}
}
