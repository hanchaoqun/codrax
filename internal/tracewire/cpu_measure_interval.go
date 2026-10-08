package tracewire

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const CPUMeasureIntervalPrefix = "# codrax_cpu_measure_interval/v1"

// CPUMeasureInterval preserves a SQL measure row, not a continuous ftrace
// control. Unknown duration never borrows a next row or the trace endpoint.
// Native idle values are opaque codes; their encoding is not ftrace cpu_idle.
type CPUMeasureInterval struct {
	RowID      int64  `json:"row_id,string"`
	FilterID   int64  `json:"filter_id,string"`
	CPU        int    `json:"cpu"`
	Kind       string `json:"kind"`
	Encoding   string `json:"encoding"`
	StartNS    *int64 `json:"start_ns,string,omitempty"`
	DurationNS *int64 `json:"duration_ns,string,omitempty"`
	Value      *int64 `json:"value,string,omitempty"`
	Issue      string `json:"issue,omitempty"`
}

func (r CPUMeasureInterval) TimestampNS() int64 {
	if r.StartNS == nil || *r.StartNS < 0 {
		return 0
	}
	return *r.StartNS
}

func (r CPUMeasureInterval) Valid() bool {
	if r.FilterID < 0 || r.CPU < 0 || r.CPU > 4095 {
		return false
	}
	if r.Kind == "idle" {
		if r.Encoding != "native_sql_idle" {
			return false
		}
	} else if r.Kind != "frequency" || r.Encoding != "khz" {
		return false
	}
	if r.DurationNS != nil && *r.DurationNS < 0 {
		return false
	}
	if r.Value != nil && (*r.Value < 0 || *r.Value > math.MaxUint32) {
		return false
	}
	if r.StartNS != nil && r.DurationNS != nil && *r.StartNS > math.MaxInt64-*r.DurationNS {
		return false
	}
	switch r.Issue {
	case "":
		return r.StartNS != nil && r.DurationNS != nil && r.Value != nil
	case "unknown_duration":
		return r.StartNS != nil && r.DurationNS == nil
	case "invalid_duration":
		return r.DurationNS == nil
	case "invalid_timestamp":
		return r.StartNS == nil
	case "invalid_value":
		return r.Value == nil
	case "invalid_filter_id":
		return true
	default:
		return false
	}
}

func FormatCPUMeasureInterval(r CPUMeasureInterval) (string, error) {
	if !r.Valid() {
		return "", fmt.Errorf("invalid CPU measure interval")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return CPUMeasureIntervalPrefix + " record=" + base64.RawURLEncoding.EncodeToString(b), nil
}

func ParseCPUMeasureInterval(line string) (CPUMeasureInterval, bool) {
	prefix := CPUMeasureIntervalPrefix + " record="
	if !strings.HasPrefix(line, prefix) || len(line) > 1024 {
		return CPUMeasureInterval{}, false
	}
	var r CPUMeasureInterval
	b, err := base64.RawURLEncoding.Strict().DecodeString(line[len(prefix):])
	if err != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	canonical, err := FormatCPUMeasureInterval(r)
	return r, err == nil && canonical == line
}
