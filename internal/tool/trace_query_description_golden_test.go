package tool

// trace_query_description_golden_test.go — PIN-1 B5 (§29.65 回归口, 2026-07-13):
// byte-golden snapshot of dispatch-sensitive tool Descriptions.
//
// EVOLUTION RECORD / why this exists (§29.64 h3 归因关账, 教训级): the
// trace_query Description is an LLM-FACING DISPATCH SURFACE — inserting one
// teaching sentence mid-Description flipped the h3 golden case 6-green →
// 2/2 red (the model stopped dispatching any rank view), and the root fix was
// Description delta ZERO (byte identity with baseline; the new note-key
// teaching moved to the data-side Summary/legend). The pre-existing substring
// pins (TestTraceQuerySchemaDocumentsViews etc.) cannot see a mid-section
// insertion or reorder, so the §29.64 root-cause shape could silently
// reappear. This golden makes any Description byte change a DELIBERATE act.
//
// UPDATE RITUAL (deliberate gate — do NOT casually regenerate):
//  1. justify the wording change against §29.64 (new note-key teaching goes
//     to the wire Summary/legend, NOT mid-Description; R2' description-slot
//     exemptions must be recorded);
//  2. regenerate via
//     PIN1_UPDATE_DESCRIPTION_GOLDEN=1 go test ./internal/tool/ -run TestToolDescriptionByteGolden
//  3. commit the golden WITH an EVOLUTION RECORD note naming the delta and,
//     for trace_query, weigh a golden-eval A/B (h2/h3 are the proven
//     dispatch-variance-sensitive cases).

import (
	"os"
	"testing"
)

func TestToolDescriptionByteGolden(t *testing.T) {
	cases := []struct {
		name   string
		golden string
		got    string
	}{
		// trace_query is THE proven dispatch-sensitive Description (§29.64
		// h3). Add further trace-facing tools here as they earn evidence of
		// dispatch sensitivity — one golden per tool.
		{"trace_query", "testdata/trace_query_description.golden", (&TraceQuery{}).Description()},
	}
	for _, tc := range cases {
		if os.Getenv("PIN1_UPDATE_DESCRIPTION_GOLDEN") == "1" {
			if err := os.WriteFile(tc.golden, []byte(tc.got), 0o644); err != nil {
				t.Fatalf("%s: update golden: %v", tc.name, err)
			}
			t.Logf("%s: golden regenerated — record an EVOLUTION RECORD in the commit (§29.64)", tc.name)
			continue
		}
		want, err := os.ReadFile(tc.golden)
		if err != nil {
			t.Fatalf("%s: read golden: %v", tc.name, err)
		}
		if tc.got == string(want) {
			continue
		}
		// Locate the first divergent byte for a useful failure message.
		i := 0
		for i < len(tc.got) && i < len(want) && tc.got[i] == want[i] {
			i++
		}
		lo := i - 80
		if lo < 0 {
			lo = 0
		}
		hiGot, hiWant := i+80, i+80
		if hiGot > len(tc.got) {
			hiGot = len(tc.got)
		}
		if hiWant > len(want) {
			hiWant = len(want)
		}
		t.Fatalf("%s: Description drifted from its byte golden at byte %d.\n"+
			"Description is a DISPATCH-SENSITIVE LLM surface (§29.64 h3: one mid-section sentence = 2/2 golden red).\n"+
			"If the change is deliberate, follow the UPDATE RITUAL in this file's header.\n"+
			"--- got  around byte %d ---\n%q\n--- want around byte %d ---\n%q",
			tc.name, i, i, tc.got[lo:hiGot], i, string(want[lo:hiWant]))
	}
}
