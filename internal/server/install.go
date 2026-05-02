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
	controlImageRepo = "ghcr.io/deckplane/deckplane-server"
	defaultVersion   = "latest"
	postgresImage    = "postgres:16-alpine"
)

func controlImage(version string) string {
	if version == "" {
		version = defaultVersion
	}
	return controlImageRepo + ":" + version
}

// composeTemplateFor renders compose.yml for the given image version and
// network configuration. Written verbatim on first install; subsequent runs
// leave the file untouched so users can tune ports/volumes freely.
func composeTemplateFor(version string, net *NetworkConfig, auth *AuthentikConfig) string {
	installAuthentik := auth != nil && auth.Mode == AuthentikInstall
	if net != nil && net.Mode == NetworkModeTraefik {
		return traefikComposeTemplate(version, net, installAuthentik)
	}
	port := 3000
	if net != nil && net.Port > 0 {
		port = net.Port
	}
	return portComposeTemplate(version, port, installAuthentik)
}

func portComposeTemplate(version string, port int, installAuthentik bool) string {
	portsSection := ""
	if port > 0 {
		portsSection = "    ports:\n      - \"" + fmt.Sprintf("%d", port) + ":3000\"\n"
	}
	authentikBlock := ""
	extraVolumes := ""
	if installAuthentik {
		authentikBlock = authentikPortServices()
		extraVolumes = "  authentik-pgdata:\n  authentik-redis:\n"
	}
	return `services:
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
    image: ` + controlImage(version) + `
    restart: unless-stopped
    env_file: .env
` + portsSection + `    depends_on:
      postgres:
        condition: service_healthy
` + authentikBlock + `
volumes:
  pgdata:
` + extraVolumes
}

func traefikComposeTemplate(version string, net *NetworkConfig, installAuthentik bool) string {
	rule := "traefik.http.routers.deckplane.rule=Host(`" + net.Host + "`)"
	authentikBlock := ""
	extraVolumes := ""
	if installAuthentik {
		authHost := "auth." + net.Host
		authentikBlock = authentikTraefikServices(net.NetworkName, authHost)
		extraVolumes = "  authentik-pgdata:\n  authentik-redis:\n"
	}
	return `services:
  postgres:
    image: ` + postgresImage + `
    restart: unless-stopped
    environment:
      POSTGRES_USER: deckplane
      POSTGRES_PASSWORD: deckplane
      POSTGRES_DB: deckplane
    volumes:
      - pgdata:/var/lib/postgresql/data
    networks:
      - internal
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U deckplane"]
      interval: 5s
      timeout: 5s
      retries: 10

  control:
    image: ` + controlImage(version) + `
    restart: unless-stopped
    env_file: .env
    networks:
      - traefik-net
      - internal
    labels:
      - "traefik.enable=true"
      - "` + rule + `"
      - "traefik.http.services.deckplane.loadbalancer.server.port=3000"
    depends_on:
      postgres:
        condition: service_healthy
` + authentikBlock + `
volumes:
  pgdata:
` + extraVolumes + `
networks:
  traefik-net:
    external: true
    name: ` + net.NetworkName + `
  internal:
    driver: bridge
    internal: true
`
}

// InstallOpts carries every user-tunable flag for the install workflow.
// Defaults are applied by the cmd layer before we get here.
type InstallOpts struct {
	LicenseKey string
	CloudURL   string
	DataDir    string
	Port       int
	Version    string // image tag, e.g. "latest" or "v0.1.5"
	Output     io.Writer
}

