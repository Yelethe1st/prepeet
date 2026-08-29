package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: apicompat <previous.yaml> <current.yaml>")
		os.Exit(2)
	}

	previous, err := Load(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	current, err := Load(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	breaks := Compare(previous, current)
	if len(breaks) == 0 {
		fmt.Println("PASS the HTTP contract would not break a deployed client")
		return
	}

	fmt.Fprintf(os.Stderr, "%d breaking change(s) to the HTTP contract:\n\n", len(breaks))
	for _, broken := range breaks {
		fmt.Fprintf(os.Stderr, "  %s\n\n", broken)
	}
	fmt.Fprintln(os.Stderr, "A deployed client built against the previous release would stop working.")
	fmt.Fprintln(os.Stderr, "Add the change alongside what is there rather than in place of it, or")
	fmt.Fprintln(os.Stderr, "carry it in a new version of the operation.")
	os.Exit(1)
}
