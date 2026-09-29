package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestExactTraceMarkWireRoundTripPreservesPipeNameAndCPU(t *testing.T) {
	mark := ExactTraceMark{
		TimestampNS: 1_234_567_890,
		CPU:         7,
		TID:         33410,
		TGID:        69326,
		SpanPID:     32788,
		Action:      "S",
		Comm:        "ss.hm.ugc.aweme",
		Name:        "H:Task|Frame|42",
		Value:       "cookie|value",
	}
	line, err := FormatExactTraceMark(mark)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, mark.Name) || strings.Contains(line, "[007]") ||
		!strings.HasPrefix(line, exactTraceMarkPrefix+" ") {
		t.Fatalf("exact wire leaked ambiguous physical syntax: %q", line)
	}
	ts, ok := ParseLineTimestampNS(line)
	if !ok || ts != mark.TimestampNS {
		t.Fatalf("timestamp parse=(%d,%t) want (%d,true)", ts, ok, mark.TimestampNS)
	}
	ev, ok := ParseLine(7, line, newStringInterner())
	if !ok || ev.Type != EventTraceMark || ev.Line != 7 || ev.CPU != mark.CPU ||
		ev.PID != mark.TID || ev.TGID != mark.TGID || ev.SpanPID != mark.SpanPID ||
		ev.SpanAction != mark.Action || ev.SpanName != mark.Name || ev.SpanValue != mark.Value ||
		ev.PluginFields != nil {
		t.Fatalf("exact marker round-trip drifted: %+v", ev)
	}
}

func TestExactTraceMarkNameOriginCannotOverrideMarkerIdentity(t *testing.T) {
	ref := int64(0)
	mark := ExactTraceMark{TimestampNS: 1000000100, CPU: 1, TID: 100, TGID: 100, SpanPID: 100, Action: "B", Comm: "app", Name: "AppStartup:startup", NameOrigin: &tracewire.MarkerNameOrigin{SourceTable: "app_startup", Name: tracewire.HiSysEventName{Status: "unresolved_reference", Reference: &ref}}}
	line, err := FormatExactTraceMark(mark)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := ParseLine(1, line, newStringInterner())
	if !ok || ev.MarkerNameOrigin == nil || ev.MarkerNameOrigin.Name.Reference == nil || *ev.MarkerNameOrigin.Name.Reference != 0 || ev.CPU != 1 || ev.PID != 100 {
		t.Fatal("origin lost or changed marker identity")
	}
	if ts, ok := ParseLineTimestampNS(line); !ok || ts != mark.TimestampNS {
		t.Fatal("exact timestamp lost")
	}
	for _, bad := range []string{
		strings.Replace(line, "name=QXBwU3RhcnR1cDpzdGFydHVw", "name=b3RoZXI", 1),
		strings.Replace(line, "action=B", "action=S", 1),
		strings.Replace(line, exactTraceMarkOriginPrefix, exactTraceMarkPrefix, 1),
		strings.Split(line, " name_origin=")[0], line + " extra=x",
		line + " name_origin=~",
	} {
		if _, ok := ParseLine(1, bad, newStringInterner()); ok {
			t.Fatalf("accepted inconsistent v2 wire: %q", bad)
		}
	}
	without := ev
	without.PluginFields = nil
	if eventSideTableBytes(&ev) <= eventSideTableBytes(&without) {
		t.Fatal("retained origin omitted from cache accounting")
	}
}

func TestExactTraceMarkRejectsNonCanonicalOrOpenEndedWire(t *testing.T) {
	base, err := FormatExactTraceMark(ExactTraceMark{
		TimestampNS: 10,
		CPU:         0,
		TID:         11,
		TGID:        12,
		SpanPID:     13,
		Action:      "B",
		Comm:        "worker",
		Name:        "name|with|pipe",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		strings.Replace(base, "action=B", "action=X", 1),
		strings.Replace(base, "cpu=0", "cpu=4096", 1),
		strings.Replace(base, "cpu=0", "cpu=+0", 1),
		strings.Replace(base, "tid=11", "tid=011", 1),
		strings.Replace(base, "comm=d29ya2Vy", "comm=d29ya2Vy=", 1),
		strings.Replace(base, " ts_ns=10", "  ts_ns=10", 1),
		base + " extra=x",
		strings.Replace(base, "name=bmFtZXx3aXRofHBpcGU", "name=~", 1),
	} {
		if _, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("accepted non-canonical exact wire row: %q", line)
		}
	}
}

func TestExactTraceMarkBuildIndexReconstructsPipeNamedSpan(t *testing.T) {
	begin, err := FormatExactTraceMark(ExactTraceMark{
		TimestampNS: 1_000_000_100,
		CPU:         2,
		TID:         101,
		TGID:        100,
		SpanPID:     37722,
		Action:      "B",
		Comm:        "worker",
		Name:        "layout|pipeline",
	})
	if err != nil {
		t.Fatal(err)
	}
	end, err := FormatExactTraceMark(ExactTraceMark{
		TimestampNS: 1_005_000_900,
		CPU:         3,
		TID:         101,
		TGID:        100,
		SpanPID:     37722,
		Action:      "E",
		Comm:        "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "exact-pipe-name.systrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\n"+begin+"\n"+end+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 0.9, TimeEnd: 1.1}, 16)
	if len(spans) != 1 || spans[0].Name != "layout|pipeline" || spans[0].SpanPID != 37722 ||
		math.Abs(spans[0].DurationMs-5.0008) > 1e-9 || spans[0].CPUStatus != "" {
		t.Fatalf("exact pipe span was not reconstructed: spans=%+v caveats=%+v", spans, caveats)
	}
}
