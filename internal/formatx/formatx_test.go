package formatx

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInteger(t *testing.T) {
	assert.Equal(t, "0", Integer(uint64(0)))
	assert.Equal(t, "1,500,000", Integer(uint64(1_500_000)))
	// Past MaxInt64 the int64 conversion would wrap, hence the big.Int path.
	assert.Equal(t, "18,446,744,073,709,551,615", Integer(uint64(math.MaxUint64)))
}

func TestCount(t *testing.T) {
	cases := map[uint64]string{
		0:     "0",
		912:   "912",
		999:   "999",
		1_000: "1k",
		3_400: "3.4k",
		3_450: "3.5k",
		// Rounding to one decimal must promote the unit rather than render "1000k".
		999_999:       "1M",
		6_800_000:     "6.8M",
		1_200_000_000: "1.2B",
	}

	for in, expected := range cases {
		assert.Equal(t, expected, Count(in), "Count(%d)", in)
	}
}

func TestRate(t *testing.T) {
	assert.Equal(t, "0 blk/s", Rate(0, "blk"))
	assert.Equal(t, "1.7k blk/s", Rate(1_700, "blk"))
	// A rate derived from a window can go negative or NaN on a counter reset.
	assert.Equal(t, "0 blk/s", Rate(-5, "blk"))
	assert.Equal(t, "0 blk/s", Rate(math.NaN(), "blk"))
}

func TestDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                                       "0s",
		-time.Second:                            "0s",
		41 * time.Second:                        "41s",
		3*time.Minute + 52*time.Second:          "3m52s",
		time.Hour + 4*time.Minute:               "1h04m",
		25*time.Hour + 30*time.Minute:           "1d02h",
		49 * time.Hour:                          "2d01h",
		59*time.Minute + 59500*time.Millisecond: "1h00m",
	}

	for in, expected := range cases {
		assert.Equal(t, expected, Duration(in), "Duration(%s)", in)
	}
}

func TestMillis(t *testing.T) {
	assert.Equal(t, "0ms", Millis(0))
	assert.Equal(t, "1.5ms", Millis(1.5))
	assert.Equal(t, "9ms", Millis(9.04))
	assert.Equal(t, "13ms", Millis(12.5))
}

func TestBytes(t *testing.T) {
	assert.Equal(t, "512 B", Bytes(512))
	assert.Equal(t, "1.0 KiB", Bytes(1024))
	assert.Equal(t, "1.5 MiB", Bytes(1024*1024*3/2))

	// Binary units are the default because that is what a size is measured in; the decimal ones
	// are an option rather than a second function.
	assert.Equal(t, "1.0 KB", Bytes(1000, WithDecimalUnits()))
	assert.Equal(t, "1.5 MB", Bytes(1_500_000, WithDecimalUnits()))
}

// Zero is an amount for some values and the absence of a measurement for others, so which of
// the two it is belongs to the caller rather than to a dedicated wrapper per call site.
func TestWithZero(t *testing.T) {
	assert.Equal(t, "0s", Duration(0))
	assert.Equal(t, "N/A", Duration(0, WithZero("N/A")))
	assert.Equal(t, "41s", Duration(41*time.Second, WithZero("N/A")))

	assert.Equal(t, "0", Integer(uint64(0)))
	assert.Equal(t, "none", Integer(uint64(0), WithZero("none")))
	assert.Equal(t, "1,500,000", Integer(uint64(1_500_000), WithZero("none")))

	assert.Equal(t, "0", Count(0))
	assert.Equal(t, "none", Count(0, WithZero("none")))

	assert.Equal(t, "0 B", Bytes(0))
	assert.Equal(t, "nothing", Bytes(0, WithZero("nothing")))

	assert.Equal(t, "0ms", Millis(0))
	assert.Equal(t, "N/A", Millis(0, WithZero("N/A")))

	assert.Equal(t, "0 blk/s", Rate(0, "blk"))
	assert.Equal(t, "idle", Rate(0, "blk", WithZero("idle")))
	// A negative or NaN rate is clamped to zero first, so it takes the same branch.
	assert.Equal(t, "idle", Rate(-5, "blk", WithZero("idle")))
}

func TestJoinNonEmpty(t *testing.T) {
	assert.Equal(t, "a · c", JoinNonEmpty(" · ", "a", "", "c"))
	assert.Equal(t, "", JoinNonEmpty(" · ", "", ""))
}

func TestModuleLeafName(t *testing.T) {
	assert.Equal(t, "map_events", ModuleLeafName("erc20_metadata:erc20_stores:map_events"))
	assert.Equal(t, "map_events", ModuleLeafName("map_events"))
	// A trailing colon is not a qualification, and stripping it would leave nothing at all.
	assert.Equal(t, "map_events:", ModuleLeafName("map_events:"))
}
