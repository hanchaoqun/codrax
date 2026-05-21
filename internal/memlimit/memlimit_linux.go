//go:build linux

package memlimit

import (
	"os"
	"strconv"
	"strings"
)

// systemTotalMemory reads total host RAM from /proc/meminfo's MemTotal
// line (reported in kibibytes).
func systemTotalMemory() (uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		// Expected shape: "MemTotal:  3902345 kB"
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || kb == 0 {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
