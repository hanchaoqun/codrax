package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProcessMeasureEventInventoryPublicSourceTimeIsNotSortingZero(t *testing.T) {
	integer := func(v int64) tracewire.ProcessMeasureScalar {
		return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: strconv.FormatInt(v, 10)}
	}
	absent := tracewire.ProcessMeasureScalar{Status: "unavailable", StorageClass: "absent"}
	cases := []struct {
		name  string
		start tracewire.ProcessMeasureScalar
		known bool
		value float64
	}{
		{"null", tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}, false, 0},
		{"absent", absent, false, 0},
		{"numeric_text", tracewire.ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "text", Value: "0"}, false, 0},
		{"numeric_real", tracewire.ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "real", Value: "0"}, false, 0},
		{"negative", integer(-1), true, -0.000000001},
		{"actual_zero", integer(0), true, 0},
		{"positive", integer(1000000001), true, 1.000000001},
	}
	var source strings.Builder
	for i, tc := range cases {
		record := tracewire.ProcessMeasureInterval{RowID: int64(i + 1), FilterID: integer(1), StartNS: tc.start,
			DurationNS: integer(0), Value: integer(0), IPID: absent, OwnerStatus: "unknown", Name: tc.name, NameKnown: true}
		line, err := tracewire.FormatProcessMeasureInterval(record)
		if err != nil {
			t.Fatal(err)
		}
		source.WriteString(line + "\n")
	}
	// An ordinary physical timestamp of zero remains known: the correction
	// applies only to the new carrier's exact typed source-time receipt.
	source.WriteString("worker-101 (101) [001] .... 0.000000000: cpu_frequency: state=900000 cpu_id=1\n")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "measure.systrace"), []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"path":"measure.systrace","view":"event_search","limit":20}`))
	if err != nil || !result.Success {
		t.Fatalf("query failed: %v %s", err, result.Summary)
	}
	inv := requireEventSearchInventory(t, result).EventSearchInventory
	if len(inv.Rows) != len(cases)+1 {
		t.Fatalf("rows=%d, want %d", len(inv.Rows), len(cases)+1)
	}
	for _, row := range inv.Rows {
		if row.LocalLine == len(cases)+1 {
			if !row.SourceTimeKnown || row.SourceTimeSeconds != 0 {
				t.Fatal("ordinary physical zero lost source time", row)
			}
			continue
		}
		tc := cases[row.LocalLine-1]
		if row.SourceTimeKnown != tc.known || row.SourceTimeSeconds != tc.value {
			t.Errorf("%s: sorting coordinate acquired source-time authority: %+v", tc.name, row)
		}
		if row.EmitterTIDKnown == nil || *row.EmitterTIDKnown || row.CPUKnown == nil || *row.CPUKnown {
			t.Errorf("%s: process observation acquired emitter or CPU", tc.name)
		}
	}
	// A selected nonnegative time window cannot admit an unknown source
	// timestamp or a negative source row merely because its sort key is zero.
	result, err = (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"path":"measure.systrace","view":"event_search","time_start":0,"time_end":2,"limit":20}`))
	if err != nil || !result.Success {
		t.Fatalf("window query failed: %v %s", err, result.Summary)
	}
	window := requireEventSearchInventory(t, result).EventSearchInventory
	if len(window.Rows) != 3 {
		t.Fatalf("selected window retained unpositioned or negative rows: %+v", window.Rows)
	}
	for _, row := range window.Rows {
		if !row.SourceTimeKnown || row.SourceTimeSeconds < 0 || row.LocalLine <= 5 {
			t.Fatalf("sorting substitute admitted as window evidence: %+v", row)
		}
	}
}
