# Deckplane CLI

A command-line interface for managing Deckplane resources.

## Installation

### The Easy Way (Linux & macOS)

You can install the latest release directly using our install script:

```bash
curl -sL https://raw.githubusercontent.com/deckplane/deckplane-cli/main/install.sh | bash
```

### The Easy Way (Windows)

You can install the latest release on Windows using our PowerShell script:

```powershell
irm https://raw.githubusercontent.com/deckplane/deckplane-cli/main/install.ps1 | iex
```

### With Go

If you have Go installed, you can build and install it via:

```bash
go install github.com/deckplane/deckplane-cli@latest
```

## Usage

```bash
deckplane <command> [flags]
```

### Available Commands

| Command | Description |
|---------|-------------|
| `server` | Install, update, and manage the Deckplane control plane on this host |
| `agent` | Manage Deckplane agents |
| `version` | Print the CLI version |
| `help` | Show help for a command |
| `completion` | Generate shell completion script |

### Server Commands

| Command | Description |
|---------|-------------|
| `server install` | Install the Deckplane control plane (requires a license JWT) |
| `server update` | Pull the latest control plane image and restart |
| `server uninstall` | Stop the control plane (pass `--remove-data` to also drop the postgres volume) |

### Agent Commands

| Command | Description |
|---------|-------------|
| `agent install` | Install Deckplane agent on the current Docker host |

#### `agent install` Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--server-url` | | **(required)** Control Plane URL |
| `--token` | `-t` | **(required)** Bootstrap token from Control Plane UI |
| `--name` | `-n` | Agent name (default: hostname) |
| `--data-dir` | `-d` | Agent data directory (default: `/opt/deckplane-agent`) |

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable verbose output |

### Examples

```bash
# Install the control plane on a fresh host
deckplane server install --license eyJhbGc...

# Pull a newer image and restart later
deckplane server update

# Install agent on a Docker host
deckplane agent install \
  --server-url https://deckplane.company.com \
  --token eyJhbGc... \
  --name web-server-01

# Print version
deckplane version

# Generate shell completion (bash/zsh/fish/powershell)
deckplane completion bash
```

## Development

### Build

```bash
go build -o deckplane.exe .
```

### Run

```bash
./deckplane --help
```

## Powered By

🚀 This CLI tool is proudly developed and maintained using **[Kommando](https://github.com/yigit433/kommando)**, a powerful framework for building efficient command-line applications.

## License

This project is licensed under the Apache License 2.0 — see the [LICENSE](LICENSE) file for details.
