package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Version value, injected via go build `ldflags` at build time
var version = "dev"

func main() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		panic("we should have been able to retrieve info from 'runtime/debug#ReadBuildInfo'")
	}

	rootCmd.Version = computeVersionString(version, info.Settings)
	setup()

	if err := rootCmd.Execute(); err != nil {

	}
}

func computeVersionString(version string, settings []debug.BuildSetting) string {
	commit := findSetting("vcs.revision", settings)
	date := findSetting("vcs.time", settings)

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

func findSetting(key string, settings []debug.BuildSetting) (value string) {
	for _, setting := range settings {
		if setting.Key == key {
			return setting.Value
		}
	}

	return ""
}
