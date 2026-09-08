package types

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func b1618p2aWaitRows(scope, capture string, start, end float64, durations ...float64) []ObservationRecord {
	count := len(durations)
	ref := ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: capture, PayloadRef: "/payload/" + scope + ".json", RawRef: "/raw/" + scope + ".txt"}
	notes := []string{fmt.Sprintf("selected_window=%.6f..%.6f", start, end), fmt.Sprintf("%s=status=complete,emitted=%d,total=%d", TraceNoteKeyTargetWaitOccurrencePrompt, count, count)}
	var sum float64
	var leaves []ObservationRecord
	for i, duration := range durations {
		begin := start + float64(i+1)*.001
		finish := begin + duration/1000
		notes = append(notes, fmt.Sprintf("%s=#%d state=d_sleep %.6f..%.6f duration=%.3fms iowait=0 caller=wait_call", TraceNoteKeyTargetWaitOccurrence, i+1, begin, finish, duration))
		sum += duration
		leaves = append(leaves, ObservationRecord{ID: fmt.Sprintf("trace_query:%s#target_window_wait_occurrence:%d", scope, i+1), Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: ref, Span: ObservationSpan{StartTs: begin, EndTs: finish}, Subject: "app-100", Predicate: "target_window_wait_occurrence", Object: "state=d_sleep;iowait=0;caller=wait_call", Value: fmt.Sprintf("%.3f", duration), Unit: "ms", RichNotes: []string{fmt.Sprintf("selected_window=%.6f..%.6f", start, end)}, ObservedAt: "2026-09-08T10:00:00Z"})
	}
	notes = append(notes, fmt.Sprintf("%s=%.3f", TraceNoteKeyTargetWaitOccurrencePromptSum, sum))
	set := ObservationRecord{ID: "trace_query:" + scope + "#target_window_wait_occurrences", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: ref, Span: ObservationSpan{StartTs: start, EndTs: end}, Subject: "app-100", Predicate: "target_window_wait_occurrences", Object: "complete", Value: fmt.Sprint(count), Unit: "occurrences", ResultCount: &count, RichNotes: notes, ObservedAt: "2026-09-08T10:00:00Z"}
	return append([]ObservationRecord{set}, leaves...)
}

func b1618p2aWaitRequest() *RequestModel {
	return &RequestModel{RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit"}}}
}

func TestB1618P2aWaitAuthoritiesKeepCaptureAndQueryDomains(t *testing.T) {
	for _, tc := range []struct {
		name, capture        string
		start, end, duration float64
	}{
		{"same_basename_other_capture", "/capture/b/same.ftrace", 10, 10.02, .4},
		{"same_capture_other_query", "/capture/a/same.ftrace", 10, 10.0205, .4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := b1618p2aWaitRows("first", "/capture/a/same.ftrace", 10, 10.02, .3)
			b := b1618p2aWaitRows("second", tc.capture, tc.start, tc.end, tc.duration)
			for _, records := range [][]ObservationRecord{append(append([]ObservationRecord{}, a...), b...), append(append([]ObservationRecord{}, b...), a...)} {
				ledger := ObservationLedger{Records: records}
				if got := BuildTargetWaitOccurrenceAuthorities(ledger, b1618p2aWaitRequest()); len(got) != 2 {
					t.Errorf("independent preview domains lost: %+v", got)
				}
				if got := BuildTraceTargetWaitSummaryAuthorities(ledger, b1618p2aWaitRequest()); len(got) != 2 {
					t.Errorf("independent full domains lost: %+v", got)
				}
			}
		})
	}
}

func TestB1618P2aMeasuredZeroHasCompleteFullAccount(t *testing.T) {
	rows := b1618p2aWaitRows("zero", "/capture/a/same.ftrace", 10, 10.02)
	got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
	if len(got) != 1 || got[0].Count != 0 || got[0].WallClockMS != 0 || len(got[0].Occurrences) != 0 {
		t.Fatalf("measured complete zero was treated as missing: %+v", got)
	}
}

