package types

// HMC377 E3 projection-compile pins: the three optional business-clue notes
// (raw wall beside a chain member's in-window part; an own phase's self time
// and scheduler split) parse strictly when present and leave older records
// valid when absent; the budgeted selection keeps the analysed thread's own
// phases ahead of dependency threads.

import "testing"

func TestBusinessClueOptionalNotesParseStrictly(t *testing.T) {
	plain := spanvisCompile(spanvisMentionRecord("trace_query:a#business_span_mention:1"))
	if len(plain.BusinessSpanMentions) != 1 || plain.BusinessSpanMentions[0].SelfKnown || plain.BusinessSpanMentions[0].States != nil || plain.BusinessSpanMentions[0].WallMS != 0 {
		t.Fatalf("a record without the new notes stays valid and carries none of them: %+v", plain.BusinessSpanMentions)
	}
	full := spanvisCompile(spanvisMentionRecord("trace_query:a#business_span_mention:1",
		TraceNoteKeyBusinessSpanWallMS+"=0.612",
		TraceNoteKeyBusinessSpanSelfMS+"=0.000",
		TraceNoteKeyBusinessSpanStates+"=running=0.112;runnable=0.000;sleep=0.500;d_state=0.000;io_wait=0.000;unknown=0.000"))
	if len(full.BusinessSpanMentions) != 1 {
		t.Fatalf("a complete record must parse: %+v", full)
	}
	m := full.BusinessSpanMentions[0]
	if !m.SelfKnown || m.SelfMS != 0 || m.States == nil || m.States.SleepMS != 0.5 || m.WallMS != 0.612 {
		t.Fatalf("own-phase account (a pure wrapper's zero self time is a measured value): %+v", m)
	}
	for name, note := range map[string]string{
		"self above total":         TraceNoteKeyBusinessSpanSelfMS + "=0.700",
		"states not adding up":     TraceNoteKeyBusinessSpanStates + "=running=0.100;runnable=0.000;sleep=0.100;d_state=0.000;io_wait=0.000;unknown=0.000",
		"states malformed":         TraceNoteKeyBusinessSpanStates + "=running=0.612",
		"wall below total":         TraceNoteKeyBusinessSpanWallMS + "=0.300",
		"self on a non-own record": "",
	} {
		record := spanvisMentionRecord("trace_query:a#business_span_mention:1", note)
		if name == "self on a non-own record" {
			record = spanvisOverrideNote(spanvisMentionRecord("trace_query:a#business_span_mention:1", TraceNoteKeyBusinessSpanSelfMS+"=0.100"), TraceNoteKeyBusinessSpanBasis, "chain_member")
		}
		if got := spanvisCompile(record); len(got.BusinessSpanMentions) != 0 {
			t.Fatalf("%s: a present but invalid note must drop the record whole: %+v", name, got.BusinessSpanMentions)
		}
	}
	dependency := spanvisCompile(spanvisOverrideNote(spanvisMentionRecord("trace_query:a#business_span_mention:1",
		TraceNoteKeyBusinessSpanWallMS+"=9.000"), TraceNoteKeyBusinessSpanBasis, "chain_member"))
	if len(dependency.BusinessSpanMentions) != 1 || !TraceBusinessSpanMentionDependencyPart(dependency.BusinessSpanMentions[0]) {
		t.Fatalf("a chain member counted only inside its waits is typed as such: %+v", dependency.BusinessSpanMentions)
	}
}

func TestBusinessClueBudgetedSelectionKeepsOwnPhasesFirst(t *testing.T) {
	row := func(subject, name, basis string, total, self float64, selfKnown bool) TraceCausalProjectionBusinessSpanMention {
		return TraceCausalProjectionBusinessSpanMention{Subject: subject, Name: name, Count: 1, TotalMS: total, MaxMS: total, StartLine: 1, EndLine: 2,
			Basis: basis, SelfKnown: selfKnown, SelfMS: self, WallMS: total}
	}
	in := []TraceCausalProjectionBusinessSpanMention{
		row("app-1", "wrapper", "self", 300, 1, true),
		row("app-1", "attach", "self", 290, 250, true),
		row("app-1", "lockWait", "self", 50, 50, true),
		row("worker-2", "longWork", "chain_member", 400, 0, false),
		row("worker-2", "shortWork", "chain_member", 20, 0, false),
	}
	got := TraceBusinessSpanMentionBudgetedSelection(in, 2)
	want := []string{"attach", "lockWait", "longWork", "shortWork"}
	if len(got) != len(want) {
		t.Fatalf("two own phases by own time, then two dependency rows: %+v", got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("position %d: want %s, got %+v", i, name, got)
		}
	}
}
