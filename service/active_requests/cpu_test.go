package active_requests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCgroupFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	}
}

func TestReadQuotaCores(t *testing.T) {
	dir := t.TempDir()

	writeCgroupFiles(t, dir, map[string]string{"cpu.max": "400000 100000\n"})
	quota, err := readQuotaCores(filepath.Join(dir, "cpu.max"))
	require.NoError(t, err)
	assert.Equal(t, 4.0, quota)

	writeCgroupFiles(t, dir, map[string]string{"cpu.max": "max 100000\n"})
	quota, err = readQuotaCores(filepath.Join(dir, "cpu.max"))
	require.NoError(t, err)
	assert.Equal(t, 0.0, quota)

	writeCgroupFiles(t, dir, map[string]string{"cpu.max": "garbage\n"})
	_, err = readQuotaCores(filepath.Join(dir, "cpu.max"))
	require.Error(t, err)
}

func TestReadCPUStat(t *testing.T) {
	dir := t.TempDir()
	writeCgroupFiles(t, dir, map[string]string{"cpu.stat": `usage_usec 1000000
user_usec 800000
system_usec 200000
nr_periods 500
nr_throttled 50
throttled_usec 123456
`})

	usage, periods, throttled, err := readCPUStat(filepath.Join(dir, "cpu.stat"))
	require.NoError(t, err)
	assert.Equal(t, uint64(1000000), usage)
	assert.Equal(t, uint64(500), periods)
	assert.Equal(t, uint64(50), throttled)
}

func TestReadPressureSomeAvg10(t *testing.T) {
	dir := t.TempDir()
	writeCgroupFiles(t, dir, map[string]string{"cpu.pressure": `some avg10=1.50 avg60=0.80 avg300=0.20 total=12345
full avg10=0.10 avg60=0.05 avg300=0.01 total=678
`})

	psi, err := readPressureSomeAvg10(filepath.Join(dir, "cpu.pressure"))
	require.NoError(t, err)
	assert.InDelta(t, 0.015, psi, 1e-9)
}

func TestComputeRatios(t *testing.T) {
	t0 := time.Now()
	prev := &cpuSample{at: t0, usageUsec: 1_000_000, nrPeriods: 100, nrThrottled: 10}
	// 10s elapsed, 38s of CPU consumed on a 4-core quota => 95% usage; 45/50 periods throttled
	cur := &cpuSample{at: t0.Add(10 * time.Second), usageUsec: 39_000_000, nrPeriods: 150, nrThrottled: 55}

	usage, throttle := computeRatios(prev, cur, 4.0)
	assert.InDelta(t, 0.95, usage, 1e-9)
	assert.InDelta(t, 0.9, throttle, 1e-9)

	usage, throttle = computeRatios(nil, cur, 4.0)
	assert.Zero(t, usage)
	assert.Zero(t, throttle)

	// counter reset (container restart of a shared reader): ratios stay at 0 rather than going negative
	usage, throttle = computeRatios(cur, prev, 4.0)
	assert.Zero(t, usage)
	assert.Zero(t, throttle)
}

func TestNewCPUReaderAt(t *testing.T) {
	dir := t.TempDir()
	writeCgroupFiles(t, dir, map[string]string{
		"cpu.max": "350000 100000\n",
		"cpu.stat": `usage_usec 1000000
nr_periods 500
nr_throttled 50
`,
		"cpu.pressure": "some avg10=2.00 avg60=1.00 avg300=0.50 total=999\n",
	})

	reader, err := newCPUReaderAt(dir)
	require.NoError(t, err)
	assert.Equal(t, 3.5, reader.QuotaCores())

	signals, err := reader.Read()
	require.NoError(t, err)
	assert.Equal(t, 3.5, signals.QuotaCores)
	assert.Zero(t, signals.UsageRatio) // first read has no previous sample
	assert.InDelta(t, 0.02, signals.PressureAvg10, 1e-9)

	_, err = newCPUReaderAt(t.TempDir()) // no cgroup files at all
	require.Error(t, err)
}

func TestResolveCgroupDir(t *testing.T) {
	mount := t.TempDir()
	nested := filepath.Join(mount, "kubepods", "pod1")
	require.NoError(t, os.MkdirAll(nested, 0755))
	writeCgroupFiles(t, nested, map[string]string{"cpu.stat": "usage_usec 1\n"})

	procFile := filepath.Join(t.TempDir(), "cgroup")
	require.NoError(t, os.WriteFile(procFile, []byte("0::/kubepods/pod1\n"), 0644))

	dir, err := resolveCgroupDir(mount, procFile)
	require.NoError(t, err)
	assert.Equal(t, nested, dir)

	// cgroup namespace case: /proc path resolves nowhere useful, fall back to the mount root
	writeCgroupFiles(t, mount, map[string]string{"cpu.stat": "usage_usec 1\n"})
	dir, err = resolveCgroupDir(mount, filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	assert.Equal(t, mount, dir)
}
