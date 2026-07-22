package types

// trace_causal_projection_spanvis_test.go — SPANVIS-1 projection-compile pins:
// the business_span_mention side channel parses ALL-OR-NOTHING per record
// (any invalid typed field drops the record whole), never classifies into a
// node bucket, dedupes re-published record sets, and takes the first omitted
// counter.

import (
	"strings"
	"testing"
)

func spanvisMentionRecord(id string, notes ...string) ObservationRecord {
	base := []string{
		TraceNoteKeyBusinessSpanName + "=Lock contention on a monitor lock (owner tid: 62020)",
		TraceNoteKeyBusinessSpanCount + "=3",
		TraceNoteKeyBusinessSpanTotalMS + "=0.612",
		TraceNoteKeyBusinessSpanMaxMS + "=0.303",
		TraceNoteKeyBusinessSpanLines + "=5..9",
		TraceNoteKeyBusinessSpanBasis + "=self",
		TraceNoteKeyBusinessSpanHidden + "=3",
		TraceNoteKeyBusinessSpanOmitted + "=2",
	}
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Subject:         "LegoHandler-17585",
		Predicate:       "business_span_mention",
		Summary:         "advisory business span lead",
		RichNotes:       append(base, notes...),
	}
}

// spanvisCompanionNodeRecord is a minimal ranked node record: the mention
// side channel deliberately does NOT activate a projection (Active() reads
// only node/path populations — 不参与 structural half), so every compile in
// these pins rides beside one real node.
func spanvisCompanionNodeRecord() ObservationRecord {
	return ObservationRecord{
		ID:              "trace_query:w1#root_cause_rank:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Subject:         "RxComputationT-16816",
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary",
		Object:          "runnable_wait",
		Value:           "1.598",
		Unit:            "ms",
		RichNotes:       []string{"rank=1", "type=runnable_wait"},
	}
}

func spanvisCompile(records ...ObservationRecord) TraceCausalProjection {
	return TraceCausalProjectionFromObservationRecords(append([]ObservationRecord{spanvisCompanionNodeRecord()}, records...))
}

// spanvisOverrideNote replaces the value of one note key on a record.
func spanvisOverrideNote(record ObservationRecord, key, value string) ObservationRecord {
	var notes []string
	for _, note := range record.RichNotes {
		if strings.HasPrefix(note, key+"=") {
			if value != "" {
				notes = append(notes, key+"="+value)
			}
			continue
		}
		notes = append(notes, note)
	}
	record.RichNotes = notes
	return record
}

func TestSpanvisMentionRecordParsesStrict(t *testing.T) {
	proj := spanvisCompile(spanvisMentionRecord("trace_query:a#business_span_mention:1"))
	if len(proj.BusinessSpanMentions) != 1 {
		t.Fatalf("valid record must parse: %+v", proj.BusinessSpanMentions)
	}
	m := proj.BusinessSpanMentions[0]
	if m.Subject != "LegoHandler-17585" ||
		m.Name != "Lock contention on a monitor lock (owner tid: 62020)" ||
		m.Count != 3 || m.TotalMS != 0.612 || m.MaxMS != 0.303 ||
		m.StartLine != 5 || m.EndLine != 9 || m.Basis != "self" || m.Hidden != 3 {
		t.Fatalf("typed transport must be verbatim: %+v", m)
	}
	if proj.BusinessSpanMentionOmitted != 2 {
		t.Fatalf("omitted counter must parse: %d", proj.BusinessSpanMentionOmitted)
	}
	// Side channel: the mention record classifies into NO node bucket (only
	// the companion node populates). ISPGAP-1 件2' (§29.202, 2026-07-21): the
	// companion is an UNDECLARED-relevance rank record, whose honest landing
	// is now the ▒ background seat (never the primary/⛓ crown lane) — the
	// single-node population stays the assertion.
	if len(proj.PrimaryRootCauses) != 0 || len(proj.OnChainCauses) != 0 ||
		len(proj.AdjacentCauses) != 0 || len(proj.BackgroundCauses) != 1 ||
		len(proj.SemanticSpans) != 0 || len(proj.SupportingHops) != 0 {
		t.Fatalf("a mention record must never become a node: %+v", proj)
	}
	// 不参与 structural half: a mentions-ONLY record set never ACTIVATES a
	// projection (Active() reads node/path populations exclusively) — the
	// advisory face cannot conjure a report by itself.
	alone := TraceCausalProjectionFromObservationRecords([]ObservationRecord{spanvisMentionRecord("trace_query:a#business_span_mention:1")})
	if alone.Active() || len(alone.BusinessSpanMentions) != 0 {
		t.Fatalf("mention records alone must not activate a projection: %+v", alone)
	}
}

