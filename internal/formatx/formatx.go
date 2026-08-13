// Package formatx renders values for human consumption — block numbers, counts, rates,
// durations, sizes.
//
// It exists so that the same quantity does not get formatted three different ways in three
// different commands. Anything that turns a number into something a user reads belongs here,
// and nothing here knows about substreams types.
//
// Every function takes the same Option list rather than coming in named variants, so a caller
// that wants "N/A" instead of "0s", or decimal units instead of binary ones, asks for that at
// the call site instead of growing a wrapper next to it.
package formatx

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// Option adjusts how a value is rendered. Options are shared across every function here; one
// that does not apply to a given value is simply ignored.
type Option func(*config)

type config struct {
	zeroText string
	hasZero  bool
	decimal  bool
}

func newConfig(opts []Option) config {
	var c config
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// WithZero renders a zero value as the given text instead of formatting it. Use it where zero
// means "not known" or "not applicable" rather than an actual measurement — an unset duration
// reading "N/A" says more than one reading "0s".
func WithZero(text string) Option {
	return func(c *config) {
		c.zeroText, c.hasZero = text, true
	}
}

// WithDecimalUnits switches Bytes from binary units to powers of a thousand: 1.5 MB rather
// than 1.5 MiB.
func WithDecimalUnits() Option {
	return func(c *config) { c.decimal = true }
}

// Integer renders a whole number with thousands separators, for block numbers and for exact
// counts alike — anything a reader has to be able to take in digit by digit, as opposed to
// Count, which trades those digits for an order of magnitude. The values are uint64 and can in
// principle exceed what humanize.Comma accepts, hence the big.Int path.
func Integer[T ~uint64](value T, opts ...Option) string {
	c := newConfig(opts)
	if value == 0 && c.hasZero {
		return c.zeroText
	}

	if value > math.MaxInt64 {
		return humanize.BigComma(new(big.Int).SetUint64(uint64(value)))
	}

	return humanize.Comma(int64(value))
}

// Count renders a quantity compactly: 912, 3.4k, 6.8M, 1.2B. Use it where the order of
// magnitude is the point and the exact figure is not; use BlockNumber when the number
// identifies something and has to be readable digit by digit.
func Count(n uint64, opts ...Option) string {
	c := newConfig(opts)
	if n == 0 && c.hasZero {
		return c.zeroText
	}

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
func Rate(perSecond float64, unit string, opts ...Option) string {
	if perSecond < 0 || math.IsNaN(perSecond) {
		perSecond = 0
	}

	c := newConfig(opts)
	if perSecond == 0 && c.hasZero {
		return c.zeroText
	}

	return Count(uint64(perSecond)) + " " + unit + "/s"
}

// Duration is shorter than time.Duration.String() at the scales that matter to a reader: an
// age or an estimate never needs sub-second precision, and hours never need seconds.
func Duration(d time.Duration, opts ...Option) string {
	c := newConfig(opts)
	if d == 0 && c.hasZero {
		return c.zeroText
	}

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
func Millis(ms float64, opts ...Option) string {
	c := newConfig(opts)
	if ms == 0 && c.hasZero {
		return c.zeroText
	}

	if ms < 10 {
		return trimZero(ms) + "ms"
	}

	return fmt.Sprintf("%dms", int(ms+0.5))
}

// Bytes renders a byte count in binary units by default — 1.5 MiB — since that is what a file
// size or a transfer volume is measured in. WithDecimalUnits switches to 1.5 MB.
func Bytes(bytes uint64, opts ...Option) string {
	c := newConfig(opts)
	if bytes == 0 && c.hasZero {
		return c.zeroText
	}

	base, suffix := uint64(1024), "iB"
	if c.decimal {
		base, suffix = 1000, "B"
	}

	if bytes < base {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := base, 0
	for n := bytes / base; n >= base; n /= base {
		div *= base
		exp++
	}

	return fmt.Sprintf("%.1f %c%s", float64(bytes)/float64(div), "KMGTPE"[exp], suffix)
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
