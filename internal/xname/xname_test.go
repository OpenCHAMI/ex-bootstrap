package xname

import "testing"

func TestBMCXnameToNode(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"x1000c0s0b0", "x1000c0s0n0"},
		{"x1000c0s0b1", "x1000c0s0n1"},
		{"x9999c1s2", "x9999c1s2-n0"},
	}
	for _, c := range cases {
		got := BMCXnameToNode(c.in)
		if got != c.out {
			t.Fatalf("BMCXnameToNode(%q)=%q want %q", c.in, got, c.out)
		}
	}
}

func TestBMCXnameToNodeN(t *testing.T) {
	cases := []struct {
		bmcX    string
		nodeNum int
		out     string
	}{
		{"x9000c1s0b0", 0, "x9000c1s0b0n0"},
		{"x9000c1s0b0", 1, "x9000c1s0b0n1"},
		{"x9000c1s0b0", 2, "x9000c1s0b0n2"},
		{"x1000c0s0b1", 0, "x1000c0s0b1n0"},
		{"x1000c0s0b1", 3, "x1000c0s0b1n3"},
		{"x9999c1s2", 0, "x9999c1s2n0"},
		{"x9999c1s2", 1, "x9999c1s2n1"},
	}
	for _, c := range cases {
		got := BMCXnameToNodeN(c.bmcX, c.nodeNum)
		if got != c.out {
			t.Fatalf("BMCXnameToNodeN(%q, %d)=%q want %q", c.bmcX, c.nodeNum, got, c.out)
		}
	}
}
