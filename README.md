# Deckplane CLI

A command-line interface for managing Deckplane resources.

## Installation

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
| `init` | Initialize a new Deckplane project |
| `agent` | Manage DeckPlane agents |
| `version` | Print the CLI version |
| `help` | Show help for a command |
| `completion` | Generate shell completion script |

### Agent Commands

| Command | Description |
|---------|-------------|
| `agent install` | Install DeckPlane agent on the current Docker host |

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
# Initialize a new project
deckplane init my-project

# Initialize with a specific template
deckplane init my-project --template api

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

## License

This project is licensed under the Apache License 2.0 — see the [LICENSE](LICENSE) file for details.
