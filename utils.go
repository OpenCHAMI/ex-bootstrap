package main

import (
	"fmt"
	"os"
)

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func findByXname(list []Entry, x string) *Entry {
	for i := range list {
		if list[i].Xname == x {
			return &list[i]
		}
	}
	return nil
}
