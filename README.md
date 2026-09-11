# azssh

[![CI](https://github.com/reinier-vegter/azssh/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/reinier-vegter/azssh/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/reinier-vegter/azssh)](https://github.com/reinier-vegter/azssh/releases/latest)
[![Coverage](https://codecov.io/gh/reinier-vegter/azssh/graph/badge.svg)](https://app.codecov.io/gh/reinier-vegter/azssh)
[![License](https://img.shields.io/github/license/reinier-vegter/azssh)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/reinier-vegter/azssh)](go.mod)
[![Platforms](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20(amd64%2C%20arm64)-1f6feb)](https://github.com/reinier-vegter/azssh/releases/latest)
[![AI assisted](https://img.shields.io/badge/development-AI--assisted-6b4fbb)](#development)

`azssh` is a terminal UI for quickly finding Linux Azure virtual machines that
can be reached through Azure Bastion, then starting the native Azure CLI SSH
connection.

It uses Azure Resource Graph to discover compatible Bastion hosts, direct VNet
peering routes, and Linux VMs. Inventory is cached locally per Azure account so
the finder is available while refresh runs in the background.

## Requirements

- Linux or macOS on `amd64` or `arm64`
- Azure CLI (`az`) installed
- az extensions `bastion` and `ssh`:
```sh
az extension add --name bastion
az extension add --name ssh
```
- An authenticated Azure CLI session: `az login`
- Azure Bastion Standard or Premium hosts with native client/tunneling enabled
- Permission to read the relevant Azure resources and connect through Bastion

The default connection type is Microsoft Entra ID (`AAD`). Guest access and VM
authentication requirements remain enforced by Azure Bastion and the Azure CLI.

## Install

Download the OS- and architecture-appropriate `.gz` binary from the [latest release](https://github.com/reinier-vegter/azssh/releases/latest), then decompress and install it:

```sh
gunzip azssh_v0.0.5_<os>_<arch>.gz
chmod +x azssh_v0.0.5_<os>_<arch>
sudo install -m 0755 azssh_v0.0.5_<os>_<arch> /usr/local/bin/azssh
```

Use `linux_amd64` or `linux_arm64` on Linux. Use `darwin_amd64` on Intel Macs
or `darwin_arm64` on Apple silicon. Replace the placeholders with the downloaded
release version, OS, and architecture.

## Usage

```sh
azssh
```

Connection options:

```sh
azssh --auth-type AAD
azssh --auth-type ssh-key --username azureuser --ssh-key ~/.ssh/id_ed25519
azssh --version
```

Key bindings:

| Key | Action |
| --- | --- |
| `enter` | Connect to the selected VM, or choose its Bastion route |
| `shift+enter` | Exit the TUI and review the exact command before running it |
| `/` | Search VMs, subscriptions, resource groups, and Bastions |
| `x` | Toggle the selected VM as a favorite; favorites appear first |
| `f` | Filter subscriptions |
| `b` | Choose a Bastion route when multiple routes are available |
| `r` | Refresh Azure inventory |
| `?` | Show help |
| `q` | Quit |

## Cache and Security

Cache files are stored under the operating system user cache directory in an
account-specific `azssh` directory. The cache contains inventory metadata and
subscription and favorite preferences only. It never stores Azure access tokens, passwords,
or SSH private keys.

## Build from Source

Use a Go toolchain matching the version in `go.mod`:

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -o azssh ./cmd/azssh
./azssh
```

## Development

Pull requests and pushes are checked with unit tests, `go vet`, and a static
Linux build. Tagged releases publish gzip-compressed standalone binaries for
Linux and macOS on `amd64` and `arm64`.

This project is AI-assisted. Maintainers review, test, and take responsibility
for all changes.

## License

Released under the [MIT License](LICENSE).
