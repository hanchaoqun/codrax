package types

import "testing"

// SELFRUN-DISC (§29.192① (b), 2026-07-21): parser-layer fail-closed battery
// for the self_running_fold_unmeasured side-channel record (the PARTSPLIT
// per-arm negative-arm discipline — every fail-closed arm carries its own
// red). The identity arm is load-bearing: a record whose running exceeds its
// unknown wall is a PARTIALLY-known basis, exactly the shape this absence
// disclosure must never claim.
func selfrunDiscRecord(mutate func(map[string]string)) ObservationRecord {
	notes := map[string]string{
		TraceNoteKeySelfRunningFoldUnmeasuredRunningMS: "19.800",
		TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS: "19.800",
	}
	if mutate != nil {
		mutate(notes)
	}
	rich := make([]string, 0, len(notes))
	for k, v := range notes {
		if v != "" {
			rich = append(rich, k+"="+v)
		}
	}
	return ObservationRecord{Subject: "app-100", RichNotes: rich}
}

func TestSelfRunningFoldUnmeasuredParserFailClosedArms(t *testing.T) {
	if out, ok := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(selfrunDiscRecord(nil)); !ok {
		t.Fatalf("positive control must parse, got drop")
	} else if out.Subject != "app-100" || out.RunningMS != 19.8 || out.UnknownMS != 19.8 {
		t.Fatalf("positive control fields wrong: %+v", out)
	}
	cases := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"identity broken beyond 2µs headroom (partially-known basis)", func(n map[string]string) {
			n[TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS] = "19.700"
		}},
		{"running note absent", func(n map[string]string) {
			n[TraceNoteKeySelfRunningFoldUnmeasuredRunningMS] = ""
		}},
		{"unknown note absent", func(n map[string]string) {
			n[TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS] = ""
		}},
		{"non-positive running", func(n map[string]string) {
			n[TraceNoteKeySelfRunningFoldUnmeasuredRunningMS] = "0.000"
			n[TraceNoteKeySelfRunningFoldUnmeasuredUnknownMS] = "0.000"
		}},
		{"unparseable value", func(n map[string]string) {
			n[TraceNoteKeySelfRunningFoldUnmeasuredRunningMS] = "n/a"
		}},
	}
	for _, tc := range cases {
		if _, ok := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(selfrunDiscRecord(tc.mutate)); ok {
			t.Fatalf("%s: record must drop whole (all-or-nothing), got parsed", tc.name)
		}
	}
	// Empty subject drops whole regardless of the value pair.
	blank := selfrunDiscRecord(nil)
	blank.Subject = "  "
	if _, ok := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(blank); ok {
		t.Fatalf("empty subject must drop whole")
	}
}

// The compile routes the predicate past node classification into the side
// list (deduped by subject — a re-published record set cannot double the
// list), and a records set without the predicate compiles to an empty list
// (absence silent). The projection needs one node record to be Active() —
// the side channel never activates a projection by itself (sibling
// discipline: no seat, no node, no ordinal).
func TestSelfRunningFoldUnmeasuredCompileSideChannel(t *testing.T) {
	seat := ObservationRecord{
		ID: "trace_query:t#root_cause_primary:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:sleep",
		Subject: "dep-200", Object: "sleep_wait", Value: "12.000", Unit: "ms",
		Span:      ObservationSpan{LineStart: 10, LineEnd: 20},
		RichNotes: []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "dominant_state=s_sleep"},
	}
	base := selfrunDiscRecord(nil)
	base.Origin = AnswerEvidenceOriginRuntimeArtifact
	base.Producer = "trace_query"
	base.GroundingPolicy = ClaimGroundingHard
	base.Predicate = "self_running_fold_unmeasured"
	dup := base
	proj := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{seat, base, dup}})
	if len(proj.SelfRunningFoldUnmeasured) != 1 {
		t.Fatalf("one deduped side-channel row expected, got %+v", proj.SelfRunningFoldUnmeasured)
	}
	if d := proj.SelfRunningFoldUnmeasured[0]; d.Subject != "app-100" || d.RunningMS != 19.8 || d.UnknownMS != 19.8 {
		t.Fatalf("compiled row drifted: %+v", d)
	}
	// Absence arm: the same active set WITHOUT the disclosure record.
	noDisc := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{seat}})
	if len(noDisc.SelfRunningFoldUnmeasured) != 0 {
		t.Fatalf("absence must compile silent, got %+v", noDisc.SelfRunningFoldUnmeasured)
	}
}
