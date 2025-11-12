package xname

import (
	"fmt"
	"regexp"
)

var trailingB = regexp.MustCompile(`b(\d+)$`)

// BMCXnameToNode converts e.g. x1000c0s0b0 -> x1000c0s0n0, x...b1 -> x...n1.
// If it does not match, we append "-n0".
func BMCXnameToNode(bmcX string) string {
	if m := trailingB.FindStringSubmatch(bmcX); m != nil {
		return trailingB.ReplaceAllString(bmcX, "n$1")
	}
	return bmcX + "-n0"
}

// BMCXnameToNodeN converts a BMC xname to a node xname with a specific node number.
// E.g. x9000c1s0b0 with nodeNum 0 -> x9000c1s0b0n0, with nodeNum 1 -> x9000c1s0b0n1.
func BMCXnameToNodeN(bmcX string, nodeNum int) string {
	// Append nY where Y is the nodeNum
	return fmt.Sprintf("%sn%d", bmcX, nodeNum)
}
