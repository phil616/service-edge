package agent

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

// collectSystemInfo gathers host info (Linux: from /proc, falling back to zero).
func collectSystemInfo() protocol.SystemInfo {
	info := protocol.SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
	info.Kernel = readKernel()
	info.MemoryMB = readMemTotalMB()
	info.UptimeS = readUptime()
	return info
}

func readKernel() string {
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func readMemTotalMB() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseUint(fields[1], 10, 64)
				return kb / 1024
			}
		}
	}
	return 0
}

func readUptime() uint64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 1 {
		secs, _ := strconv.ParseFloat(fields[0], 64)
		return uint64(secs)
	}
	return 0
}
