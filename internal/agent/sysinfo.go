package agent

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

// maxReportedPorts caps the listening-port list so a busy host can't bloat the
// status payload.
const maxReportedPorts = 2000

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

// collectListeningPorts returns the sorted, de-duplicated set of TCP listening
// and UDP bound local ports on the host, parsed from /proc/net (Linux only;
// returns nil elsewhere). This is a point-in-time snapshot, not a live scan.
func collectListeningPorts() []int {
	seen := map[int]bool{}
	// TCP sockets in LISTEN state (st field == "0A").
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		for _, p := range parseProcNet(f, "0A") {
			seen[p] = true
		}
	}
	// UDP sockets are connectionless; any entry with a bound local port counts.
	for _, f := range []string{"/proc/net/udp", "/proc/net/udp6"} {
		for _, p := range parseProcNet(f, "") {
			seen[p] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	if len(out) > maxReportedPorts {
		out = out[:maxReportedPorts]
	}
	return out
}

// parseProcNet parses local ports from a /proc/net/{tcp,udp}* table. If wantState
// is non-empty, only rows whose state column equals it are included.
func parseProcNet(path, wantState string) []int {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var ports []int
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 { // header
			continue
		}
		fields := strings.Fields(line)
		// Layout: sl local_address rem_address st ...
		if len(fields) < 4 {
			continue
		}
		if wantState != "" && fields[3] != wantState {
			continue
		}
		local := fields[1] // "HEXIP:HEXPORT"
		colon := strings.LastIndexByte(local, ':')
		if colon < 0 {
			continue
		}
		port, err := strconv.ParseInt(local[colon+1:], 16, 32)
		if err != nil || port <= 0 {
			continue
		}
		ports = append(ports, int(port))
	}
	return ports
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
