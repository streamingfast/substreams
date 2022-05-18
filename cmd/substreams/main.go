package main

import (
	"fmt"
	"strings"

	"github.com/streamingfast/derr"
)

// Commit sha1 value, injected via go build `ldflags` at build time
var commit = ""

// Version value, injected via go build `ldflags` at build time
var version = "dev"

// Date value, injected via go build `ldflags` at build time
var date = ""

func main() {
	rootCmd.Version = computeVersionString(version, commit, date)
	setup()
	derr.Check("substreams", rootCmd.Execute())
}

func computeVersionString(version, commit, date string) string {
	var labels []string
	if len(commit) >= 7 {
		labels = append(labels, fmt.Sprintf("Commit %s", commit[0:7]))
	}

	if date != "" {
		labels = append(labels, fmt.Sprintf("Built %s", date))
	}

	if len(labels) == 0 {
		return version
	}

	return fmt.Sprintf("%s (%s)", version, strings.Join(labels, ", "))
}
