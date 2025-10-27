package main

import "regexp"

var trailingB = regexp.MustCompile(`b(\d+)$`)

// bmcXnameToNode converts e.g. x1000c0s0b0 -> x1000c0s0n0, x...b1 -> x...n1.
// If it does not match, we append "-n0".
func bmcXnameToNode(bmcX string) string {
	if m := trailingB.FindStringSubmatch(bmcX); m != nil {
		return trailingB.ReplaceAllString(bmcX, "n$1")
	}
	return bmcX + "-n0"
}