func TestB1618P2aFullRosterRejectsForeignPhysicalCaptureLeaf(t *testing.T) {
	rows := b1618p2aWaitRows("shared", "/payload/attached_trace.txt", 10, 10.02, .3)
	rows[0].SourceRef.CaptureIdentityPath = "/capture/a/same.ftrace"
	rows[1].SourceRef.CaptureIdentityPath = "/capture/b/same.ftrace"
	if got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest()); len(got) != 0 {
		t.Fatalf("same export/payload borrowed another physical capture's leaf: %+v", got)
	}
}

func TestB1618P2aScopeAndResultAxesAreIndependent(t *testing.T) {
	base := b1618p2aWaitRows("one", "/capture/a/same.ftrace", 10, 10.02, .3)[0]
	for _, tc := range []struct {
		name          string
		mutate        func(*ObservationRecord)
		query, result bool
	}{
		{"identical", func(*ObservationRecord) {}, true, true},
		{"one_microsecond", func(r *ObservationRecord) { r.RichNotes = []string{"selected_window=10.000001..10.020001"} }, true, true},
		{"neighbor_500us", func(r *ObservationRecord) { r.RichNotes = []string{"selected_window=10.000000..10.020500"} }, false, true},
		{"same_basename", func(r *ObservationRecord) { r.SourceRef.Path = "/capture/b/same.ftrace" }, false, false},
		{"target_case", func(r *ObservationRecord) { r.Subject = "App-100" }, false, true},
		{"unknown_query", func(r *ObservationRecord) { r.RichNotes = nil }, false, true},
		{"unknown_capture", func(r *ObservationRecord) { r.SourceRef.Path = ""; r.SourceRef.ArtifactID = "trace_query" }, false, false},
		{"other_payload", func(r *ObservationRecord) { r.SourceRef.PayloadRef = "/payload/other.json" }, true, false},
		{"other_raw", func(r *ObservationRecord) { r.SourceRef.RawRef = "/raw/other.txt" }, true, false},
		{"no_result_source", func(r *ObservationRecord) { r.SourceRef.RawRef, r.SourceRef.PayloadRef = "", "" }, true, false},
		{"other_execution", func(r *ObservationRecord) { r.ObservedAt = "2026-09-08T10:00:01Z" }, true, false},
		{"not_runtime_source", func(r *ObservationRecord) { r.SourceRef.Kind = "" }, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			tc.mutate(&other)
			if got := TraceRuntimeAccountRecordScope(base).SameQuery(TraceRuntimeAccountRecordScope(other)); got != tc.query {
				t.Fatalf("query=%t want=%t", got, tc.query)
			}
			if got := TraceRuntimeAccountRecordsSameResult(base, other); got != tc.result {
				t.Fatalf("result=%t want=%t", got, tc.result)
			}
		})
	}
	zero := b1618p2aWaitRows("zero-start", "/capture/a/same.ftrace", 0, .02)[0]
	if !TraceRuntimeAccountRecordScope(zero).Complete() {
		t.Fatal("explicit zero-start query is valid")
	}
}

func TestB1618P2aRepeatedCompleteAccountsRetainEveryRepresentedSource(t *testing.T) {
	a := b1618p2aWaitRows("first", "/capture/a/same.ftrace", 10, 10.02, .3)
	b := b1618p2aWaitRows("second", "/capture/a/same.ftrace", 10, 10.02, .3)
	rows := append(append([]ObservationRecord{}, a...), b...)
	before := fmt.Sprintf("%#v", rows)
	for _, records := range [][]ObservationRecord{rows, append(append([]ObservationRecord{}, b...), a...)} {
		full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
		preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
		wantIDs := traceRuntimeAccountMergeRecordIDs(nil, []string{a[0].ID, a[1].ID, b[0].ID, b[1].ID})
		if len(full) != 1 || full[0].WallClockMS != .3 || !reflect.DeepEqual(full[0].SourceRecordIDs, wantIDs) {
			t.Fatalf("full duplicate did not preserve exact sources: %+v", full)
		}
		if len(preview) != 1 || preview[0].SumMS != .3 || !reflect.DeepEqual(preview[0].SourceRecordIDs, wantIDs) {
			t.Fatalf("preview duplicate did not preserve verified sources: %+v", preview)
		}
		full[0].SourceRecordIDs[0] = "mutated-copy"
	}
	if before != fmt.Sprintf("%#v", rows) {
		t.Fatal("building or mutating result changed ledger")
	}
}

