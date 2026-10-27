package main

import (
	"fmt"
	"os"

	"github.com/ahmetartuc/dighub/v2/internal/cmd"
)

// version is overridden at release build time with -ldflags "-X main.version=..."
var version = "2.1.0"

func main() {
	os.Args = normalizeArgs(os.Args)
	if err := cmd.Execute(version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// normalizeArgs rewrites single-dash long flags (e.g. "-token") to their
// double-dash form ("--token") so they aren't misparsed by pflag as a
// shorthand flag with an attached value (e.g. "-t" + "oken"), which
// silently corrupts values like tokens and org names instead of erroring.
// Single-letter shorthand flags (e.g. "-t", "-v") are left untouched, and
// argument parsing stops at a bare "--" separator.
func normalizeArgs(args []string) []string {
	normalized := make([]string, len(args))
	copy(normalized, args)

	for i := 1; i < len(normalized); i++ {
		arg := normalized[i]
		if arg == "--" {
			break
		}
		if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
			normalized[i] = "-" + arg
		}
	}

	return normalized
}
