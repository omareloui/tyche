// Package sysinfo reads the process' memory and runtime state for the admin
// logs page: Go's own numbers, plus what the kernel sees (the RSS includes
// SQLite's C allocations, which the Go heap doesn't), the container's cgroup
// and the host.
package sysinfo

import (
	"bufio"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/buildinfo"
)

var started = time.Now()

type Info struct {
	Commit    string
	GoVersion string
	Uptime    time.Duration

	HeapAlloc uint64
	HeapInuse uint64
	// GoSys is all the memory the Go runtime got from the OS.
	GoSys uint64
	// MemLimit is GOMEMLIMIT, 0 when unset.
	MemLimit   uint64
	NumGC      uint32
	LastGC     time.Time
	LastPause  time.Duration
	Goroutines int

	// RSS is the process' resident memory, 0 when unknown (not Linux).
	RSS uint64
	// CgroupCurrent is the container's memory, CgroupMax its hard limit
	// (0 = none), OOMKills the cgroup's count; all 0 outside cgroup v2.
	CgroupCurrent uint64
	CgroupMax     uint64
	OOMKills      int
	// HostTotal / HostAvailable are the host's memory (/proc/meminfo).
	HostTotal     uint64
	HostAvailable uint64
}

func Read() Info {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	i := Info{
		Commit:     buildinfo.Commit,
		GoVersion:  runtime.Version(),
		Uptime:     time.Since(started),
		HeapAlloc:  ms.HeapAlloc,
		HeapInuse:  ms.HeapInuse,
		GoSys:      ms.Sys,
		NumGC:      ms.NumGC,
		Goroutines: runtime.NumGoroutine(),
	}
	if l := debug.SetMemoryLimit(-1); l != math.MaxInt64 {
		i.MemLimit = uint64(l)
	}
	if ms.LastGC > 0 {
		i.LastGC = time.Unix(0, int64(ms.LastGC))
		i.LastPause = time.Duration(ms.PauseNs[(ms.NumGC+255)%256])
	}

	i.RSS = statusKB("/proc/self/status", "VmRSS")
	i.CgroupCurrent = fileUint("/sys/fs/cgroup/memory.current")
	i.CgroupMax = fileUint("/sys/fs/cgroup/memory.max")
	i.OOMKills = int(keyed("/sys/fs/cgroup/memory.events", "oom_kill", 1))
	i.HostTotal = statusKB("/proc/meminfo", "MemTotal")
	i.HostAvailable = statusKB("/proc/meminfo", "MemAvailable")
	return i
}

// fileUint reads a file holding one number, 0 when missing or "max".
func fileUint(path string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return n
}

// statusKB reads a "Key: N kB" line, in bytes.
func statusKB(path, key string) uint64 {
	return keyed(path, key+":", 1024)
}

// keyed reads the number after key on its line of path, times unit.
func keyed(path, key string, unit uint64) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) >= 2 && fields[0] == key {
			n, _ := strconv.ParseUint(fields[1], 10, 64)
			return n * unit
		}
	}
	return 0
}
