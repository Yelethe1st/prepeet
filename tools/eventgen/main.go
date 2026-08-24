package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Where the contract is read from and where the generated code goes.
//
// Paths are relative to the repository root, and the generator refuses to run
// from anywhere else rather than writing a tree of directories wherever it
// happens to be. An earlier generator in this repository did exactly that and
// left a stray directory nobody noticed for days, because .gitignore hid it.
const (
	contractsDir = "packages/contracts/events"
	goOut        = "packages/generated/go/prepeetevents/catalogue.gen.go"
	tsOut        = "packages/generated/typescript/events.gen.ts"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "eventgen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	baseline := flag.String("baseline", "",
		"a checkout of packages/contracts/events at the previous release. When given, the "+
			"contracts are compared against it and anything that would break a consumer is "+
			"reported instead of generating.")
	flag.Parse()

	if *baseline != "" {
		return check(*baseline)
	}
	return generate()
}

// check compares the current contracts against a previous release.
//
// Against a release rather than the previous commit, per ADR-0004, so that a
// change can be revised while in progress without the gate firing on every
// intermediate state.
func check(baselineDir string) error {
	before, err := Load(baselineDir)
	if err != nil {
		return fmt.Errorf("reading the baseline: %w", err)
	}
	after, err := Load(contractsDir)
	if err != nil {
		return err
	}

	breaks := Compare(before, after)
	if len(breaks) == 0 {
		fmt.Printf("eventgen: %d events, no change that would break a consumer\n", len(after.Events))
		return nil
	}

	fmt.Fprintf(os.Stderr, "eventgen: %d change(s) would break a consumer built against the previous release:\n\n", len(breaks))
	for _, b := range breaks {
		fmt.Fprintf(os.Stderr, "  %s\n\n", b)
	}
	return fmt.Errorf("refusing a breaking change to the event contracts")
}

func generate() error {
	// The marker is the contracts directory itself. Checking for it turns
	// "wrote the files somewhere unexpected" into a refusal with a message.
	if _, err := os.Stat(contractsDir); err != nil {
		return fmt.Errorf("run from the repository root: %s is not here", contractsDir)
	}

	catalogue, err := Load(contractsDir)
	if err != nil {
		return err
	}
	if len(catalogue.Events) == 0 {
		return fmt.Errorf("%s holds no events; refusing to generate an empty catalogue", contractsDir)
	}

	for path, content := range map[string]string{
		goOut: generateGo(catalogue),
		tsOut: generateTypeScript(catalogue),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	fmt.Printf("eventgen: %d events to %s and %s\n", len(catalogue.Events), goOut, tsOut)
	return nil
}
