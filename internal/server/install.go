// Package server implements the `deckplane server install|update|uninstall`
// workflow. Everything on the host side: config generation, registry auth
// against the short-lived Cloud-minted token, and `docker compose` control.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deckplane/deckplane-cli/internal/cloud"
	"github.com/deckplane/deckplane-cli/internal/docker"
)

const (
	controlImage  = "ghcr.io/deckplane/deckplane-server:latest"
	postgresImage = "postgres:16-alpine"
)

// composeTemplate is written verbatim on first install. Updates leave the
// file untouched — users may have tuned ports/volumes and we don't want to
// clobber their customizations.
const composeTemplate = `services:
  postgres:
    image: ` + postgresImage + `
    restart: unless-stopped
    environment:
      POSTGRES_USER: deckplane
      POSTGRES_PASSWORD: deckplane
      POSTGRES_DB: deckplane
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U deckplane"]
      interval: 5s
      timeout: 5s
      retries: 10

  control:
    image: ` + controlImage + `
    restart: unless-stopped
    env_file: .env
    ports:
      - "${DECKPLANE_PORT:-3000}:3000"
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  pgdata:
`

// InstallOpts carries every user-tunable flag for the install workflow.
// Defaults are applied by the cmd layer before we get here.
type InstallOpts struct {
	LicenseKey string
	CloudURL   string
	DataDir    string
	Port       int
	Output     io.Writer
}

// Install brings up a fresh control plane on this host. Safe to re-run:
// existing secrets in the generated .env are preserved, so `install` after
// a reboot or migration is idempotent.
func Install(opts InstallOpts) error {
	if opts.LicenseKey == "" {
		return fmt.Errorf("--license is required (copy your license JWT from Deckplane Cloud → Licenses)")
	}

	out := opts.Output
	logf := func(format string, args ...any) {
		fmt.Fprintf(out, format+"\n", args...)
	}

	if err := docker.CheckInstalled(); err != nil {
		return fmt.Errorf("docker is not installed or not running — see https://docs.docker.com/get-docker/")
	}
	logf("[+] Docker is installed")

	if err := docker.CheckComposeInstalled(); err != nil {
		return fmt.Errorf("docker compose is not installed — see https://docs.docker.com/compose/install/")
	}
	logf("[+] Docker Compose is installed")

	// Mint a short-lived registry token. Cloud verifies the license
	// signature, expiry, and revocation before handing one out.
	token, err := cloud.New(opts.CloudURL).MintRegistryToken(opts.LicenseKey)
	if err != nil {
		return fmt.Errorf("could not mint registry token: %w", err)
	}
	logf("[+] License accepted — registry token expires %s", token.ExpiresAt)

	if err := os.MkdirAll(opts.DataDir, 0o750); err != nil {
		return fmt.Errorf("could not create %s: %w", opts.DataDir, err)
	}

	if err := writeComposeFile(opts.DataDir); err != nil {
		return err
	}

	envPath := filepath.Join(opts.DataDir, ".env")
	envValues, created, err := ensureEnvFile(envPath, opts.LicenseKey, opts.Port)
	if err != nil {
		return err
	}
	if created {
		logf("[+] Generated fresh secrets at %s", envPath)
	} else {
		logf("[+] Reusing existing secrets at %s", envPath)
	}

	if err := docker.Login(token.Registry, token.Username, token.Token); err != nil {
		return err
	}
	defer docker.Logout(token.Registry)
	logf("[+] Authenticated to %s", token.Registry)

	if err := docker.Compose(opts.DataDir, "pull"); err != nil {
		return fmt.Errorf("image pull failed: %w", err)
	}
	logf("[+] Images pulled")

	if err := docker.Compose(opts.DataDir, "up", "-d"); err != nil {
		return fmt.Errorf("failed to start control plane: %w", err)
	}
	logf("[+] Control plane container started")

	if err := waitForHealth(opts.Port, out); err != nil {
		return err
	}

	logf("\nDeckplane is running on http://localhost:%d", opts.Port)
	logf("")
	logf("Next steps:")
	logf("  1. Create the first admin account:")
	logf("       curl -X POST http://localhost:%d/api/v1/setup/initialize \\", opts.Port)
	logf("         -H 'Content-Type: application/json' \\")
	logf("         -d '{\"email\":\"admin@example.com\",\"password\":\"...\",\"fullName\":\"Admin\"}'")
	logf("  2. Register a host agent with the bootstrap token below.")
	logf("")
	logf("AGENT_BOOTSTRAP_TOKEN: %s", envValues["AGENT_BOOTSTRAP_TOKEN"])
	logf("(store this — you'll paste it into `deckplane agent install --token ...`)")
	return nil
}