func TestB1618P2aCollidingIDsNeverAuthorizeReaderShadow(t *testing.T) {
	a := b1618p2aWaitRows("same-id", "/capture/a/same.ftrace", 10, 10.02, .3)
	b := b1618p2aWaitRows("same-id", "/capture/b/same.ftrace", 10, 10.02, .3)
	ledger := ObservationLedger{Records: append(a, b...)}
	full := BuildTraceTargetWaitSummaryAuthorities(ledger, b1618p2aWaitRequest())
	preview := BuildTargetWaitOccurrenceAuthorities(ledger, b1618p2aWaitRequest())
	if len(full) != 2 || len(preview) != 2 {
		t.Fatalf("ID collision lost scoped values: full=%+v preview=%+v", full, preview)
	}
	for i := range full {
		if len(full[i].SourceRecordIDs) != 0 || len(preview[i].SourceRecordIDs) != 0 {
			t.Fatal("ambiguous ID conferred reader-shadow authority")
		}
		if !strings.HasPrefix(full[i].ArtifactLabel, "/capture/") || !strings.HasPrefix(preview[i].ArtifactLabel, "/capture/") {
			t.Fatal("same-basename accounts need disambiguated reader labels")
		}
	}
}

func TestB1618P2aUnknownDomainsKeepIndependentZeroPreviews(t *testing.T) {
	for _, missing := range []string{"query", "capture"} {
		a := b1618p2aWaitRows("first", "/capture/a/same.ftrace", 10, 10.02)
		b := b1618p2aWaitRows("second", "/capture/a/same.ftrace", 10, 10.02)
		for _, records := range [][]ObservationRecord{a, b} {
			if missing == "query" {
				records[0].RichNotes = records[0].RichNotes[1:]
			} else {
				records[0].SourceRef.Path = ""
			}
		}
		ledger := ObservationLedger{Records: append(a, b...)}
		preview := BuildTargetWaitOccurrenceAuthorities(ledger, b1618p2aWaitRequest())
		if len(preview) != 2 || preview[0].Count != 0 || preview[1].Count != 0 {
			t.Fatalf("unknown domains must not merge or erase each other: %+v", preview)
		}
		if got := BuildTraceTargetWaitSummaryAuthorities(ledger, b1618p2aWaitRequest()); len(got) != 0 {
			t.Fatalf("unknown scope minted complete zero account: %+v", got)
		}
	}
}

func TestB1618P2aCompleteZeroRejectsPartialOrContradictoryRows(t *testing.T) {
	for _, mutation := range []string{"partial", "unknown_query", "extra_leaf", "wrong_count"} {
		t.Run(mutation, func(t *testing.T) {
			rows := b1618p2aWaitRows("zero", "/capture/a/same.ftrace", 10, 10.02)
			switch mutation {
			case "partial":
				rows[0].Object = "incomplete"
			case "unknown_query":
				rows[0].RichNotes = rows[0].RichNotes[1:]
			case "extra_leaf":
				rows = append(rows, b1618p2aWaitRows("zero", "/capture/a/same.ftrace", 10, 10.02, .3)[1])
			case "wrong_count":
				one := 1
				rows[0].ResultCount = &one
			}
			if got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest()); len(got) != 0 {
				t.Fatalf("invalid zero admitted: %+v", got)
			}
		})
	}
}

