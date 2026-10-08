package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func resourceSQLiteFixture(t *testing.T, extra ...string) (string, *tracequery.Index) {
	t.Helper()
	sql, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := createTraceDBFixture(t, append([]string{string(sql)}, extra...))
	out := filepath.Join(t.TempDir(), "resource.systrace")
	if _, err := exportTraceDBToSystrace(context.Background(), path, out); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	return out, idx
}

func TestNativeResourceStackSQLiteOwnerUnknownCPUAndSourceQuality(t *testing.T) {
	out, idx := resourceSQLiteFixture(t)
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 101, TimeStart: 10, TimeEnd: 10.05}).ResourceStack
	if !tracequery.ValidResourceStack(*p) || p.Status != "available" || p.MatchedEvents != 3 {
		t.Fatalf("bad resource stack: %+v", p)
	}
	if p.Events[0].DepthStatus != "contiguous_observed" || p.Events[0].Source.FrameCount != 3 || p.Events[0].Frames[0].IP.Value != "-1" || p.Events[0].Frames[0].Symbol.Value != "malloc" {
		t.Fatalf("lost ordinary source stack: %+v", p.Events[0])
	}
	if p.Events[1].MissingDepths != 1 || p.Events[1].UnknownSymbols != 1 || p.Events[1].Frames[1].VAddr.Status != "null" {
		t.Fatalf("gap/null lost %+v", p.Events[1])
	}
	if p.Events[2].DuplicateDepths != 1 || p.Events[2].DepthStatus != "ambiguous" || p.Events[2].Frames[2].Symbol.Status != "ambiguous" {
		t.Fatalf("duplicate chose winner %+v", p.Events[2])
	}
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventResourceStack && ev.CPU != -1 {
			t.Fatal("fabricated resource CPU")
		}
		if ev.Type == tracequery.EventPerfSample {
			t.Fatal("resource stack became execution sample")
		}
	}
	body, _ := os.ReadFile(out)
	if strings.Contains(string(body), "NativeHook:") {
		t.Fatal("no scheduler witness must not bypass legacy I/C gate")
	}
}

func TestNativeResourceStackSQLiteSignedPhysicalIdentity(t *testing.T) {
	_, idx := resourceSQLiteFixture(t, "UPDATE native_hook SET rowid=0 WHERE id=1", "UPDATE native_hook SET rowid=-9223372036854775808 WHERE id=2", "UPDATE native_hook SET rowid=9223372036854775807 WHERE id=3", "UPDATE native_hook_frame SET rowid=0 WHERE id=1", "UPDATE native_hook_frame SET rowid=-9223372036854775808 WHERE id=2", "UPDATE native_hook_frame SET rowid=9223372036854775807 WHERE id=3")
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 101, TimeStart: 10, TimeEnd: 10.05}).ResourceStack
	if !tracequery.ValidResourceStack(*p) || p.MatchedEvents != 3 || p.Events[0].RowID != 0 || p.Events[1].RowID != -9223372036854775808 || p.Events[2].RowID != 9223372036854775807 {
		t.Fatalf("signed event identities lost: %+v", p)
	}
	for i, want := range []int64{0, -9223372036854775808, 9223372036854775807} {
		if p.Events[0].Frames[i].RowID != want {
			t.Fatalf("signed frame row lost %+v", p.Events[0].Frames)
		}
	}
}

func TestNativeResourceStackSQLiteMissingFramesOwnerAndUnresolvedKeys(t *testing.T) {
	for _, tc := range []struct {
		sql, status string
		complete    bool
	}{{"DROP TABLE native_hook_frame", "frame_table_unavailable", false}, {"DELETE FROM native_hook_frame", "no_frames", true}, {"UPDATE native_hook SET callchain_id=-1", "unresolved_callchain", false}, {"UPDATE native_hook SET callchain_id=NULL", "unresolved_callchain", false}} {
		t.Run(tc.status, func(t *testing.T) {
			_, idx := resourceSQLiteFixture(t, tc.sql)
			p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 101, TimeStart: 10, TimeEnd: 10.05}).ResourceStack
			if !tracequery.ValidResourceStack(*p) || p.MatchedEvents != 3 {
				t.Fatalf("metadata event lost %+v", p)
			}
			for _, e := range p.Events {
				if e.Source.StackStatus != tc.status || e.SourceFramesComplete != tc.complete {
					t.Fatalf("unknown represented complete %+v", e)
				}
			}
		})
	}
	_, idx := resourceSQLiteFixture(t, "UPDATE native_hook SET ipid=2 WHERE id=1", "UPDATE native_hook SET itid=999 WHERE id=2")
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 101, TimeStart: 10, TimeEnd: 10.05}).ResourceStack
	if !tracequery.ValidResourceStack(*p) || p.MatchedEvents != 1 || p.Events[0].Source.SourceID.Value != "3" {
		t.Fatalf("unowned event got stack %+v", p)
	}
}
