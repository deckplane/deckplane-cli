package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/deckplane/deckplane-cli/internal/docker"
)

const (
	agentImage    = "ghcr.io/deckplane/deckplane-agent:latest"
	containerName = "deckplane-agent"
)

// BootstrapResponse holds the response from the Control Plane bootstrap endpoint.
type BootstrapResponse struct {
	RegistryToken string `json:"registry_token"`
	ServerURL     string `json:"server_url"`
	AgentToken    string `json:"agent_token"`
}

// InstallOpts contains all options for the agent install operation.
type InstallOpts struct {
	ServerURL string
	Token     string
	Name      string
	DataDir   string
	Output    io.Writer
}

// Install performs the full agent installation workflow.
func Install(opts InstallOpts) error {
	out := opts.Output

	// Step 1: Verify Docker is installed
	if err := docker.CheckInstalled(); err != nil {
		return fmt.Errorf("docker is not installed or not running\n  Install Docker: https://docs.docker.com/get-docker/")
	}
	fmt.Fprintln(out, "[+] Docker is installed")

	// Step 2: Verify Docker Compose is installed
	if err := docker.CheckComposeInstalled(); err != nil {
		return fmt.Errorf("docker compose is not installed\n  Install Docker Compose: https://docs.docker.com/compose/install/")
	}
	fmt.Fprintln(out, "[+] Docker Compose is installed")

	// Step 3: Bootstrap — validate token and get configuration
	bootstrap, err := callBootstrap(opts.ServerURL, opts.Token)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "[+] Token validated")

	// Step 4: Pull agent image
	if err := pullImage(bootstrap.RegistryToken); err != nil {
		return err
	}
	fmt.Fprintln(out, "[+] Agent image pulled")

	// Step 5: Create data directory
	if err := os.MkdirAll(opts.DataDir, 0750); err != nil {
		return fmt.Errorf("failed to create data directory %s: %w", opts.DataDir, err)
	}

	// Step 6: Start agent container
	if err := startContainer(bootstrap, opts.Name, opts.DataDir); err != nil {
		return err
	}
	fmt.Fprintln(out, "[+] Agent container started")

	// Step 7: Verify Control Plane connection
	if err := verifyConnection(out); err != nil {
		return err
	}

	fmt.Fprintf(out, "\nAgent %q registered successfully.\n", opts.Name)
	return nil
}

func callBootstrap(serverURL, token string) (*BootstrapResponse, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid server URL: %s\n  URL must start with http:// or https://", serverURL)
	}

	endpoint := strings.TrimRight(serverURL, "/") + "/agents/bootstrap"

	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Control Plane: %w\n  Verify the server URL: %s", err, serverURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bootstrap response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("invalid or expired token\n  Generate a new bootstrap token from the Control Plane UI")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("bootstrap failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var bootstrap BootstrapResponse
	if err := json.Unmarshal(body, &bootstrap); err != nil {
		return nil, fmt.Errorf("failed to parse bootstrap response: %w", err)
	}

	if bootstrap.AgentToken == "" || bootstrap.RegistryToken == "" {
		return nil, fmt.Errorf("incomplete bootstrap response\n  Check your Control Plane configuration")
	}

	return &bootstrap, nil
}

func pullImage(registryToken string) error {
	if err := docker.Login(docker.RegistryHost, docker.RegistryUsername, registryToken); err != nil {
		return err
	}
	defer docker.Logout(docker.RegistryHost)

	if err := docker.Pull(agentImage); err != nil {
		return fmt.Errorf("failed to pull agent image\n  Verify registry access and network connectivity")
	}
	return nil
}

func startContainer(bootstrap *BootstrapResponse, agentName, dataDir string) error {
	docker.RemoveContainer(containerName)

	volumes := []string{
		"/var/run/docker.sock:/var/run/docker.sock",
		dataDir + ":" + dataDir,
	}
	envs := []string{
		"SERVER_URL=" + bootstrap.ServerURL,
		"AGENT_TOKEN=" + bootstrap.AgentToken,
		"AGENT_NAME=" + agentName,
	}

	return docker.RunContainer(containerName, agentImage, volumes, envs)
}

func verifyConnection(out io.Writer) error {
	const maxAttempts = 15

	for i := range maxAttempts {
		time.Sleep(2 * time.Second)

		if !docker.IsContainerRunning(containerName) {
			if i > 2 {
				break
			}
			continue
		}

		logs, err := docker.ContainerLogs(containerName, 50)
		if err == nil && strings.Contains(logs, "connected") {
			fmt.Fprintln(out, "[+] Control Plane connection established")
			return nil
		}
	}

	if docker.IsContainerRunning(containerName) {
		fmt.Fprintln(out, "[!] Agent is running but Control Plane connection could not be verified")
		fmt.Fprintln(out, "    Check logs: docker logs deckplane-agent")
		return nil
	}

	return fmt.Errorf("agent failed to start or connect to Control Plane\n  Check logs: docker logs %s", containerName)
}
