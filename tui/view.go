package tui

import (
	"fmt"
	"strings"
)

// View renders the live region: the backprocessing block, and the failure report when there
// is one. The session preamble is deliberately not part of it — see formatSessionPreamble.
//
// This used to be a text/template. It was replaced by plain string building because the
// template carried a branch on a field nothing ever set, which silently swallowed the
// backprocessing target line and emitted stray blank lines in its place.
func (m model) View() string {
	if !m.Connected {
		return "Connecting...\n"
	}

	lines := m.progressLines()
	lines = append(lines, m.failureLines()...)

	return strings.Join(lines, "\n") + "\n"
}

func (m model) failureLines() []string {
	if m.Failures == 0 || m.LastFailure == nil {
		return nil
	}

	lines := []string{
		"",
		fmt.Sprintf("Failures: %d.", m.Failures),
		"Last failure:",
		fmt.Sprintf("  Reason: %s", m.LastFailure.Reason),
	}

	if len(m.LastFailure.Logs) != 0 {
		lines = append(lines, "  Logs:")
		for _, log := range m.LastFailure.Logs {
			lines = append(lines, "    "+log)
		}
	}
	if m.LastFailure.LogsTruncated {
		lines = append(lines, "  <logs truncated>")
	}

	return lines
}
