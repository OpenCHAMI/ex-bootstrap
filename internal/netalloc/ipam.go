package netalloc

import (
	"context"
	"net"

	ipam "github.com/metal-stack/go-ipam"
)

type Allocator struct {
	ipm    ipam.Ipamer
	prefix *ipam.Prefix
}

func NewAllocator(cidr string) (*Allocator, error) {
	ctx := context.Background()
	ipm := ipam.New(ctx)
	pr, err := ipm.NewPrefix(ctx, cidr)
	if err != nil {
		return nil, err
	}
	if gw := firstHost(pr); gw != "" {
		_, _ = ipm.AcquireSpecificIP(ctx, pr.Cidr, gw)
	}
	return &Allocator{ipm: ipm, prefix: pr}, nil
}

func (a *Allocator) Reserve(ip string) {
	_, _ = a.ipm.AcquireSpecificIP(context.Background(), a.prefix.Cidr, ip)
}

func (a *Allocator) Next() (string, error) {
	addr, err := a.ipm.AcquireIP(context.Background(), a.prefix.Cidr)
	if err != nil {
		return "", err
	}
	return addr.IP.String(), nil
}

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

func firstHost(pr *ipam.Prefix) string {
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
