package active_requests

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CPUSignals is one evaluation of the CPU accounting of this process's own
// cgroup. Every value is scoped to the container's cgroup: neighbor pods on
// the same node do not affect UsageRatio.
type CPUSignals struct {
	QuotaCores float64 // from cpu.max; 0 means no limit set
	UsageRatio float64 // CPU consumed since the previous Read, as a fraction of quota (0 when no quota)
}

type cpuSample struct {
	at        time.Time
	usageUsec uint64
}

// CPUReader reads CPU usage from the current process's cgroup v2 directory.
// UsageRatio is a delta between consecutive Read calls, so the first Read
// returns it as 0.
type CPUReader struct {
	cgroupDir  string
	quotaCores float64
	prev       *cpuSample
}

// NewCPUReader resolves the process's cgroup v2 directory and validates that
// CPU accounting is readable. It returns an error on hosts without cgroup v2
// (macOS, cgroup v1 nodes); callers treat that as "CPU signals unavailable".
// SUBSTREAMS_CGROUP_DIR overrides the directory, for testing the evictor on
// hosts without cgroups by feeding cpu.max and cpu.stat files.
func NewCPUReader() (*CPUReader, error) {
	if dir := os.Getenv("SUBSTREAMS_CGROUP_DIR"); dir != "" {
		return newCPUReaderAt(dir)
	}
	dir, err := resolveCgroupDir("/sys/fs/cgroup", "/proc/self/cgroup")
	if err != nil {
		return nil, err
	}
	return newCPUReaderAt(dir)
}

func newCPUReaderAt(dir string) (*CPUReader, error) {
	quota, err := readQuotaCores(filepath.Join(dir, "cpu.max"))
	if err != nil {
		return nil, fmt.Errorf("reading cpu.max: %w", err)
	}
	if _, err := readUsageUsec(filepath.Join(dir, "cpu.stat")); err != nil {
		return nil, fmt.Errorf("reading cpu.stat: %w", err)
	}
	return &CPUReader{cgroupDir: dir, quotaCores: quota}, nil
}

// resolveCgroupDir finds the cgroup v2 directory of the current process.
// Inside a container with a cgroup namespace, the root mount point is the
// container's own cgroup. Without a cgroup namespace, the process's cgroup
// path from /proc/self/cgroup must be joined to the mount point.
func resolveCgroupDir(mountPoint, procSelfCgroup string) (string, error) {
	if data, err := os.ReadFile(procSelfCgroup); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			// cgroup v2 entry: "0::/some/path"
			path, found := strings.CutPrefix(line, "0::")
			if !found {
				continue
			}
			dir := filepath.Join(mountPoint, path)
			if _, err := os.Stat(filepath.Join(dir, "cpu.stat")); err == nil {
				return dir, nil
			}
		}
	}
	if _, err := os.Stat(filepath.Join(mountPoint, "cpu.stat")); err == nil {
		return mountPoint, nil
	}
	return "", fmt.Errorf("no readable cgroup v2 cpu.stat found under %s", mountPoint)
}

func (r *CPUReader) QuotaCores() float64 { return r.quotaCores }

func (r *CPUReader) Read() (CPUSignals, error) {
	usageUsec, err := readUsageUsec(filepath.Join(r.cgroupDir, "cpu.stat"))
	if err != nil {
		return CPUSignals{}, fmt.Errorf("reading cpu.stat: %w", err)
	}

	sample := &cpuSample{at: time.Now(), usageUsec: usageUsec}
	out := CPUSignals{QuotaCores: r.quotaCores, UsageRatio: computeUsageRatio(r.prev, sample, r.quotaCores)}
	r.prev = sample

	return out, nil
}

func computeUsageRatio(prev, cur *cpuSample, quotaCores float64) float64 {
	if prev == nil {
		return 0
	}
	elapsed := cur.at.Sub(prev.at)
	if elapsed <= 0 || quotaCores <= 0 || cur.usageUsec < prev.usageUsec {
		return 0
	}
	usedCores := float64(cur.usageUsec-prev.usageUsec) / float64(elapsed.Microseconds())
	return usedCores / quotaCores
}

// readQuotaCores parses cpu.max: "$MAX $PERIOD" in microseconds, or "max $PERIOD"
// when no limit is set (returned as 0).
func readQuotaCores(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, fmt.Errorf("unexpected cpu.max content %q", strings.TrimSpace(string(data)))
	}
	if fields[0] == "max" {
		return 0, nil
	}
	quota, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing cpu.max quota: %w", err)
	}
	period, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing cpu.max period: %w", err)
	}
	if period == 0 {
		return 0, fmt.Errorf("cpu.max period is 0")
	}
	return float64(quota) / float64(period), nil
}

func readUsageUsec(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "usage_usec" {
			continue
		}
		return strconv.ParseUint(fields[1], 10, 64)
	}
	return 0, fmt.Errorf("no usage_usec entry in %s", path)
}
