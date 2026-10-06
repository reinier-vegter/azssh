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
- Bash and OpenSSH `scp` for file transfer
- SSHFS and its user-mount runtime for directory mounts: FUSE 3 on Linux, or
  macFUSE/FUSE-T on macOS
- Permission to read the relevant Azure resources and connect through Bastion

The default connection type is Microsoft Entra ID (`AAD`). Guest access and VM
authentication requirements remain enforced by Azure Bastion and the Azure CLI.

### Install SSHFS

Install SSHFS only when using the `m` directory-mount action:

```sh
# Ubuntu
sudo apt update && sudo apt install sshfs

# Fedora
sudo dnf install fuse-sshfs

# macOS: install the FUSE runtime, then SSHFS
brew install --cask macfuse
open https://github.com/libfuse/sshfs/releases/latest
```

On macOS, download and run the SSHFS `.pkg` from the release page after
installing macFUSE. Approve macFUSE in System Settings if prompted. On Linux,
confirm that your user can access `/dev/fuse` before mounting.

## Installation

Install a standalone binary from the [latest release](https://github.com/reinier-vegter/azssh/releases/latest) into `/usr/local/bin`. Choose the archive for your operating system and architecture, then run its commands from the download directory. Only the final installation requires sudo; run azssh normally without sudo.

### Linux

**Intel/AMD 64-bit (`x86_64`):** download [azssh_v0.1.0_linux_amd64.gz](https://github.com/reinier-vegter/azssh/releases/download/v0.1.0/azssh_v0.1.0_linux_amd64.gz).

```sh
<<<<<<< HEAD

mkdir -p "$HOME/.local/bin"
install -m 0755 azssh_v0.1.1_<os>_<arch> "$HOME/.local/bin/azssh"
export PATH="$HOME/.local/bin:$PATH"
=======
gunzip azssh_v0.1.0_linux_amd64.gz
sudo mkdir -p /usr/local/bin
sudo install -m 0755 azssh_v0.1.0_linux_amd64 /usr/local/bin/azssh
>>>>>>> f9c3f7e (updater + readme)
```

**ARM 64-bit (`aarch64` / `arm64`):** download [azssh_v0.1.0_linux_arm64.gz](https://github.com/reinier-vegter/azssh/releases/download/v0.1.0/azssh_v0.1.0_linux_arm64.gz).

```sh
<<<<<<< HEAD
gunzip azssh_v0.1.1_<os>_<arch>.gz
chmod +x azssh_v0.1.1_<os>_<arch>
sudo install -m 0755 azssh_v0.1.1_<os>_<arch> /usr/local/bin/azssh
=======
gunzip azssh_v0.1.0_linux_arm64.gz
sudo mkdir -p /usr/local/bin
sudo install -m 0755 azssh_v0.1.0_linux_arm64 /usr/local/bin/azssh
>>>>>>> f9c3f7e (updater + readme)
```

### macOS (Darwin)

**Intel Mac:** download [azssh_v0.1.0_darwin_amd64.gz](https://github.com/reinier-vegter/azssh/releases/download/v0.1.0/azssh_v0.1.0_darwin_amd64.gz).

```sh
gunzip azssh_v0.1.0_darwin_amd64.gz
sudo mkdir -p /usr/local/bin
sudo install -m 0755 azssh_v0.1.0_darwin_amd64 /usr/local/bin/azssh
```

**Apple Silicon Mac (M-series):** download [azssh_v0.1.0_darwin_arm64.gz](https://github.com/reinier-vegter/azssh/releases/download/v0.1.0/azssh_v0.1.0_darwin_arm64.gz).

```sh
gunzip azssh_v0.1.0_darwin_arm64.gz
sudo mkdir -p /usr/local/bin
sudo install -m 0755 azssh_v0.1.0_darwin_arm64 /usr/local/bin/azssh
```

After installing, verify the shell resolves the intended binary:

```sh
command -v azssh
azssh --version
```

The command should resolve to `/usr/local/bin/azssh` and report `v0.1.0`. If it does not, put `/usr/local/bin` first on PATH, persist that setting in the appropriate shell startup file, run `hash -r` in Bash, and check again.

### Update

When a newer release is available, press `U` outside text input, review the
in-place `/usr/local/bin/azssh` destination, and confirm. A protected standalone
installation may prompt for administrator authorization through the system
terminal; azssh never collects passwords. Restart azssh after a successful
update. Unsupported installations receive manual installation guidance.

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
| `t` | Open an Entra-only `scp` transfer shell for the selected VM |
| `m` | Mount a remote directory with Entra SSHFS |
| `d` | Toggle readable network names / full resource IDs |
| `pgup`, `pgdown` | Scroll VM details without changing the selected VM |
| `r` | Refresh Azure inventory |
| `U` | Update azssh when a newer release is available |
| `?` | Show help |
| `q` | Quit |

Shortcut hints use `key: action`, preserving actual casing (`U` is uppercase).
The finder adapts to terminal size with aligned side-by-side or stacked panels;
long details scroll while controls stay visible. Network names are shown by
default, with parent context for ambiguous names. Press `d` to inspect full IDs.
Changing VM selection resets detail expansion and scrolling; Help preserves them.
Printable shortcut keys remain text while editing search or the remote path.

## Cache and Security

Cache files are stored under the operating system user cache directory in an
account-specific `azssh` directory. The cache contains inventory metadata and
subscription and favorite preferences only. It never stores Azure access tokens, passwords,
or SSH private keys.

File transfer is available only with the default Entra ID (`AAD`) authentication.
Press `t` to open a temporary Bash shell after choosing the VM and Bastion route.
It accepts exactly one remote endpoint using the selected VM name, for example:

```sh
scp ./report.csv api-01:/home/reinier/
scp api-01:/var/log/app.log .
```

The transfer session creates an ephemeral key, Entra certificate, loopback
Bastion tunnel, and Bash rc file, then removes them when the shell exits. It
never modifies `~/.ssh` or any SSH config. Host-key checking is deliberately
disabled for this temporary loopback session, matching Azure Bastion native
client behavior.

Directory mounts are also available only with Entra ID. Press `m`, choose a
remote directory (default `.` for the remote home), and azssh mounts it at
`~/azssh/mnt/<vm-name>`. SSHFS remains in the foreground until it exits or you
press Ctrl-C; azssh then unmounts before removing its temporary identity and
tunnel. It prints the exact mountpoint as SSHFS starts, without claiming mount
readiness. The empty mountpoint
directory remains for later use with mode `0500` to discourage accidental local
writes while idle. azssh temporarily enables owner write access (`0700`) for
FUSE mounting and restores `0500` after the attempt ends and the path is confirmed
unmounted; this does not make the remote filesystem read-only. Cleanup failures
are reported. A crash or uncatchable termination may leave the local directory
writable until a later mount attempt prepares it again.
azssh never installs SSHFS or FUSE and never uses `sudo`; install the platform runtime
through your normal system process first. The mount uses `-F /dev/null` and
explicit temporary SSH options, so it does not read or modify `~/.ssh`.
On Linux, install the distribution's `sshfs` package and ensure the user can
access `/dev/fuse`. On macOS, install SSHFS together with macFUSE or FUSE-T and
complete any one-time system approval required by that runtime.

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