func TestB1618P2aEightPreviewDoesNotEraseElevenFullRows(t *testing.T) {
	a := b1618p2aWaitRows("eight", "/capture/a/same.ftrace", 10, 10.02, .1, .1, .1, .1, .1, .1, .1, .1)
	b := b1618p2aWaitRows("eleven", "/capture/a/same.ftrace", 20, 20.02, .1, .1, .1, .1, .1, .1, .1, .1, .1, .1, .1)
	ledger := ObservationLedger{Records: append(a, b...)}
	full := BuildTraceTargetWaitSummaryAuthorities(ledger, b1618p2aWaitRequest())
	preview := BuildTargetWaitOccurrenceAuthorities(ledger, b1618p2aWaitRequest())
	if len(full) != 2 || len(full[1].Occurrences) != 11 || len(preview) != 1 || preview[0].Count != 8 {
		t.Fatalf("bounded and full surfaces crossed domains: full=%+v preview=%+v", full, preview)
	}
	if len(preview[0].SourceRecordIDs) != 9 {
		t.Fatalf("rounded published sum must still preserve all eight validated leaf IDs: %v", preview[0].SourceRecordIDs)
	}
	for _, id := range preview[0].SourceRecordIDs {
		if strings.Contains(id, "eleven") {
			t.Fatal("8-row preview claims to represent separate 11-row account")
		}
	}
}

func TestB1618P2aWindowPairToleranceDoesNotTransitivelyCluster(t *testing.T) {
	var records []ObservationRecord
	for i, end := range []float64{10.02, 10.0200015, 10.020003} {
		rows := b1618p2aWaitRows(fmt.Sprint(i), "/capture/a/same.ftrace", 10, end, .3)
		// Preserve the exact producer endpoints to isolate grouping from display rounding.
		for j := range rows {
			rows[j].RichNotes[0] = fmt.Sprintf("selected_window=10..%.7f", end)
		}
		records = append(records, rows...)
	}
	for i := 0; i < 3; i++ {
		ledger := ObservationLedger{Records: records}
		if got := BuildTargetWaitOccurrenceAuthorities(ledger, b1618p2aWaitRequest()); len(got) != 3 {
			t.Fatalf("preview used non-transitive pair tolerance as clustering: %+v", got)
		}
		if got := BuildTraceTargetWaitSummaryAuthorities(ledger, b1618p2aWaitRequest()); len(got) != 3 {
			t.Fatalf("full used non-transitive pair tolerance as clustering: %+v", got)
		}
		records = append(records[2:], records[:2]...)
	}
}

func TestB1618P2aMultipleTargetsAndLocalConflictsDoNotSwallowOthers(t *testing.T) {
	a := b1618p2aWaitRows("first", "/capture/a/same.ftrace", 10, 10.02, .3)
	conflict := b1618p2aWaitRows("conflict", "/capture/a/same.ftrace", 10, 10.02, .4)
	b := b1618p2aWaitRows("second", "/capture/b/same.ftrace", 10, 10.02, .3)
	c := b1618p2aWaitRows("third", "/capture/a/same.ftrace", 10, 10.02, .3)
	for i := range c {
		c[i].Subject = "app-101"
	}
	rm := b1618p2aWaitRequest()
	rm.RuntimeTargets = append(rm.RuntimeTargets, RuntimeTarget{Kind: RuntimeTargetKindThread, PID: 101, Thread: "app-101", Source: "user_explicit"})
	ledger := ObservationLedger{Records: append(append(append(a, conflict...), b...), c...)}
	full := BuildTraceTargetWaitSummaryAuthorities(ledger, rm)
	preview := BuildTargetWaitOccurrenceAuthorities(ledger, rm)
	if len(full) != 2 || len(preview) != 2 {
		t.Fatalf("conflict in A/app100 erased B/app100 or A/app101: full=%+v preview=%+v", full, preview)
	}
}