// UpdateOpts reuses the existing install — we just need the data dir to
// find the stored license + secrets.
type UpdateOpts struct {
	DataDir string
	Output  io.Writer
}

// Update pulls newer images and restarts the stack without touching any
// secrets. Errors if the data dir wasn't produced by `install`.
func Update(opts UpdateOpts) error {
	envPath := filepath.Join(opts.DataDir, ".env")
	values, err := readEnvFile(envPath)
	if err != nil {
		return fmt.Errorf("could not read %s — is this host installed? run `deckplane server install` first\n  (%w)", envPath, err)
	}
	licenseKey := values["LICENSE_KEY"]
	if licenseKey == "" {
		return fmt.Errorf("LICENSE_KEY missing from %s — re-run `deckplane server install --license ...`", envPath)
	}

	out := opts.Output
	token, err := cloud.New(values["LICENSE_CLOUD_URL"]).MintRegistryToken(licenseKey)
	if err != nil {
		return err
	}

	if err := docker.Login(token.Registry, token.Username, token.Token); err != nil {
		return err
	}
	defer docker.Logout(token.Registry)

	if err := docker.Compose(opts.DataDir, "pull"); err != nil {
		return err
	}
	if err := docker.Compose(opts.DataDir, "up", "-d"); err != nil {
		return err
	}
	fmt.Fprintln(out, "[+] Control plane updated and restarted")
	return nil
}

// UninstallOpts controls whether user data is destroyed.
type UninstallOpts struct {
	DataDir     string
	RemoveData  bool
	Output      io.Writer
}

// Uninstall stops and removes the stack. By default, the postgres volume
// is preserved so users can re-install without losing data.
func Uninstall(opts UninstallOpts) error {
	args := []string{"down"}
	if opts.RemoveData {
		args = append(args, "-v")
	}
	if err := docker.Compose(opts.DataDir, "down", args[1:]...); err != nil {
		return err
	}
	if opts.RemoveData {
		if err := os.RemoveAll(opts.DataDir); err != nil {
			return fmt.Errorf("failed to remove %s: %w", opts.DataDir, err)
		}
		fmt.Fprintln(opts.Output, "[+] Data directory removed")
	}
	fmt.Fprintln(opts.Output, "[+] Control plane stopped")
	return nil
}

// ─── internal helpers ───

func writeComposeFile(dir string) error {
	path := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(composeTemplate), 0o640)
}

func ensureEnvFile(path, licenseKey string, port int) (map[string]string, bool, error) {
	if values, err := readEnvFile(path); err == nil {
		// Update license key if caller gave a fresh one but leave
		// generated secrets alone.
		if licenseKey != "" && licenseKey != values["LICENSE_KEY"] {
			values["LICENSE_KEY"] = licenseKey
			if err := writeEnvFile(path, values); err != nil {
				return nil, false, err
			}
		}
		return values, false, nil
	}

	jwt, err := randomHex(32)
	if err != nil {
		return nil, false, err
	}
	bootstrap, err := randomHex(16)
	if err != nil {
		return nil, false, err
	}

	values := map[string]string{
		"NODE_ENV":              "production",
		"PORT":                  "3000",
		"HOST":                  "0.0.0.0",
		"DATABASE_URL":          "postgresql://deckplane:deckplane@postgres:5432/deckplane",
		"JWT_SECRET":            jwt,
		"AGENT_BOOTSTRAP_TOKEN": bootstrap,
		"LICENSE_KEY":           licenseKey,
		"LICENSE_CLOUD_URL":     cloud.DefaultURL,
		"DECKPLANE_PORT":        fmt.Sprintf("%d", port),
	}
	if err := writeEnvFile(path, values); err != nil {
		return nil, false, err
	}
	return values, true, nil
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"")
	}
	return out, nil
}

func writeEnvFile(path string, values map[string]string) error {
	// Order matters for readability only; pick a stable layout.
	order := []string{
		"NODE_ENV", "PORT", "HOST",
		"DATABASE_URL",
		"JWT_SECRET", "AGENT_BOOTSTRAP_TOKEN",
		"LICENSE_KEY", "LICENSE_CLOUD_URL",
		"DECKPLANE_PORT",
	}
	seen := map[string]bool{}
	var b strings.Builder
	for _, k := range order {
		if v, ok := values[k]; ok {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
			seen[k] = true
		}
	}
	for k, v := range values {
		if !seen[k] {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o640)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func waitForHealth(port int, out io.Writer) error {
	url := fmt.Sprintf("http://localhost:%d/health", port)
	client := &http.Client{Timeout: 3 * time.Second}
	for i := 0; i < 30; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintln(out, "[+] Control plane is healthy")
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintln(out, "[!] Control plane did not respond within 60s")
	fmt.Fprintln(out, "    Check logs: docker logs deckplane-control")
	return nil
}
