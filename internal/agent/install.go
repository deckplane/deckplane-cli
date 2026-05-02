// Package agent implements the `deckplane agent install|update|uninstall`
// workflow. The agent image is public on GHCR — no registry auth needed.
// The agent itself registers with the control plane on first boot using the
// bootstrap token; the CLI just writes config, pulls, and starts.
package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deckplane/deckplane-cli/internal/docker"
)

const agentImageRepo = "ghcr.io/deckplane/deckplane-agent"

func agentImage(version string) string {
	if version == "" {
		version = "latest"
	}
	return agentImageRepo + ":" + version
}

// InstallOpts carries every user-tunable flag for the agent install workflow.
type InstallOpts struct {
	ServerURL string
	Token     string
	Name      string
	DataDir   string
	Version   string
	Output    io.Writer
}

// UpdateOpts is used to update an existing agent install.
type UpdateOpts struct {
	DataDir string
	Output  io.Writer
}

// UninstallOpts controls whether agent data is destroyed on uninstall.
type UninstallOpts struct {
	DataDir    string
	RemoveData bool
	Output     io.Writer
}

// Install deploys the Deckplane agent on this Docker host. Safe to re-run:
// existing .env is left untouched so tokens survive reinstalls.
func Install(opts InstallOpts) error {
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

	if err := checkServerReachable(opts.ServerURL); err != nil {
		return err
	}
	logf("[+] Control plane reachable at %s", opts.ServerURL)

	if err := os.MkdirAll(opts.DataDir, 0o750); err != nil {
		return fmt.Errorf("could not create %s: %w", opts.DataDir, err)
	}

	composePath := filepath.Join(opts.DataDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		if err := os.WriteFile(composePath, []byte(composeTemplate(opts.Version)), 0o640); err != nil {
			return fmt.Errorf("could not write compose file: %w", err)
		}
		logf("[+] Wrote compose file to %s", composePath)
	} else {
		logf("[+] Reusing existing compose file at %s", composePath)
	}

	envPath := filepath.Join(opts.DataDir, ".env")
	created, err := ensureEnvFile(envPath, opts.ServerURL, opts.Token, opts.Name)
	if err != nil {
		return err
	}
	if created {
		logf("[+] Generated config at %s", envPath)
	} else {
		logf("[+] Reusing existing config at %s", envPath)
	}

	if err := docker.Compose(opts.DataDir, "pull"); err != nil {
		return fmt.Errorf("image pull failed: %w", err)
	}
	logf("[+] Agent image pulled")

	// Remove any stale container with the same name so compose up doesn't conflict.
	docker.RemoveContainer("deckplane-agent")

	if err := docker.Compose(opts.DataDir, "up", "-d"); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}
	logf("[+] Agent container started")

	// Give the agent a moment to attempt first registration.
	time.Sleep(3 * time.Second)

	logf("")
	logf("Agent is running and should appear in the Deckplane UI under Agents.")
	logf("If it doesn't show up within a minute, check the logs:")
	logf("  docker logs deckplane-agent")
	return nil
}

// Update pulls a newer agent image and restarts the container without touching
// any config. Errors if the data dir wasn't produced by `install`.
func Update(opts UpdateOpts) error {
	if _, err := os.Stat(filepath.Join(opts.DataDir, "docker-compose.yml")); err != nil {
		return fmt.Errorf("no install found at %s — run `deckplane agent install` first", opts.DataDir)
	}
	if err := docker.Compose(opts.DataDir, "pull"); err != nil {
		return err
	}
	if err := docker.Compose(opts.DataDir, "up", "-d"); err != nil {
		return err
	}
	fmt.Fprintln(opts.Output, "[+] Agent updated and restarted")
	return nil
}

// Uninstall stops and removes the agent stack. By default, the named state
// volume (persists registered agent token) is preserved so the agent can
// reconnect without re-registering after a reinstall.
func Uninstall(opts UninstallOpts) error {
	args := []string{}
	if opts.RemoveData {
		args = append(args, "-v")
	}
	if err := docker.Compose(opts.DataDir, "down", args...); err != nil {
		return err
	}
	if opts.RemoveData {
		if err := os.RemoveAll(opts.DataDir); err != nil {
			return fmt.Errorf("failed to remove %s: %w", opts.DataDir, err)
		}
		fmt.Fprintln(opts.Output, "[+] Agent data removed")
	}
	fmt.Fprintln(opts.Output, "[+] Agent stopped")
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// checkServerReachable does a quick health probe so we catch a wrong URL early.
func checkServerReachable(serverURL string) error {
	parsed, err := url.Parse(serverURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid server URL %q — must start with http:// or https://", serverURL)
	}
	endpoint := strings.TrimRight(serverURL, "/") + "/health"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("control plane not reachable at %s\n  Check the URL and firewall rules", serverURL)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("control plane returned HTTP %d — is it running?", resp.StatusCode)
	}
	return nil
}

func composeTemplate(version string) string {
	return `services:
  agent:
    image: ` + agentImage(version) + `
    container_name: deckplane-agent
    restart: unless-stopped
    pid: "host"
    privileged: true
    env_file: .env
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - agent-state:/app/state
    extra_hosts:
      - "host.docker.internal:host-gateway"

volumes:
  agent-state:
`
}

func ensureEnvFile(path, serverURL, token, name string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}

	order := []string{
		"CONTROL_PLANE_URL", "BOOTSTRAP_TOKEN",
		"AGENT_HOSTNAME",
		"DOCKER_SOCKET",
		"NODE_ENV", "LOG_LEVEL",
	}
	values := map[string]string{
		"CONTROL_PLANE_URL": serverURL,
		"BOOTSTRAP_TOKEN":   token,
		"AGENT_HOSTNAME":    name,
		"DOCKER_SOCKET":     "/var/run/docker.sock",
		"NODE_ENV":          "production",
		"LOG_LEVEL":         "info",
	}

	var b strings.Builder
	seen := map[string]bool{}
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

	if err := os.WriteFile(path, []byte(b.String()), 0o640); err != nil {
		return false, fmt.Errorf("could not write %s: %w", path, err)
	}
	return true, nil
}