func TestB1618P2aSourceQueryContradictionsDoNotMintFullAccount(t *testing.T) {
	for _, at := range []int{0, 1} {
		rows := b1618p2aWaitRows("query", "/capture/a/same.ftrace", 10, 10.02, .3)
		rows[at].RichNotes[0] = "selected_window=10..10.021"
		if got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest()); len(got) != 0 {
			t.Fatalf("contradictory query metadata formed full account: %+v", got)
		}
	}
}

func TestB1618P2aUnknownPositiveQueryKeepsMeasurementWithoutPrincipalRole(t *testing.T) {
	for _, fullArtifact := range []bool{false, true} {
		rows := b1618p2aWaitRows("unknown-query", "/capture/a/same.ftrace", 10, 10.02, .3)
		rows[0].RichNotes = rows[0].RichNotes[1:]
		rows[0].SystemSupplement = fullArtifact
		start, end := 10.0, 10.02
		rm := b1618p2aWaitRequest()
		rm.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "typed window"}
		if fullArtifact {
			rm.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeFullArtifact, SourceQuote: "whole trace"}
		}
		got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, rm)
		if len(got) != 1 || got[0].WallClockMS != .3 || got[0].WindowScope.Role != TraceQueryWindowScopeUnknownQueryWindow || got[0].IsRequestedScopePrincipal() || got[0].WindowStartTs != 0 || got[0].WindowEndTs != 0 {
			t.Fatalf("unknown local query lost its value or borrowed set Span as principal: %+v", got)
		}
	}
}

func TestB1618P2aAccountWindowFormatterPreservesKnownZeroStart(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		if got := FormatTraceRuntimeAccountWindow(0, .02, lang); got != "0.000000..0.020000" {
			t.Fatalf("known zero start changed: %q", got)
		}
		for _, ends := range [][2]float64{{0, 0}, {2, 1}, {math.NaN(), 2}, {1, math.Inf(1)}} {
			got := FormatTraceRuntimeAccountWindow(ends[0], ends[1], lang)
			want := "query window not stated"
			if strings.HasPrefix(lang, "zh") {
				want = "查询范围未明确"
			}
			if got != want {
				t.Fatalf("invalid producer window printed a pseudo ruler: %q", got)
			}
		}
	}
}

func TestB1618P2aZeroAndPositiveRequireActualRuntimeResultSource(t *testing.T) {
	for _, zero := range []bool{true, false} {
		for _, missing := range []string{"empty_kind", "other_kind", "missing_result"} {
			t.Run(fmt.Sprintf("zero=%t/%s", zero, missing), func(t *testing.T) {
				durations := []float64{.3}
				if zero {
					durations = nil
				}
				rows := b1618p2aWaitRows("receipt", "/capture/a/same.ftrace", 10, 10.02, durations...)
				for i := range rows {
					switch missing {
					case "empty_kind":
						rows[i].SourceRef.Kind = ""
					case "other_kind":
						rows[i].SourceRef.Kind = "current_source"
					case "missing_result":
						rows[i].SourceRef.PayloadRef, rows[i].SourceRef.RawRef = "", ""
					}
				}
				if got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest()); len(got) != 0 {
					t.Fatalf("missing runtime result provenance minted complete account: %+v", got)
				}
			})
		}
	}
}

func TestB1618P2aPreviewSourceReceiptsDoNotUpgradeLegacyNotes(t *testing.T) {
	for _, missing := range []string{"none", "empty_kind", "other_kind", "missing_result"} {
		t.Run(missing, func(t *testing.T) {
			rows := b1618p2aWaitRows("preview-source", "/capture/a/same.ftrace", 10, 10.02, .3)
			for i := range rows {
				switch missing {
				case "empty_kind":
					rows[i].SourceRef.Kind = ""
				case "other_kind":
					rows[i].SourceRef.Kind = "current_source"
				case "missing_result":
					rows[i].SourceRef.PayloadRef, rows[i].SourceRef.RawRef = "", ""
				}
			}
			got := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
			if len(got) != 1 || got[0].RecordID != rows[0].ID || got[0].Count != 1 || got[0].SumMS != .3 || len(got[0].Rows) != 1 {
				t.Fatalf("legacy local notes were rejected or rewritten: %+v", got)
			}
			if missing == "none" {
				if len(got[0].SourceRecordIDs) != 2 {
					t.Fatalf("actual same-result set/leaf receipts lost: %+v", got)
				}
			} else if len(got[0].SourceRecordIDs) != 0 {
				t.Fatalf("legacy notes granted reader-shadow authority: %+v", got[0].SourceRecordIDs)
			}
		})
	}
}

