package main

import (
	"context"
	"net"

	"github.com/metal-stack/go-ipam"
)

type allocator struct {
	ipm    ipam.Ipamer
	prefix *ipam.Prefix
}

func newAllocator(cidr string) (*allocator, error) {
	ctx := context.Background()
	ipm := ipam.New(ctx)
	pr, err := ipm.NewPrefix(ctx, cidr)
	if err != nil {
		return nil, err
	}
	// Reserve network,broadcast implicitly by not allocating them.
	// Common gateway (.1) is reserved explicitly so we don't hand it out.
	if gw := firstHost(pr); gw != "" {
		_, _ = ipm.AcquireSpecificIP(ctx, pr.Cidr, gw)
	}
	return &allocator{ipm: ipm, prefix: pr}, nil
}

func (a *allocator) reserve(ip string) {
	_, _ = a.ipm.AcquireSpecificIP(context.Background(), a.prefix.Cidr, ip) // best-effort
}

func (a *allocator) next() (string, error) {
	addr, err := a.ipm.AcquireIP(context.Background(), a.prefix.Cidr)
	if err != nil {
		return "", err
	}
	return addr.IP.String(), nil
}

func firstHost(pr *ipam.Prefix) string {
	// crude: for IPv4, the first assignable is .1, often a gateway; we’ll reserve it.
	_, n, err := net.ParseCIDR(pr.Cidr)
	if err != nil {
		return ""
	}
	v4 := n.IP.To4()
	if v4 == nil {
		return ""
	}
	ip := net.IPv4(v4[0], v4[1], v4[2], v4[3]+1)
	if n.Contains(ip) {
		return ip.String()
	}
	return ""
}
