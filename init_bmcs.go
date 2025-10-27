package main

import (
	"fmt"
	"strings"
)

// getBmcID mirrors Python: (n+1)//2
func getBmcID(n int) int { return (n + 1) / 2 }

// 4 nodes per blade, 8 blades per chassis
func getSlot(n int) int { return ((n - 1) / 4) % 8 }

// 2 nodes per blade
func getBlade(n int) int { return ((n - 1) / 2) % 2 }

// NodeBMC xname: chassis + "s<slot>b<blade>"
func getNCXname(chassis string, n int) string {
	return fmt.Sprintf("%ss%db%d", chassis, getSlot(n), getBlade(n))
}

// MAC composition mirroring Python string concatenation.
func getNCMAC(macStart string, n int) string {
	return fmt.Sprintf("%s:%d%d:%d0", macStart, 3, getSlot(n), getBlade(n))
}

// parseChassisSpec parses comma-separated k=v mappings.
func parseChassisSpec(spec string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(spec) == "" {
		return out
	}
	parts := strings.Split(spec, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}
