// Package formatx renders values for human consumption — block numbers, counts, rates,
// durations, sizes.
//
// It exists so that the same quantity does not get formatted three different ways in three
// different commands. Anything that turns a number into something a user reads belongs here,
// and nothing here knows about substreams types.
package formatx

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// BlockNumber renders a block number with thousands separators. Block numbers are uint64 and
// can in principle exceed what humanize.Comma accepts, hence the big.Int path.
func BlockNumber[T ~uint64](blockNumber T) string {
	if blockNumber > math.MaxInt64 {
		return humanize.BigComma(new(big.Int).SetUint64(uint64(blockNumber)))
	}

	return humanize.Comma(int64(blockNumber))
}

// Count renders a quantity compactly: 912, 3.4k, 6.8M, 1.2B. Use it where the order of
// magnitude is the point and the exact figure is not; use BlockNumber when the number
// identifies something and has to be readable digit by digit.
func Count(n uint64) string {
	if n < 1_000 {
		return fmt.Sprintf("%d", n)
	}

	value, unit := float64(n), ""
	for _, suffix := range []string{"k", "M", "B"} {
		value /= 1_000
		unit = suffix

		// Stop once the value fits in three digits. The check is against 999.95 rather than
		// 1000 because rounding to one decimal is what would push it over: 999,999 divided by a
		// thousand is 999.999, which prints as "1000k" instead of "1M".
		if value < 999.95 {
			break
		}
	}

	return trimZero(value) + unit
}

// Rate renders a per-second rate of the given unit, e.g. Rate(1700, "blk") is "1.7k blk/s".
func Rate(perSecond float64, unit string) string {
	if perSecond < 0 || math.IsNaN(perSecond) {
		perSecond = 0
	}
	return Count(uint64(perSecond)) + " " + unit + "/s"
}

// Duration is shorter than time.Duration.String() at the scales that matter to a reader: an
// age or an estimate never needs sub-second precision, and hours never need seconds.
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	// Round before classifying, never after: a value just under a boundary otherwise rounds up
	// inside its own branch and renders as "60m00s" rather than "1h00m".
	switch {
	case d < time.Hour:
		d = d.Round(time.Second)
	case d < 24*time.Hour:
		d = d.Round(time.Minute)
	default:
		d = d.Round(time.Hour)
	}

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%02dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// Millis renders a millisecond figure, keeping a decimal only while it carries signal.
func Millis(ms float64) string {
	if ms < 10 {
		return trimZero(ms) + "ms"
	}
	return fmt.Sprintf("%dms", int(ms+0.5))
}

// BytesIEC renders a byte count in binary units: 1.5 MiB.
func BytesIEC(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// JoinNonEmpty assembles "a · b · c" segments, dropping the parts that had no data rather
// than rendering them as empty separators.
func JoinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// ModuleLeafName strips the imported-package qualification from a module name:
// "erc20_metadata:erc20_stores:map_events" becomes "map_events".
func ModuleLeafName(name string) string {
	if idx := strings.LastIndex(name, ":"); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}

func trimZero(v float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0")
}
