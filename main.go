package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------- YAML model (single file, read and write) ----------
// The file has two sections:
//   bmcs:  list of management controllers we query (required)
//   nodes: list of discovered bootable node NICs (this tool overwrites/updates)
//
// Both sections use the same 3 fields: xname, mac, ip (as you requested)

// ---------- Redfish minimal client ----------

// ---------- Xname helpers ----------

// ---------- Initial BMC inventory generation (ported from Python) ----------

// ---------- IPAM setup (github.com/metal-stack/go-ipam) ----------

// ---------- Main ----------

func main() {
	filePath := flag.String("file", "inventory.yaml", "YAML file containing bmcs[] and nodes[] (nodes will be overwritten)")
	initBmcs := flag.Bool("init-bmcs", false, "generate initial inventory with bmcs and exit")
	chassisSpec := flag.String("chassis", "x9000c1=02:23:28:01,x9000c3=02:23:28:03", "comma-separated chassis=macprefix list")
	bmcSubnetBase := flag.String("bmc-subnet", "192.168.100", "BMC subnet base without last octet, e.g. 192.168.100")
	nodesPerChassis := flag.Int("nodes-per-chassis", 32, "number of nodes per chassis")
	nodesPerBMC := flag.Int("nodes-per-bmc", 2, "number of nodes managed by each BMC")
	startNID := flag.Int("start-nid", 1, "starting node id (1-based)")
	subnet := flag.String("subnet", "", "CIDR to allocate from, e.g. 10.42.0.0/24")
	insecure := flag.Bool("insecure", true, "allow insecure TLS to BMCs")
	timeout := flag.Duration("timeout", 12*time.Second, "per-BMC discovery timeout")
	flag.Parse()

	// If requested, generate initial BMC inventory and exit.
	if *initBmcs {
		chassis := parseChassisSpec(*chassisSpec)
		if len(chassis) == 0 {
			die("ERROR: --chassis must specify at least one entry, e.g. x9000c1=02:23:28:01")
		}
		if strings.Count(*bmcSubnetBase, ".") != 2 {
			die("ERROR: --bmc-subnet must be a base with three octets, e.g. 192.168.100")
		}

		var bmcs []Entry
		nid := *startNID
		for c, macPref := range chassis {
			// Walk nodes in steps of nodes-per-bmc, placing one BMC per pair
			for i := nid; i < nid+*nodesPerChassis; i += *nodesPerBMC {
				x := getNCXname(c, i)
				ip := fmt.Sprintf("%s.%d", *bmcSubnetBase, getBmcID(i))
				mac := strings.ToLower(getNCMAC(macPref, i))
				bmcs = append(bmcs, Entry{Xname: x, MAC: mac, IP: ip})
			}
			nid = nid + *nodesPerChassis
		}

		doc := FileFormat{BMCs: bmcs, Nodes: nil}
		bytes, err := yaml.Marshal(&doc)
		if err != nil {
			die(fmt.Sprintf("yaml marshal: %v", err))
		}
		if err := os.WriteFile(*filePath, bytes, 0o644); err != nil {
			die(fmt.Sprintf("write %s: %v", *filePath, err))
		}
		fmt.Printf("Wrote initial BMC inventory to %s with %d entries\n", *filePath, len(bmcs))
		return
	}

	if *subnet == "" {
		die("ERROR: --subnet is required, e.g. 10.42.0.0/24")
	}

	user := os.Getenv("REDFISH_USER")
	pass := os.Getenv("REDFISH_PASSWORD")
	if user == "" || pass == "" {
		die("ERROR: REDFISH_USER and REDFISH_PASSWORD env vars are required")
	}

	// Load YAML
	raw, err := os.ReadFile(*filePath)
	if err != nil {
		die(fmt.Sprintf("read %s: %v", *filePath, err))
	}
	var doc FileFormat
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		die(fmt.Sprintf("parse %s: %v", *filePath, err))
	}
	if len(doc.BMCs) == 0 {
		die("input must contain non-empty bmcs[]")
	}

	// Set up IPAM and pre-reserve any existing node IPs (idempotent re-runs)
	alloc, err := newAllocator(*subnet)
	if err != nil {
		die(fmt.Sprintf("ipam init: %v", err))
	}
	for _, n := range doc.Nodes {
		if ip := net.ParseIP(n.IP); ip != nil {
			alloc.reserve(ip.String())
		}
	}

	// Discover bootable NICs from each BMC and assign IPs
	var out []Entry

	for _, b := range doc.BMCs {
		host := b.IP
		if host == "" {
			// if IP missing, allow FQDN in MAC field? No—use xname as hostname if it looks like one
			host = b.Xname
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		client := newRFClient(host, user, pass, *insecure, *timeout)

		sysPath, err := client.firstSystemPath(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: %s: systems: %v\n", b.Xname, err)
			cancel()
			continue
		}
		nics, err := client.listEthernetInterfaces(ctx, sysPath)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: %s: ethernet: %v\n", b.Xname, err)
			continue
		}

		bootable := make([]rfEthernetInterface, 0, len(nics))
		for _, nic := range nics {
			if nic.MACAddress == "" {
				continue
			}
			if isBootable(nic) {
				bootable = append(bootable, nic)
			}
		}
		// Fallback: take first NIC with MAC if heuristics found none
		if len(bootable) == 0 {
			for _, nic := range nics {
				if nic.MACAddress != "" {
					bootable = append(bootable, nic)
					break
				}
			}
		}
		if len(bootable) == 0 {
			fmt.Fprintf(os.Stderr, "WARN: %s: no NICs discovered\n", b.Xname)
			continue
		}

		// Allocate one IP per bootable NIC (common case is exactly 1)
		for idx, nic := range bootable {
			nodeX := bmcXnameToNode(b.Xname)
			if len(bootable) > 1 {
				nodeX = fmt.Sprintf("%s-pxe%d", nodeX, idx+1)
			}

			// If this node already has an IP in the existing nodes list, reuse it.
			existing := findByXname(doc.Nodes, nodeX)
			ipStr := ""
			if existing != nil && net.ParseIP(existing.IP) != nil {
				ipStr = existing.IP
				alloc.reserve(ipStr) // ensure IPAM knows it's taken
			} else {
				var err error
				ipStr, err = alloc.next()
				if err != nil {
					die(fmt.Sprintf("ipam allocate for %s: %v", nodeX, err))
				}
			}

			out = append(out, Entry{
				Xname: nodeX,
				MAC:   strings.ToLower(nic.MACAddress),
				IP:    ipStr,
			})
		}
	}

	// Write back to the SAME file: preserve bmcs[], replace nodes[]
	doc.Nodes = out
	bytes, err := yaml.Marshal(&doc)
	if err != nil {
		die(fmt.Sprintf("yaml marshal: %v", err))
	}
	if err := os.WriteFile(*filePath, bytes, 0o644); err != nil {
		die(fmt.Sprintf("write %s: %v", *filePath, err))
	}
	fmt.Printf("Updated %s with %d node record(s)\n", *filePath, len(out))
}

// die and findByXname moved to utils.go
