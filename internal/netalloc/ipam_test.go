// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package netalloc

import (
	"strings"
	"testing"
)

func TestAllocatorGatewayReserved(t *testing.T) {
	a, err := NewAllocator("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewAllocator: %v", err)
	}
	ip1, err := a.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !strings.HasPrefix(ip1, "10.0.0.") {
		t.Fatalf("unexpected ip1: %s", ip1)
	}
	if ip1 != "10.0.0.1" {
		t.Fatalf("expected first allocation to be .1, got %s", ip1)
	}
}

func TestAllocatorReserveAndNext(t *testing.T) {
	a, err := NewAllocator("10.0.1.0/29") // hosts .1-.6, gw .1 reserved
	if err != nil {
		t.Fatalf("NewAllocator: %v", err)
	}
	// First Next should be .1 (no gateway reserved)
	if ip, _ := a.Next(); ip != "10.0.1.1" {
		t.Fatalf("got %s want 10.0.1.1", ip)
	}
	// Reserve .4, ensure sequence does not return .4
	a.Reserve("10.0.1.4")
	// Next should be .2
	if ip, _ := a.Next(); ip != "10.0.1.2" {
		t.Fatalf("got %s want 10.0.1.2", ip)
	}
	// Next should be .3
	if ip, _ := a.Next(); ip != "10.0.1.3" {
		t.Fatalf("got %s want 10.0.1.3", ip)
	}
	// Next should skip .4 (reserved) and return .5
	if ip, _ := a.Next(); ip != "10.0.1.5" {
		t.Fatalf("got %s want 10.0.1.5", ip)
	}
}

func TestAllocatorReserveUpTo(t *testing.T) {
	a, err := NewAllocator("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewAllocator: %v", err)
	}

	// Reserve all IPs up to .10
	if err := a.ReserveUpTo("10.0.0.10"); err != nil {
		t.Fatalf("ReserveUpTo: %v", err)
	}

	// First Next should return .10
	ip1, err := a.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if ip1 != "10.0.0.10" {
		t.Fatalf("expected first allocation after ReserveUpTo to be 10.0.0.10, got %s", ip1)
	}

	// Second allocation should be .11
	ip2, err := a.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if ip2 != "10.0.0.11" {
		t.Fatalf("expected second allocation to be 10.0.0.11, got %s", ip2)
	}
}

func TestAllocatorReserveUpToInvalidIP(t *testing.T) {
	a, err := NewAllocator("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewAllocator: %v", err)
	}

	// Try to reserve up to an IP outside the subnet
	err = a.ReserveUpTo("192.168.1.1")
	if err == nil {
		t.Fatalf("expected error when reserving IP outside subnet")
	}
}