func TestB1618P2aWaitRecordIndexBoundsCandidatesAndRetainsCollisions(t *testing.T) {
	const size = 2048
	var records []ObservationRecord
	for i := 0; i < size; i++ {
		start := 10 + float64(i)
		records = append(records, b1618p2aWaitRows(fmt.Sprint(i), "/capture/a/same.ftrace", start, start+.02, .3)...)
	}
	indexed, _ := traceTargetWaitRecordIndexes(records)
	if len(indexed) != size {
		t.Fatalf("lost producer families: %d", len(indexed))
	}
	visited := 0
	for i := 0; i < size; i++ {
		positions := indexed["trace_query:"+fmt.Sprint(i)]
		visited += len(positions)
		if len(positions) != 1 || positions[0] != i*2+1 {
			t.Fatalf("family %d did not isolate its exact candidate: %v", i, positions)
		}
	}
	if visited != size {
		t.Fatalf("serial query candidate visits grew beyond retained leaves: %d", visited)
	}
	if got := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest()); len(got) != size {
		t.Fatalf("indexed serial preview dropped query accounts: %d", len(got))
	}

	a := b1618p2aWaitRows("collision", "/capture/a/same.ftrace", 10, 10.02, .3)
	b := b1618p2aWaitRows("collision", "/capture/b/same.ftrace", 10, 10.02, .4)
	for _, reversed := range []bool{false, true} {
		combined := append(append([]ObservationRecord{}, a...), b...)
		if reversed {
			combined = append(append([]ObservationRecord{}, b...), a...)
		}
		indexed, _ := traceTargetWaitRecordIndexes(combined)
		if len(indexed["trace_query:collision"]) != 2 {
			t.Fatal("index silently overwrote a same-ID leaf")
		}
		got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: combined}, b1618p2aWaitRequest())
		if len(got) != 2 || got[0].WallClockMS != .3 || got[1].WallClockMS != .4 || len(got[0].SourceRecordIDs)+len(got[1].SourceRecordIDs) != 0 {
			t.Fatalf("index borrowed foreign capture facts or IDs: %+v", got)
		}
	}

	malformed := a[1]
	malformed.ID += "#target_window_wait_occurrence:1"
	withMalformed := append(append([]ObservationRecord{}, a...), malformed)
	if got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: withMalformed}, b1618p2aWaitRequest()); len(got) != 0 {
		t.Fatalf("index concealed the prior malformed ordinal conflict: %+v", got)
	}
}

func BenchmarkB1618P2aWaitScopeCompilation(b *testing.B) {
	for _, size := range []int{64, 512, 2048} {
		var records []ObservationRecord
		for i := 0; i < size; i++ {
			start := 10 + float64(i)
			records = append(records, b1618p2aWaitRows(fmt.Sprint(i), "/capture/a/same.ftrace", start, start+.02, .3)...)
		}
		ledger := ObservationLedger{Records: records}
		for _, preview := range []bool{false, true} {
			b.Run(fmt.Sprintf("sets=%d/preview=%t", size, preview), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if preview {
						if got := BuildTargetWaitOccurrenceAuthorities(ledger, b1618p2aWaitRequest()); len(got) != size {
							b.Fatal(len(got))
						}
					} else {
						if got := BuildTraceTargetWaitSummaryAuthorities(ledger, b1618p2aWaitRequest()); len(got) != size {
							b.Fatal(len(got))
						}
					}
				}
			})
		}
	}
}
