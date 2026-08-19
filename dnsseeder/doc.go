/*
This application provides a DNS seeder service for the Lattice network.

This application crawls the Lattice network for active clients and records their ip address and port. It then replies to DNS queries with this information.

Features:
- Preconfigured support for the Lattice network. Use -net <network> to load config data.
- supports ipv4 & ipv6 addresses
- revisits clients on a configurable time basis to make sure they are still available
- Low memory & cpu requirements
*/
package main
