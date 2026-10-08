package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestNativeResourceStackStreamPublicLargeWindowAndBundle(t *testing.T) {
	src, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	_, _, native := nativeStackPublicQuery(t, src, map[string]any{"pid": 101, "time_start": 10, "time_end": 10.05})
	var rows []string
	for _, e := range native.Events[:2] {
		wire := tracewire.ResourceStackRecord{TimestampNS: e.TimestampNS, EventRowID: e.RowID, Event: &e.Source}
		line, ok := tracewire.FormatResourceStack(wire)
		if !ok {
			t.Fatal("invalid fixture event")
		}
		rows = append(rows, line+"\n")
		for _, f := range e.Frames {
			wire.Event, wire.Frame = nil, &f.ResourceFrame
			line, ok = tracewire.FormatResourceStack(wire)
			if !ok {
				t.Fatal("invalid fixture frame")
			}
			rows = append(rows, line+"\n")
		}
	}
	paths := make([]string, 2)
	for i, width := range []int{8, 8192} {
		paths[i] = filepath.Join(t.TempDir(), "events.systrace")
		f, err := os.Create(paths[i])
		if err != nil {
			t.Fatal(err)
		}
		// The event header and its frames straddle >64MiB of unrelated text.
		// Line coordinates are identical to the small file; only padding width
		// differs. A fixed-padding/windowed index must not stand in for EOF.
		if _, err := f.WriteString(rows[0]); err != nil {
			t.Fatal(err)
		}
		padding := "#" + strings.Repeat(" ", width-2) + "\n"
		for n := 0; n < 8193; n++ {
			if _, err := f.WriteString(padding); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.WriteString(strings.Join(rows[1:], "")); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	info, _ := os.Stat(paths[1])
	if info.Size() <= 64<<20 {
		t.Fatal("fixture does not enter the large-file routing class")
	}
	var baseline tracequery.ResourceStackResult
	for i, path := range paths {
		r, _, p := nativeStackPublicQuery(t, path, map[string]any{"pid": 101, "time_start": 10, "time_end": 10.05})
		if p.MatchedEvents != 2 || p.Events[0].OmittedFrames != 0 || !p.Events[0].SourceFramesComplete || len(p.Events[0].Frames) != p.Events[0].Source.FrameCount || strings.Contains(r.Summary, "parse_diagnostic=zero_events") {
			t.Fatalf("large window lost complete source binding: %+v %s", p, r.Summary)
		}
		p.SourcePath = ""
		if i == 0 {
			baseline = p
		} else if !reflect.DeepEqual(p, baseline) {
			t.Fatalf("file size changed resource facts: small=%+v large=%+v", baseline, p)
		}
	}
	bundle := filepath.Join(filepath.Dir(paths[1]), "capture.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, bundle, []byte(`{"systrace":"events.systrace","artifacts":[{"type":"systrace","path":"events.systrace"}]}`))
	_, record, bundled := nativeStackPublicQuery(t, bundle, map[string]any{"pid": 101, "time_start": 10, "time_end": 10.05})
	resolvedBundle, _ := filepath.EvalSymlinks(bundle)
	if bundled.SourcePath != resolvedBundle || record.SourceRef.Path != resolvedBundle {
		t.Fatal("verified source universe silently unwrapped to a child")
	}
	bundled.SourcePath = ""
	if !reflect.DeepEqual(bundled, baseline) {
		t.Fatal("verified identity bundle changed complete source facts")
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	dead, stop := context.WithCancel(context.Background())
	stop()
	ctx.Ctx = dead
	args, _ := json.Marshal(map[string]any{"source": "path", "path": paths[1], "view": "resource_stack", "time_start": 10, "time_end": 10.05})
	canceled, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || canceled.Success || canceled.TraceViewCancellation == nil || canceled.TraceViewCancellation.Reason != "canceled" || len(canceled.Observations) != 0 || canceled.RawRef != "" {
		t.Fatalf("public cancellation published evidence or unavailable success: %v %+v", err, canceled)
	}
}
