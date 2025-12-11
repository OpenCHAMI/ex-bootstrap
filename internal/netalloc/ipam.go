// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

// Package netalloc provides IP address allocation utilities.
package netalloc

import (
	"context"
	"fmt"
	"net"

	ipam "github.com/metal-stack/go-ipam"
)

// Allocator manages IP address allocation within a specified subnet.
type Allocator struct {
	ipm    ipam.Ipamer
	prefix *ipam.Prefix
}

// NewAllocator creates a new Allocator for the given CIDR subnet.
func NewAllocator(cidr string) (*Allocator, error) {
	ctx := context.Background()
	ipm := ipam.New(ctx)
	pr, err := ipm.NewPrefix(ctx, cidr)
	if err != nil {
		return nil, err
	}
	// Previously we reserved the first host (gateway) to avoid collisions.
	// Removing that reservation allows allocation of the .1 address when desired.
	return &Allocator{ipm: ipm, prefix: pr}, nil
}

// Reserve marks the specified IP address as reserved in the allocator.
func (a *Allocator) Reserve(ip string) {
	_, _ = a.ipm.AcquireSpecificIP(context.Background(), a.prefix.Cidr, ip)
}

// Next allocates and returns the next available IP address in the subnet.
func (a *Allocator) Next() (string, error) {
	addr, err := a.ipm.AcquireIP(context.Background(), a.prefix.Cidr)
	if err != nil {
		return "", err
	}
	return addr.IP.String(), nil
}

// Contains checks if the given IP address is within the allocator's subnet.
func (a *Allocator) Contains(ip string) bool {
	_, n, err := net.ParseCIDR(a.prefix.Cidr)
	if err != nil {
		return false
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return n.Contains(parsedIP)
}

// ReserveUpTo reserves all IP addresses from the start of the subnet up to (but not including) the specified IP.
// This is useful for skipping a range of IPs before allocation begins.
func (a *Allocator) ReserveUpTo(startIP string) error {
	if startIP == "" {
		return nil
	}
	if !a.Contains(startIP) {
		return fmt.Errorf("start IP %s is not in subnet %s", startIP, a.prefix.Cidr)
	}
	// Parse the start IP
	startParsed := net.ParseIP(startIP)
	if startParsed == nil {
		return fmt.Errorf("invalid start IP: %s", startIP)
	}

	// Reserve IPs until we reach the start IP
	for {
		addr, err := a.ipm.AcquireIP(context.Background(), a.prefix.Cidr)
		if err != nil {
			// No more IPs available or error
			return nil
		}
		allocatedIP := net.ParseIP(addr.IP.String())
		// Stop when we've reserved everything before startIP
		if allocatedIP.Equal(startParsed) || isIPGreaterThan(allocatedIP, startParsed) {
			// Release this IP since we don't want to reserve it
			_, _ = a.ipm.ReleaseIP(context.Background(), addr)
			return nil
		}
	}
}

// isIPGreaterThan returns true if ip1 > ip2
func isIPGreaterThan(ip1, ip2 net.IP) bool {
	ip1v4 := ip1.To4()
	ip2v4 := ip2.To4()
	if ip1v4 == nil || ip2v4 == nil {
		return false
	}
	for i := 0; i < 4; i++ {
		if ip1v4[i] > ip2v4[i] {
			return true
		}
		if ip1v4[i] < ip2v4[i] {
			return false
		}
	}
	return false
}
