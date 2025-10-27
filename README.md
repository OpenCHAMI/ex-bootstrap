# ochami-ex-bootstrap

A small CLI tool to discover bootable node NICs via BMC Redfish and produce a YAML inventory (`bmcs[]` and `nodes[]`).

This repository contains a lightweight Go implementation originally based on small scripts for creating an initial BMC inventory and then discovering node NICs through Redfish.

## Features

- Generate an initial `inventory.yaml` with a `bmcs` list (xname, MAC, IP) using `--init-bmcs`.
- Discover bootable NICs via Redfish on each BMC and allocate IPs from a given subnet.
- Output file format: a single YAML file with two top-level keys:
  - `bmcs`: list of management controllers (xname, mac, ip)
  - `nodes`: list of discovered node network records (xname, mac, ip)

## Layout (important files)

- `main.go` — flags, program orchestration, and discovery flow.
- `types.go` — YAML model types (`Entry`, `FileFormat`).
- `redfish.go` — minimal Redfish client and bootable NIC heuristics.
- `ipam.go` — IP allocation helpers using `github.com/metal-stack/go-ipam`.
- `xname.go` — xname helper(s) and conversions.
- `init_bmcs.go` — functions used by `--init-bmcs` mode (ported from the original Python script).
- `utils.go` — small helpers (die, findByXname).

## Build

This project uses Go modules. From the repo root:

```bash
# fetch modules and build
go mod tidy
go build -o ochami_bootstrap .
```

## Usage

Two main modes:

1) Generate an initial BMC inventory (`--init-bmcs`)

```bash
./ochami_bootstrap --init-bmcs --file inventory.yaml
```

Options (defaults):
- `--file` (default: `inventory.yaml`) — output YAML file path
- `--chassis` — comma-separated `chassis=macprefix` mappings, default: `x9000c1=02:23:28:01,x9000c3=02:23:28:03`
- `--bmc-subnet` — BMC subnet base (three-octet prefix), default: `192.168.100`
- `--nodes-per-chassis` — how many node IDs per chassis (default 32)
- `--nodes-per-bmc` — nodes handled by each BMC (default 2)

Example with custom chassis string:

```bash
./ochami_bootstrap --init-bmcs --chassis "x9000c1=02:23:28:01,x9000c5=02:23:28:05" --file inventory.yaml
```

This writes `inventory.yaml` with a `bmcs:` list and an empty `nodes: []`.

2) Discover bootable NICs and allocate IPs

The discovery flow reads the YAML `--file` (must contain non-empty `bmcs[]`) and writes back the same file with updated `nodes[]`.

Required environment variables:
- `REDFISH_USER` — Redfish username
- `REDFISH_PASSWORD` — Redfish password

Example discovery run (allocates from `--subnet`):

```bash
export REDFISH_USER=admin
export REDFISH_PASSWORD=secret
./ochami_bootstrap --file inventory.yaml --subnet 10.42.0.0/24
```

Flags relevant to discovery:
- `--subnet` (required) — CIDR to allocate node IPs from, e.g. `10.42.0.0/24`
- `--insecure` — allow insecure TLS to BMCs (default: true)
- `--timeout` — per-BMC discovery timeout (default: `12s`)

Notes:
- The program makes simple heuristic decisions about which NIC is bootable (UEFI path hints, DHCP addresses, or a MAC on an enabled interface).
- IP allocation is done with `github.com/metal-stack/go-ipam`. The code reserves `.1` (first host) as a gateway and avoids network/broadcast implicitly.

## Dependencies

- Go (module aware). The project will download dependencies with `go mod tidy`.
- `github.com/metal-stack/go-ipam` — used for IP allocation.
- `gopkg.in/yaml.v3` — YAML parsing and writing.

## Contributing / Next steps

- Add unit tests for the xname / MAC generation helpers and the Redfish parsing heuristics.
- Add input validation for chassis/macro formats if you require stricter MAC formatting.
- Consider adding a `--dry-run` mode for discovery to avoid writing changes while testing.

## License

Pick an appropriate license for your project. This repo currently has none specified.

If you'd like, I can also add a small `README` section that includes an example `inventory.yaml` and a quick test script to validate the discovery flow without real BMCs (e.g., a mock HTTP server).