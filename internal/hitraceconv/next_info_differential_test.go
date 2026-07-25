package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// next_info_differential_test.go — AUD-05(3) (§14.6, colleague audit
// 2026-07-25): the converter text lane and the direct systrace parse lane
// must yield the SAME Event semantics for one next_info payload — the
// pre-fix converter enforced packed-bit-field ranges on the text lane and
// dropped whole tokens (group=4, cgid=32) that the direct lane kept as
// doc-legitimate unknown extensions.

func nextInfoEventFromDirectLine(t *testing.T, payload string) tracequery.Event {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "diff.systrace")
	line := "        app-20   (   20) [001] .... 1.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53 next_info=" + payload + " cg=top-app\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range idx.Events {
		if ev.NextInfo == payload {
			return ev
		}
	}
	t.Fatalf("direct lane did not parse next_info=%q", payload)
	return tracequery.Event{}
}

func TestNextInfoConverterDirectLaneParity(t *testing.T) {
	for _, tc := range []struct {
		payload string
		check   func(t *testing.T, ev tracequery.Event)
	}{
		// Doc-legitimate unknown extension: sched_group ≥4 keeps its digits
		// on BOTH lanes (the converter used to drop the whole token).
		{"f,10,4,1,3", func(t *testing.T, ev tracequery.Event) {
			if ev.NextInfoGroup != 4 || !ev.NextInfoRich().NextInfoGroupKnown {
				t.Fatalf("group=4 must survive as a known extension value: %+v", ev)
			}
		}},
		// Out-of-doc boost=2: legacy restricted fill stays true (V1 frozen
		// bug-compat) while the semantic ices_boost claim withdraws.
		{"f,10,2,2,3", func(t *testing.T, ev tracequery.Event) {
			if !ev.NextInfoRestricted {
				t.Fatalf("boost=2 keeps the bug-compatible restricted fill: %+v", ev)
			}
			if ev.NextInfoRich().NextInfoBoostKnown {
				t.Fatalf("boost=2 is outside the doc closed set — the semantic claim must withdraw: %+v", ev)
			}
		}},
		// cgid beyond the packed 5-bit width is a text-lane extension too.
		{"f,10,2,1,3,32", func(t *testing.T, ev tracequery.Event) {
			if ev.NextInfoCGID != 32 || !ev.NextInfoRich().NextInfoCGIDKnown {
				t.Fatalf("cgid=32 must survive the text lane: %+v", ev)
			}
		}},
	} {
		canonical := canonicalHarmonySchedInfoText(tc.payload, strings.Count(tc.payload, ",") >= 5)
		if canonical != tc.payload {
			t.Fatalf("converter text lane must preserve %q losslessly, got %q", tc.payload, canonical)
		}
		tc.check(t, nextInfoEventFromDirectLine(t, canonical))
	}
	// Malformed sibling still fails the whole token closed on the converter
	// lane (lexical validation is not relaxed).
	if got := canonicalHarmonySchedInfoText("f,1x,2,1,3", false); got != "" {
		t.Fatalf("lexically-invalid payloads must still fail closed: %q", got)
	}
}
