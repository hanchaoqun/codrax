package tool

import (
	"fmt"
	"strconv"
	"strings"
)

// Keep whole native intervals, prioritizing both retained boundaries before
// interior rows. Coordinates describe a preview, never a replacement population.
func traceQueryCompactTimeline(key, unit string, count, omitted int, interval func(int) string) string {
	render := func(indices []int) (string, bool) {
		var parts, rows []string
		for _, index := range indices {
			parts = append(parts, interval(index))
			rows = append(rows, strconv.Itoa(index))
		}
		body := strings.Join(parts, ";")
		value := fmt.Sprintf("seconds:%s; omitted=%d; sample_rows0=%s; %s", unit,
			omitted+count-len(indices), strings.Join(rows, ","), body)
		return value, len(body) <= 90 && len(key+"="+value) <= 160
	}
	var indices []int
	if count > 0 {
		indices = []int{0}
		if _, fits := render(indices); !fits {
			indices = nil
		}
	}
	last := count - 1
	if last > 0 && len(indices) > 0 {
		if _, fits := render([]int{0, last}); fits {
			indices = append(indices, last)
		}
	}
	for next := 1; next < last && len(indices) > 0; next++ {
		candidate := append([]int(nil), indices...)
		if candidate[len(candidate)-1] == last {
			candidate = append(candidate[:len(candidate)-1], next, last)
		} else {
			candidate = append(candidate, next)
		}
		if _, fits := render(candidate); !fits {
			break
		}
		indices = candidate
	}
	value, _ := render(indices)
	return value
}