func TestSpanvisMentionRecordInvalidArmsDropWhole(t *testing.T) {
	arms := map[string]ObservationRecord{
		"name_missing":   spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanName, ""),
		"count_zero":     spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanCount, "0"),
		"count_garbage":  spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanCount, "many"),
		"total_zero":     spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanTotalMS, "0.000"),
		"total_garbage":  spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanTotalMS, "n/a"),
		"max_zero":       spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanMaxMS, "0"),
		"max_over_total": spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanMaxMS, "0.614"),
		"lines_missing":  spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanLines, ""),
		"lines_shape":    spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanLines, "5-9"),
		"lines_zero":     spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanLines, "0..9"),
		"lines_invert":   spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanLines, "9..5"),
		"basis_unknown":  spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanBasis, "guessed"),
		"basis_missing":  spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanBasis, ""),
		// POOL2-1 件① EVOLUTION (§29.160① ruling): hidden_zero moved to the
		// POSITIVE arm below (a fully-visible family is valid now); the strict
		// arm keeps requiring the key's PRESENCE and the 0..Count range.
		"hidden_missing": spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanHidden, ""),
		"hidden_junk":    spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanHidden, "some"),
		"hidden_neg":     spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanHidden, "-1"),
		"hidden_over":    spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanHidden, "4"),
	}
	subjectless := spanvisMentionRecord("r")
	subjectless.Subject = "  "
	arms["subject_empty"] = subjectless
	for name, record := range arms {
		proj := spanvisCompile(record)
		if len(proj.BusinessSpanMentions) != 0 {
			t.Fatalf("%s: invalid record must drop whole (all-or-nothing): %+v", name, proj.BusinessSpanMentions)
		}
		if len(proj.PrimaryRootCauses)+len(proj.OnChainCauses)+len(proj.AdjacentCauses)+
			len(proj.BackgroundCauses)+len(proj.SemanticSpans)+len(proj.SupportingHops) != 1 {
			t.Fatalf("%s: a dropped mention record must not leak into node buckets", name)
		}
	}
}

// TestSpanvisMentionHiddenZeroParses — POOL2-1 件① (§29.160/§29.160.1 ruling
// 2026-07-20 verbatim: 「这里的 "某个span过长…则需要提及" 指的是链上的(注意
// 自身也属于链上),如果非链上…属于噪音。」): a fully-visible on-chain family
// (hidden=0, published explicitly) is a VALID record — the former third gate
// is removed, so the parser admits the 0 value while still requiring the key.
func TestSpanvisMentionHiddenZeroParses(t *testing.T) {
	rec := spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanHidden, "0")
	proj := spanvisCompile(rec)
	if len(proj.BusinessSpanMentions) != 1 || proj.BusinessSpanMentions[0].Hidden != 0 {
		t.Fatalf("hidden=0 (fully-visible family) must parse post-§29.160①: %+v", proj.BusinessSpanMentions)
	}
}

// TestSpanvisMentionMaxTotalPrintQuantum — the µs print-quantum tolerance:
// max == total survives (single-member family), max == total+0.001 survives
// (one quantum), max == total+0.002 drops.
func TestSpanvisMentionMaxTotalPrintQuantum(t *testing.T) {
	ok := spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanMaxMS, "0.612")
	if proj := spanvisCompile(ok); len(proj.BusinessSpanMentions) != 1 {
		t.Fatalf("max==total must parse")
	}
	edge := spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanMaxMS, "0.613")
	if proj := spanvisCompile(edge); len(proj.BusinessSpanMentions) != 1 {
		t.Fatalf("one print quantum must survive")
	}
	over := spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanMaxMS, "0.614")
	if proj := spanvisCompile(over); len(proj.BusinessSpanMentions) != 0 {
		t.Fatalf("beyond one quantum must drop")
	}
}

// TestSpanvisMentionDedupeAndOmittedFirstWins — a re-published record set
// cannot double the list; the first parsed omitted counter wins.
func TestSpanvisMentionDedupeAndOmittedFirstWins(t *testing.T) {
	first := spanvisMentionRecord("trace_query:a#business_span_mention:1")
	twin := spanvisMentionRecord("trace_query:b#business_span_mention:1")
	second := spanvisOverrideNote(spanvisMentionRecord("trace_query:a#business_span_mention:2"), TraceNoteKeyBusinessSpanLines, "50..90")
	second = spanvisOverrideNote(second, TraceNoteKeyBusinessSpanOmitted, "7")
	proj := spanvisCompile(first, twin, second)
	if len(proj.BusinessSpanMentions) != 2 {
		t.Fatalf("identity dedupe must fold the re-published twin: %+v", proj.BusinessSpanMentions)
	}
	if proj.BusinessSpanMentionOmitted != 2 {
		t.Fatalf("first omitted counter wins: %d", proj.BusinessSpanMentionOmitted)
	}
	// Omitted absent → zero (zero-drop wire discipline).
	bare := spanvisOverrideNote(spanvisMentionRecord("r"), TraceNoteKeyBusinessSpanOmitted, "")
	if proj := spanvisCompile(bare); proj.BusinessSpanMentionOmitted != 0 {
		t.Fatalf("absent omitted note parses as zero: %d", proj.BusinessSpanMentionOmitted)
	}
}