// Install brings up a fresh control plane on this host. Safe to re-run:
// existing secrets in the generated .env are preserved, so `install` after
// a reboot or migration is idempotent.
func Install(opts InstallOpts) error {
	if opts.LicenseKey == "" {
		return fmt.Errorf("--license is required (copy your license JWT from Deckplane Cloud → Licenses)")
	}

	var err error
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

	// Resolve network + Authentik config before writing the compose file. Only
	// prompt on first install (compose file absent) and when stdin is a real terminal.
	composePath := filepath.Join(opts.DataDir, "docker-compose.yml")
	var netConfig *NetworkConfig
	var authentikCfg *AuthentikConfig
	if _, statErr := os.Stat(composePath); os.IsNotExist(statErr) {
		if isInteractive() {
			nets, _ := docker.ListBridgeNetworks()
			netConfig, authentikCfg, err = promptNetworkConfig(nets)
			if err != nil {
				return err
			}
			if netConfig.Mode == NetworkModeTraefik && netConfig.CreateNet {
				if err := docker.CreateNetwork(netConfig.NetworkName); err != nil {
					return fmt.Errorf("could not create network %q: %w", netConfig.NetworkName, err)
				}
				logf("[+] Created Docker network %q", netConfig.NetworkName)
			}
		} else {
			// Non-interactive: fall back to direct port binding, no SSO.
			netConfig = &NetworkConfig{Mode: NetworkModePort, Port: opts.Port}
			authentikCfg = &AuthentikConfig{Mode: AuthentikSkip}
		}
	}

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

	if err := writeComposeFile(opts.DataDir, opts.Version, netConfig, authentikCfg); err != nil {
		return err
	}

	envPath := filepath.Join(opts.DataDir, ".env")
	envValues, created, err := ensureEnvFile(envPath, opts.LicenseKey, opts.Port, authentikCfg)
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

	healthPort := opts.Port
	if netConfig != nil && netConfig.Mode == NetworkModeTraefik {
		healthPort = 0 // no localhost port binding in Traefik mode
	}
	if err := waitForHealth(healthPort, out); err != nil {
		return err
	}

	baseURL := fmt.Sprintf("http://localhost:%d", opts.Port)
	if netConfig != nil && netConfig.Mode == NetworkModeTraefik {
		baseURL = "https://" + netConfig.Host
	}

	logf("\nDeckplane is running at %s", baseURL)
	logf("")
	logf("Next steps:")
	logf("  1. Create the first admin account:")
	logf("       curl -X POST %s/api/v1/setup/initialize \\", baseURL)
	logf("         -H 'Content-Type: application/json' \\")
	logf("         -d '{\"email\":\"admin@example.com\",\"password\":\"...\",\"fullName\":\"Admin\"}'")
	logf("  2. Register a host agent with the bootstrap token below.")
	logf("")
	logf("AGENT_BOOTSTRAP_TOKEN: %s", envValues["AGENT_BOOTSTRAP_TOKEN"])
	logf("(store this — you'll paste it into `deckplane agent install --token ...`)")

	if authentikCfg != nil {
		switch authentikCfg.Mode {
		case AuthentikInstall:
			logf("")
			logf("Authentik SSO:")
			logf("  Authentik is starting at http://localhost:9000 (or https://auth.%s via Traefik)", func() string {
				if netConfig != nil && netConfig.Mode == NetworkModeTraefik {
					return netConfig.Host
				}
				return "your-domain"
			}())
			logf("  To enable SSO after Authentik is configured:")
			logf("    1. In Authentik, create an OAuth2/OpenID provider for Deckplane.")
			logf("    2. Add to %s/.env:", opts.DataDir)
			logf("         AUTHENTIK_ENABLED=true")
			logf("         AUTHENTIK_ISSUER=https://<auth-host>/application/o/<slug>/")
			logf("         AUTHENTIK_CLIENT_ID=<from Authentik>")
			logf("         AUTHENTIK_CLIENT_SECRET=<from Authentik>")
			logf("         AUTHENTIK_REDIRECT_URI=%s/api/v1/auth/authentik/callback", baseURL)
			logf("         AUTHENTIK_POST_LOGIN_REDIRECT=%s", baseURL)
			logf("    3. Run: docker compose -f %s/docker-compose.yml restart control", opts.DataDir)
		case AuthentikExisting:
			logf("")
			logf("Authentik SSO:")
			logf("  To complete SSO setup:")
			logf("    1. In Authentik, create an OAuth2/OpenID provider for Deckplane.")
			logf("    2. Update %s/.env with the client credentials:", opts.DataDir)
			logf("         AUTHENTIK_CLIENT_ID=<from Authentik>")
			logf("         AUTHENTIK_CLIENT_SECRET=<from Authentik>")
			logf("    3. Run: docker compose -f %s/docker-compose.yml restart control", opts.DataDir)
		}
	}
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

// SetLicenseOpts replaces the LICENSE_KEY in an existing install's .env.
type SetLicenseOpts struct {
	LicenseKey string
	DataDir    string
	Output     io.Writer
}

// SetLicense rotates the stored license key. Validates by minting a registry
// token against Cloud before persisting — a bad license is rejected up front
// rather than after restart. Does not touch any other secret.
func SetLicense(opts SetLicenseOpts) error {
	envPath := filepath.Join(opts.DataDir, ".env")
	values, err := readEnvFile(envPath)
	if err != nil {
		return fmt.Errorf("could not read %s — is this host installed? run `deckplane server install` first\n  (%w)", envPath, err)
	}

	// Validate before writing so we never persist a license cloud rejects.
	if _, err := cloud.New(values["LICENSE_CLOUD_URL"]).MintRegistryToken(opts.LicenseKey); err != nil {
		return fmt.Errorf("license rejected: %w", err)
	}

	values["LICENSE_KEY"] = opts.LicenseKey
	if err := writeEnvFile(envPath, values); err != nil {
		return err
	}

	fmt.Fprintln(opts.Output, "[+] License updated in .env")
	fmt.Fprintln(opts.Output, "    Run `deckplane server update` to pull images with the new license")
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

func writeComposeFile(dir, version string, net *NetworkConfig, auth *AuthentikConfig) error {
	path := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(composeTemplateFor(version, net, auth)), 0o640)
}

func ensureEnvFile(path, licenseKey string, port int, auth *AuthentikConfig) (map[string]string, bool, error) {
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

	if auth != nil {
		switch auth.Mode {
		case AuthentikInstall:
			authSecret, err := randomHex(32)
			if err != nil {
				return nil, false, err
			}
			authDBPass, err := randomHex(16)
			if err != nil {
				return nil, false, err
			}
			values["AUTHENTIK_SECRET_KEY"] = authSecret
			values["AUTHENTIK_DB_PASSWORD"] = authDBPass
			// Placeholders — filled in after the user configures the OAuth2 provider.
			values["AUTHENTIK_ENABLED"] = "false"
			values["AUTHENTIK_ISSUER"] = ""
			values["AUTHENTIK_CLIENT_ID"] = ""
			values["AUTHENTIK_CLIENT_SECRET"] = ""
			values["AUTHENTIK_REDIRECT_URI"] = ""
			values["AUTHENTIK_POST_LOGIN_REDIRECT"] = ""
		case AuthentikExisting:
			values["AUTHENTIK_ENABLED"] = "false"
			values["AUTHENTIK_ISSUER"] = auth.ExistingURL + "/application/o/deckplane/"
			values["AUTHENTIK_CLIENT_ID"] = ""
			values["AUTHENTIK_CLIENT_SECRET"] = ""
			values["AUTHENTIK_REDIRECT_URI"] = ""
			values["AUTHENTIK_POST_LOGIN_REDIRECT"] = ""
		}
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
		"AUTHENTIK_ENABLED",
		"AUTHENTIK_ISSUER", "AUTHENTIK_CLIENT_ID", "AUTHENTIK_CLIENT_SECRET",
		"AUTHENTIK_REDIRECT_URI", "AUTHENTIK_POST_LOGIN_REDIRECT",
		"AUTHENTIK_SECRET_KEY", "AUTHENTIK_DB_PASSWORD",
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

func authentikPortServices() string {
	return `
  authentik-db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: authentik
      POSTGRES_PASSWORD: ${AUTHENTIK_DB_PASSWORD}
      POSTGRES_DB: authentik
    volumes:
      - authentik-pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U authentik"]
      interval: 5s
      timeout: 5s
      retries: 10

  authentik-redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: --save 60 1 --loglevel warning
    volumes:
      - authentik-redis:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep PONG"]
      interval: 5s
      timeout: 5s
      retries: 5

  authentik-server:
    image: ghcr.io/goauthentik/server:2024.12
    restart: unless-stopped
    command: server
    environment:
      AUTHENTIK_SECRET_KEY: ${AUTHENTIK_SECRET_KEY}
      AUTHENTIK_POSTGRESQL__HOST: authentik-db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${AUTHENTIK_DB_PASSWORD}
      AUTHENTIK_POSTGRESQL__NAME: authentik
      AUTHENTIK_REDIS__HOST: authentik-redis
      AUTHENTIK_ERROR_REPORTING__ENABLED: "false"
    ports:
      - "9000:9000"
      - "9443:9443"
    depends_on:
      authentik-db:
        condition: service_healthy
      authentik-redis:
        condition: service_healthy

  authentik-worker:
    image: ghcr.io/goauthentik/server:2024.12
    restart: unless-stopped
    command: worker
    user: root
    environment:
      AUTHENTIK_SECRET_KEY: ${AUTHENTIK_SECRET_KEY}
      AUTHENTIK_POSTGRESQL__HOST: authentik-db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${AUTHENTIK_DB_PASSWORD}
      AUTHENTIK_POSTGRESQL__NAME: authentik
      AUTHENTIK_REDIS__HOST: authentik-redis
      AUTHENTIK_ERROR_REPORTING__ENABLED: "false"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    depends_on:
      authentik-db:
        condition: service_healthy
      authentik-redis:
        condition: service_healthy
`
}

func authentikTraefikServices(traefikNet, authHost string) string {
	rule := "traefik.http.routers.authentik.rule=Host(`" + authHost + "`)"
	return `
  authentik-db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: authentik
      POSTGRES_PASSWORD: ${AUTHENTIK_DB_PASSWORD}
      POSTGRES_DB: authentik
    volumes:
      - authentik-pgdata:/var/lib/postgresql/data
    networks:
      - internal
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U authentik"]
      interval: 5s
      timeout: 5s
      retries: 10

  authentik-redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: --save 60 1 --loglevel warning
    volumes:
      - authentik-redis:/data
    networks:
      - internal
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep PONG"]
      interval: 5s
      timeout: 5s
      retries: 5

  authentik-server:
    image: ghcr.io/goauthentik/server:2024.12
    restart: unless-stopped
    command: server
    environment:
      AUTHENTIK_SECRET_KEY: ${AUTHENTIK_SECRET_KEY}
      AUTHENTIK_POSTGRESQL__HOST: authentik-db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${AUTHENTIK_DB_PASSWORD}
      AUTHENTIK_POSTGRESQL__NAME: authentik
      AUTHENTIK_REDIS__HOST: authentik-redis
      AUTHENTIK_ERROR_REPORTING__ENABLED: "false"
    networks:
      - traefik-net
      - internal
    labels:
      - "traefik.enable=true"
      - "` + rule + `"
      - "traefik.http.services.authentik.loadbalancer.server.port=9000"
    depends_on:
      authentik-db:
        condition: service_healthy
      authentik-redis:
        condition: service_healthy

  authentik-worker:
    image: ghcr.io/goauthentik/server:2024.12
    restart: unless-stopped
    command: worker
    user: root
    environment:
      AUTHENTIK_SECRET_KEY: ${AUTHENTIK_SECRET_KEY}
      AUTHENTIK_POSTGRESQL__HOST: authentik-db
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: ${AUTHENTIK_DB_PASSWORD}
      AUTHENTIK_POSTGRESQL__NAME: authentik
      AUTHENTIK_REDIS__HOST: authentik-redis
      AUTHENTIK_ERROR_REPORTING__ENABLED: "false"
    networks:
      - internal
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    depends_on:
      authentik-db:
        condition: service_healthy
      authentik-redis:
        condition: service_healthy
`
}

func waitForHealth(port int, out io.Writer) error {
	if port <= 0 {
		fmt.Fprintln(out, "[+] Control plane started (health check skipped — no direct port binding)")
		return nil
	}
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
