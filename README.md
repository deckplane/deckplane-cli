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
| `init`  | Initialize a new Deckplane project |
| `version` | Print the CLI version |
| `help` | Show help for a command |
| `completion` | Generate shell completion script |

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
