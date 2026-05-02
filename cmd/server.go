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
			serverSetLicenseCmd(),
			serverSetConfigCmd(),
		},
	}
}

func serverInstallCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "install",
		Description: "Install the Deckplane control plane",
		Flags: []kommando.Flag{
			{Name: "license", Short: 'l', Type: kommando.FlagString, Description: "license JWT copied from Deckplane Cloud → Licenses"},
			{Name: "cloud-url", Type: kommando.FlagString, Description: "Deckplane Cloud base URL (default: https://cloud.deckplane.com)"},
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultServerDataDir, Description: "directory for compose.yml, .env and postgres volume"},
			{Name: "port", Short: 'p', Type: kommando.FlagInt, Default: "3000", Description: "port to expose the control plane on"},
			{Name: "version", Type: kommando.FlagString, Default: "latest", Description: "control plane image tag to pin (e.g. v0.1.5)"},
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
			version, _ := ctx.String("version")
			return server.Install(server.InstallOpts{
				LicenseKey: license,
				CloudURL:   cloudURL,
				DataDir:    dataDir,
				Port:       int(port),
				Version:    version,
				Output:     ctx.Output(),
			})
		},
	}
}

func serverSetLicenseCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "set-license",
		Description: "Replace the license stored in .env (e.g. when rotating keys)",
		Flags: []kommando.Flag{
			{Name: "license", Short: 'l', Type: kommando.FlagString, Description: "new license JWT"},
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultServerDataDir, Description: "install directory produced by `server install`"},
		},
		Execute: func(ctx *kommando.Context) error {
			license, _ := ctx.String("license")
			if license == "" {
				return fmt.Errorf("--license is required\nUsage: deckplane server set-license --license <jwt>")
			}
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultServerDataDir
			}
			return server.SetLicense(server.SetLicenseOpts{
				LicenseKey: license,
				DataDir:    dataDir,
				Output:     ctx.Output(),
			})
		},
	}
}

func serverSetConfigCmd() *kommando.Command {
	return &kommando.Command{
		Name:        "set-config",
		Description: "Set optional configuration values in .env (GitHub OAuth, Google Drive, encryption key, etc.)",
		Flags: []kommando.Flag{
			{Name: "data-dir", Short: 'd', Type: kommando.FlagString, Default: defaultServerDataDir, Description: "install directory produced by `server install`"},
			{Name: "encryption-key", Type: kommando.FlagString, Description: "64-char hex key for encrypting secrets (generate: openssl rand -hex 32)"},
			{Name: "github-client-id", Type: kommando.FlagString, Description: "GitHub OAuth App client ID"},
			{Name: "github-client-secret", Type: kommando.FlagString, Description: "GitHub OAuth App client secret"},
			{Name: "github-callback-url", Type: kommando.FlagString, Description: "GitHub OAuth callback URL (e.g. https://your-domain.com/api/v1/github/callback)"},
			{Name: "frontend-url", Type: kommando.FlagString, Description: "Frontend URL for OAuth redirects (e.g. deckplane://auth/callback)"},
			{Name: "public-base-url", Type: kommando.FlagString, Description: "Public base URL for webhook receivers (e.g. https://your-domain.com)"},
			{Name: "google-client-id", Type: kommando.FlagString, Description: "Google OAuth App client ID"},
			{Name: "google-client-secret", Type: kommando.FlagString, Description: "Google OAuth App client secret"},
			{Name: "google-callback-url", Type: kommando.FlagString, Description: "Google OAuth callback URL"},
			{Name: "restart", Type: kommando.FlagBool, Description: "restart the control plane after applying changes"},
		},
		Execute: func(ctx *kommando.Context) error {
			dataDir, _ := ctx.String("data-dir")
			if dataDir == "" {
				dataDir = defaultServerDataDir
			}

			updates := map[string]string{}
			strFlags := map[string]string{
				"encryption-key":      "ENCRYPTION_KEY",
				"github-client-id":    "GITHUB_CLIENT_ID",
				"github-client-secret": "GITHUB_CLIENT_SECRET",
				"github-callback-url": "GITHUB_CALLBACK_URL",
				"frontend-url":        "FRONTEND_URL",
				"public-base-url":     "PUBLIC_BASE_URL",
				"google-client-id":    "GOOGLE_CLIENT_ID",
				"google-client-secret": "GOOGLE_CLIENT_SECRET",
				"google-callback-url": "GOOGLE_CALLBACK_URL",
			}
			for flag, envKey := range strFlags {
				if v, _ := ctx.String(flag); v != "" {
					updates[envKey] = v
				}
			}
			if len(updates) == 0 {
				return fmt.Errorf("no config values provided — use --help to see available flags")
			}

			restart, _ := ctx.Bool("restart")
			return server.SetConfig(server.SetConfigOpts{
				DataDir: dataDir,
				Updates: updates,
				Restart: restart,
				Output:  ctx.Output(),
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
