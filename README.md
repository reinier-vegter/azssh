# azssh

`azssh` is a terminal UI for quickly finding Linux Azure virtual machines that
can be reached through Azure Bastion, then starting the native Azure CLI SSH
connection.

It uses Azure Resource Graph to discover compatible Bastion hosts, direct VNet
peering routes, and Linux VMs. Inventory is cached locally per Azure account so
the finder is available while refresh runs in the background.

## Requirements

- Linux on `amd64` or `arm64`
- Azure CLI with the Bastion extension available
- An authenticated Azure CLI session: `az login`
- Azure Bastion Standard or Premium hosts with native client/tunneling enabled
- Permission to read the relevant Azure resources and connect through Bastion

The default connection type is Microsoft Entra ID (`AAD`). Guest access and VM
authentication requirements remain enforced by Azure Bastion and the Azure CLI.

## Install

Download the architecture-appropriate `.gz` binary from the [latest release](https://github.com/reinier-vegter/azssh/releases/latest), then decompress and install it:

```sh
gunzip azssh_v0.1.0_linux_amd64.gz
chmod +x azssh_v0.1.0_linux_amd64
sudo install -m 0755 azssh_v0.1.0_linux_amd64 /usr/local/bin/azssh
```

Use the `linux_arm64` release asset for 64-bit ARM systems. Replace the example
version with the downloaded release version.

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
| `f` | Filter subscriptions |
| `b` | Choose a Bastion route when multiple routes are available |
| `r` | Refresh Azure inventory |
| `?` | Show help |
| `q` | Quit |

## Cache and Security

Cache files are stored under the operating system user cache directory in an
account-specific `azssh` directory. The cache contains inventory metadata and
subscription preferences only. It never stores Azure access tokens, passwords,
or SSH private keys.

## Build from Source

Use a Go toolchain matching the version in `go.mod`:

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -o azssh ./cmd/azssh
```

## Development

Pull requests and pushes are checked with unit tests, `go vet`, and a static
Linux build. Tagged releases publish gzip-compressed standalone binaries for
Linux `amd64` and `arm64`.
