package tui

import (
	"strings"

	"github.com/streamingfast/substreams/internal/formatx"
)

const (
	barFull    = "█"
	barPartial = "▓"
	barEmpty   = "░"

	defaultBarWidth = 20
	minBarWidth     = 8
)

// renderBar draws a fixed-width progress bar. The partial glyph marks a bin that is started
// but not finished, so a bar only reads as full when the work actually is.
func renderBar(ratio float64, width int) string {
	if width < 1 {
		width = 1
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	full := int(ratio * float64(width))
	if full > width {
		full = width
	}

	out := strings.Repeat(barFull, full)
	remaining := width - full
	if remaining == 0 {
		return out
	}

	// Anything strictly above the last full bin gets a partial glyph, so a bar at 1% shows
	// movement instead of reading as "not started".
	if ratio*float64(width) > float64(full) {
		out += barPartial
		remaining--
	}

	return out + strings.Repeat(barEmpty, remaining)
}

// barWidthFor keeps the bars readable on narrow terminals. Width is 0 until the first
// tea.WindowSizeMsg arrives, in which case the default applies.
func barWidthFor(terminalWidth int) int {
	if terminalWidth <= 0 || terminalWidth >= 80 {
		return defaultBarWidth
	}

	width := terminalWidth - 55
	if width < minBarWidth {
		return minBarWidth
	}

	return width
}

// costPerBlock renders a module's per-block cost.
func costPerBlock(msPerBlock float64) string {
	return formatx.Millis(msPerBlock) + "/blk"
}

// shortModuleNames maps each module to its leaf name, keeping the fully qualified name
// whenever two modules would otherwise collide.
func shortModuleNames(names []string) map[string]string {
	leaves := map[string]int{}
	for _, name := range names {
		leaves[formatx.ModuleLeafName(name)]++
	}

	out := make(map[string]string, len(names))
	for _, name := range names {
		leaf := formatx.ModuleLeafName(name)
		if leaves[leaf] > 1 {
			out[name] = name
		} else {
			out[name] = leaf
		}
	}
	return out
}
