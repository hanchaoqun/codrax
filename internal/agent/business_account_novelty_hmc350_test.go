package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC350NoveltyIsExactContentNotQueryOrAuthorityIdentity(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700, "time_start": 5.0, "time_end": 5.111487})
	a := hmc325FullAccounts(answerDocObservationLedger(ctx))[0]
	a.key = traceEventPublicationRecordKey(a.record)
	want := answerDocBusinessAccountNoveltyKey(a)
	if want == "" {
		t.Fatal("current native account has no display preference key")
	}
	clone := func() answerDocBusinessAccount {
		raw, _ := json.Marshal(a.record)
		b := a
		if err := json.Unmarshal(raw, &b.record); err != nil {
			t.Fatal(err)
		}
		return b
	}
	b := clone()
	b.record.ID += "-another-publication"
	b.record.ObservedAt += "-later-observation"
	b.record.SourceRef.QueryScopeID += "-another-query"
	b.record.SourceRef.ToolCallID += "-another-call"
	b.record.SourceRef.RawRef += "-another-text-output"
	if answerDocBusinessAccountNoveltyKey(b) != want || traceEventPublicationRecordKey(a.record) == traceEventPublicationRecordKey(b.record) {
		t.Fatal("equal display content merged independent original record identities")
	}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"physical_path": func(r *types.ObservationRecord) { r.SourceRef.Path += "-other" },
		"capture":       func(r *types.ObservationRecord) { r.SourceRef.CaptureIdentityPath += "-other" },
		"artifact":      func(r *types.ObservationRecord) { r.SourceRef.ArtifactID += "-other" },
		"payload":       func(r *types.ObservationRecord) { r.SourceRef.PayloadRef += "-other" },
		"clock":         func(r *types.ObservationRecord) { r.SourceRef.ClockAlignment += "-other" },
		"window":        func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs += 1 },
		"target":        func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID++ },
		"scope":         func(r *types.ObservationRecord) { r.SourceRef.QueryTargetScope += "-other" },
		"line_filter":   func(r *types.ObservationRecord) { r.SourceRef.QueryLineEnd++ },
		"unit":          func(r *types.ObservationRecord) { r.Unit += "-other" },
		"value":         func(r *types.ObservationRecord) { r.Value += "-other" },
		"span":          func(r *types.ObservationRecord) { r.Span.EndTs++ },
		"support":       func(r *types.ObservationRecord) { r.SupportRefs = append(r.SupportRefs, "different source coordinate") },
		"full_note":     func(r *types.ObservationRecord) { r.RichNotes = append(r.RichNotes, "unshown precision/coverage note") },
	} {
		t.Run(name, func(t *testing.T) {
			b := clone()
			mutate(&b.record)
			if answerDocBusinessAccountNoveltyKey(b) == want {
				t.Fatal("different complete source/scope/content got the same display preference")
			}
		})
	}
}
