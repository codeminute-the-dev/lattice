# Lattice DNS Seeder

A Go language DNS seeder for the Lattice network, adapted from the generic Bitcoin DNS seeder.

> **Note:** This codebase was originally cloned from [gombadi/dnsseeder](https://github.com/gombadi/dnsseeder) and has been modified to support the Lattice protocol.

## Overview

This program crawls the Lattice network to find active nodes and serves their IP addresses via DNS. When a new Lattice node starts up, it queries this DNS seeder to find peers to connect to.

## Features

- **Lattice Protocol Support:** Adapted to speak the Lattice wire protocol.
- **Smart Crawling:** Cycles through nodes to keep the list fresh and verifies they are active.
- **DNS Server:** Runs a custom DNS server to answer `A` and `AAAA` queries.
- **Web Interface:** Provides a status page to monitor the crawler and seeder health.
- **Low Resource Usage:** Efficiently handles network crawling and DNS serving.

## Installation

Run from the root of the repo:

```bash
cd dnsseeder
go build .
```

## Usage

### Command Line Arguments

```
Usage: ./dnsseeder [flags]

Flags:
  -host string
    	DNS hostname to serve (e.g. seed.lattice.org)
  -p string
    	DNS Port to listen on (default "8053")
  -w string
    	Web Port to listen on (e.g. "8080"). If empty, web server is disabled.
  -testnet
    	Use TestNet parameters
  -i string
    	Comma separated list of initial IPs to crawl
  -v	Display verbose output
  -d	Display debug output
  -s	Display stats output
```

### Example

To run a seeder for the Lattice Testnet:

```bash
sudo ./dnsseeder \
  -host seeder.testnet.lattice.org \
  -testnet \
  -p 53 \
  -w 8080 \
  -i "1.2.3.4,5.6.7.8"
```

## Deployment

A deployment script for Google Compute Engine (GCE) is available in `scripts/dns-seeder-deploy-gce.sh`.

```bash
./scripts/dns-seeder-deploy-gce.sh \
  --project <project-id> \
  --zone <zone> \
  --instance-name <name> \
  --host <dns-hostname> \
  --testnet \
  --initial-nodes "<ip1>,<ip2>" \
  --open-firewall
```

## License

This project is licensed under the Apache 2.0 License.

- Original code: [gombadi/dnsseeder](https://github.com/gombadi/dnsseeder)
- DNS library: [github.com/miekg/dns](https://github.com/miekg/dns)
- Lattice node library: [github.com/codeminute-the-dev/lattice/node](https://github.com/codeminute-the-dev/lattice/node)
