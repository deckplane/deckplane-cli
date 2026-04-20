package docker

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const (
	// RegistryHost is the container registry Deckplane images live in. The
	// Cloud mints short-lived pull tokens via a GitHub App; end users
	// never hold persistent GHCR credentials.
	RegistryHost = "ghcr.io"

	// RegistryUsername is the fixed username for GitHub App installation
	// access tokens. GitHub expects this exact value when the password is
	// an installation token.
	RegistryUsername = "x-access-token"
)

// CheckInstalled verifies that Docker is available and running.
func CheckInstalled() error {
	return exec.Command("docker", "version").Run()
}

// CheckComposeInstalled verifies that Docker Compose is available.
func CheckComposeInstalled() error {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		return exec.Command("docker-compose", "version").Run()
	}
	return nil
}

// Login authenticates with the given registry using password-stdin.
func Login(host, username, password string) error {
	cmd := exec.Command("docker", "login", host, "-u", username, "--password-stdin")
	cmd.Stdin = strings.NewReader(password)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("registry login failed: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Logout removes stored credentials for the given registry.
func Logout(host string) {
	_ = exec.Command("docker", "logout", host).Run()
}

// Pull downloads an image from the registry.
func Pull(image string) error {
	cmd := exec.Command("docker", "pull", image)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// RemoveContainer force-removes a container by name. Errors are ignored
// because the container may not exist.
func RemoveContainer(name string) {
	_ = exec.Command("docker", "rm", "-f", name).Run()
}

// RunContainer starts a detached container with the given configuration.
func RunContainer(name, image string, volumes, envs []string) error {
	args := []string{"run", "-d", "--name", name, "--restart", "unless-stopped"}

	for _, v := range volumes {
		args = append(args, "-v", v)
	}
	for _, e := range envs {
		args = append(args, "-e", e)
	}
	args = append(args, image)

	var stderr bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container start failed: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

// IsContainerRunning checks whether a container is in running state.
func IsContainerRunning(name string) bool {
	out, err := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", name).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// ContainerLogs returns the last n lines of a container's logs.
func ContainerLogs(name string, tail int) (string, error) {
	out, err := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", tail), name).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// composeArgs returns the correct invocation for the installed Compose.
// Modern Docker ships `docker compose`; legacy installs ship
// `docker-compose`. We sniff once and cache the result.
var composeBin = []string{"docker", "compose"}

// Compose runs `docker compose` (or `docker-compose` on older systems) in
// the given directory with the supplied subcommand and args.
func Compose(dir string, subcommand string, args ...string) error {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		composeBin = []string{"docker-compose"}
	}

	full := append([]string{}, composeBin[1:]...)
	full = append(full, subcommand)
	full = append(full, args...)

	cmd := exec.Command(composeBin[0], full...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s failed: %s", subcommand, strings.TrimSpace(stderr.String()))
	}
	return nil
}
