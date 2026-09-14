package orchestrator

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1618IdentityRecord() types.ObservationRecord {
	r := p6AccountRecord("worker-41", 2, 0, 8, 0, 0, 10, 10)
	r.ID = "state-account"
	r.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact,
		Path: "/captures/a/same.systrace", CaptureIdentityPath: "/captures/a/same.systrace",
		PayloadRef: "payload-a", RawRef: "raw-a", QueryScopeID: "query-a",
		QueryWindowKnown: true, QueryWindowStartTs: 10, QueryWindowEndTs: 10.01,
		QueryTargetPID: 41, QueryTargetThread: "worker", QueryTargetScope: "thread", QueryLineRangeKnown: true}
	r.RichNotes[len(r.RichNotes)-1] = types.TraceNoteKeySelectedWindow + "=10.000000..10.010000"
	r.ObservedAt = "2026-09-13T00:00:00Z"
	r.SupportRefs = nil
	return r
}

func b1618IdentityAccount(r types.ObservationRecord) proseWallClockAccount {
	return proseWallClockAccount{subject: r.Subject, tid: "41", scope: types.TraceRuntimeAccountRecordScope(r),
		dims:    map[proseWallClockDimension]float64{proseWallClockDimRunning: 2, proseWallClockDimRunnable: 0, proseWallClockDimSleep: 8, proseWallClockDimDState: 0},
		totalMS: 10, windowMS: 10, source: r, sourceKnown: true}
}

// White-box source-bridge test: the account has already been elected. These
// checks do not manufacture an authority or claim an end-to-end query replay.
func TestB1618StateAccountSourceLookupRejectsAmbiguousResultReceipt(t *testing.T) {
	base := b1618IdentityRecord()
	for _, tc := range []struct {
		name string
		edit func(*types.ObservationRecord)
	}{
		{"payload", func(r *types.ObservationRecord) { r.SourceRef.PayloadRef = "payload-b" }},
		{"raw", func(r *types.ObservationRecord) { r.SourceRef.RawRef = "raw-b" }},
		{"query_id", func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID = "query-b" }},
		{"line_filter", func(r *types.ObservationRecord) { r.SourceRef.QueryLineStart, r.SourceRef.QueryLineEnd = 1, 6 }},
		{"unknown_line_filter", func(r *types.ObservationRecord) { r.SourceRef.QueryLineRangeKnown = false }},
		{"parent_window", func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 11 }},
		{"parent_target", func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 42 }},
		{"parent_scope", func(r *types.ObservationRecord) { r.SourceRef.QueryTargetScope = "process" }},
		{"record_time", func(r *types.ObservationRecord) { r.ObservedAt = "2026-09-13T00:00:01Z" }},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tc.name, reverse), func(t *testing.T) {
				other := base
				tc.edit(&other)
				rows := []types.ObservationRecord{base, other}
				if reverse {
					rows[0], rows[1] = rows[1], rows[0]
				}
				before, _ := json.Marshal(rows)
				acc := b1618IdentityAccount(base)
				if got, ok := proseWallClockAuthoritySource(acc, []string{base.ID}, types.ObservationLedger{Records: rows}); ok || !reflect.DeepEqual(got, types.ObservationRecord{}) {
					t.Fatalf("same ID and state values cannot choose one conflicting result: %+v / %t", got, ok)
				}
				after, _ := json.Marshal(rows)
				if string(before) != string(after) {
					t.Fatal("source lookup mutated input records")
				}
			})
		}
	}
	acc := b1618IdentityAccount(base)
	if got, ok := proseWallClockAuthoritySource(acc, []string{base.ID}, types.ObservationLedger{Records: []types.ObservationRecord{base, base}}); !ok || !reflect.DeepEqual(got, base) {
		t.Fatal("an exact repeated original record must remain resolvable")
	}
	if _, ok := proseWallClockAuthoritySource(acc, nil, types.ObservationLedger{Records: []types.ObservationRecord{base}}); ok {
		t.Fatal("matching numbers without the elected record ID cannot mint a receipt")
	}
	for _, tc := range []struct {
		name string
		edit func(*types.ObservationRecord)
	}{
		{"capture", func(r *types.ObservationRecord) { r.SourceRef.CaptureIdentityPath = "/captures/b/same.systrace" }},
		{"subject", func(r *types.ObservationRecord) { r.Subject = "different-name-41" }},
		{"window", func(r *types.ObservationRecord) {
			r.RichNotes = append([]string(nil), r.RichNotes...)
			r.RichNotes[len(r.RichNotes)-1] = types.TraceNoteKeySelectedWindow + "=20.000000..20.010000"
		}},
		{"value", func(r *types.ObservationRecord) {
			r.RichNotes = append([]string(nil), r.RichNotes...)
			r.RichNotes[0] = types.TraceNoteKeyRunning + "=3.000"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.edit(&r)
			if _, ok := proseWallClockAuthoritySource(acc, []string{base.ID}, types.ObservationLedger{Records: []types.ObservationRecord{r}}); ok {
				t.Fatal("record ID alone cannot join a different account")
			}
		})
	}
}

// The fixture begins at the legacy observation boundary; the actual ledger
// compilation and shipped-answer appendix are exercised by the shared helper.
// It intentionally does not claim the current TraceQuery emits unknown scopes.
func TestB1618PublishedUnknownStateAccountsRemainIndependentAtDisplayCaps(t *testing.T) {
	for _, count := range []int{2, 5, 12} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(fmt.Sprintf("count=%d/%s", count, lang), func(t *testing.T) {
				var rows []types.ObservationRecord
				for i := 0; i < count; i++ {
					r := b1618IdentityRecord()
					r.ID = fmt.Sprintf("legacy-state-%02d", i)
					r.ClaimKey = "target_window_states:" + r.ID
					r.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact}
					r.Span = types.ObservationSpan{}
					r.SupportRefs = nil
					r.RichNotes = append([]string(nil), r.RichNotes[:len(r.RichNotes)-1]...)
					rows = append(rows, r)
				}
				body := b1618PublishStateAppendix(t, []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: rows}}, nil, lang)
				shown := 0
				for _, line := range strings.Split(body, "\n") {
					if strings.Contains(line, "running 2.000") {
						shown++
						if !strings.Contains(line, "worker-41") || (!strings.Contains(line, "来源未明确") && !strings.Contains(line, "source not stated")) {
							t.Fatalf("unknown record received invented source or lost own subject: %s", line)
						}
					}
				}
				want := count
				if count > systemCrossCheckFindingCap {
					want = systemCrossCheckFindingCap - 1
				} // one partition-section omission disclosure occupies a slot
				if shown != want {
					t.Fatalf("equal unknown accounts folded or duplicate faces consumed cap: shown=%d want=%d\n%s", shown, want, body)
				}
				if strings.Contains(body, "0.000000..0.000000") || strings.Contains(body, "/captures/a/") || strings.Contains(body, "query-a") {
					t.Fatalf("legacy unknown coordinates or source were invented: %s", body)
				}
				if count > proseFactPartitionCap {
					omission := fmt.Sprintf("本分区栏另有 %d 条状态账户未逐条展示", count-proseFactPartitionCap)
					if lang == "en" {
						omission = fmt.Sprintf("%d additional state accounts are not expanded in this partition section", count-proseFactPartitionCap)
					}
					if !strings.Contains(body, omission) {
						t.Fatalf("partition cap omitted accounts without its actual remaining count: %s", body)
					}
				}
			})
		}
	}
}

func TestB1618StateAccountUnknownWindowDoesNotInventDuration(t *testing.T) {
	r := b1618IdentityRecord()
	r.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact}
	var notes []string
	for _, note := range r.RichNotes {
		if !strings.HasPrefix(note, types.TraceNoteKeySelectedWindow+"=") && !strings.HasPrefix(note, types.TraceNoteKeyWindowMS+"=") {
			notes = append(notes, note)
		}
	}
	r.RichNotes = notes
	accounts := proseWallClockAccountsFromLedger(types.ObservationLedger{Records: []types.ObservationRecord{r}})
	if len(accounts) != 1 || accounts[0].scope.WindowKnown || accounts[0].windowMS != 0 || accounts[0].totalMS != 10 {
		t.Fatalf("legacy observed duration must not become a measured query duration: %+v", accounts)
	}
	zh, en := proseFactPartitionFact(&proseFactThreadFacts{subject: r.Subject, account: &accounts[0]})
	if !strings.Contains(zh, "窗长未明确") || !strings.Contains(en, "window duration not stated") || strings.Contains(zh, "窗长 10.000") || strings.Contains(en, "window 10.000") {
		t.Fatalf("missing duration was manufactured from the state sum: %s / %s", zh, en)
	}
	zero := b1618IdentityAccount(b1618IdentityRecord())
	zero.scope.WindowStartTs, zero.scope.WindowEndTs = 0, .01
	zh, en = proseFactAccountSource(&zero)
	if !strings.Contains(zh, "0.000000..0.010000") || !strings.Contains(en, "0.000000..0.010000") {
		t.Fatalf("a valid zero-start measurement must not become unknown: %s / %s", zh, en)
	}
}

// Display-order and republication pins are intentionally white-box. They do
// not change scope election, request order, or source eligibility.
func TestB1618StateAccountOrderingAndCompleteOnlyDeduplication(t *testing.T) {
	base := b1618IdentityAccount(b1618IdentityRecord())
	for _, tc := range []struct {
		name string
		edit func(*proseWallClockAccount)
		want int
	}{
		{"exact_complete", func(*proseWallClockAccount) {}, 1},
		{"missing_result", func(a *proseWallClockAccount) { a.source.SourceRef.PayloadRef, a.source.SourceRef.RawRef = "", "" }, 2},
		{"missing_window", func(a *proseWallClockAccount) { a.scope.WindowKnown = false }, 2},
		{"missing_capture", func(a *proseWallClockAccount) { a.scope.ArtifactKey = "" }, 2},
		{"missing_query", func(a *proseWallClockAccount) { a.source.SourceRef.QueryScopeID = "" }, 2},
		{"missing_time", func(a *proseWallClockAccount) { a.source.ObservedAt = "" }, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := base
			tc.edit(&a)
			got := proseWallClockOrderedAccounts([]proseWallClockAccount{a, a})
			if len(got) != tc.want {
				t.Fatalf("dedup count=%d want=%d", len(got), tc.want)
			}
			if len(got) == 2 && (got[0].observationOrdinal == 0 || got[0].observationOrdinal == got[1].observationOrdinal) {
				t.Fatal("independent equal rows need distinct display-only occurrences")
			}
		})
	}
	a, b := base, base
	a.windowScope = types.TraceQueryWindowScope{RequestedWindowCount: 2, RequestedWindowOrdinal: 2}
	b.windowScope = types.TraceQueryWindowScope{RequestedWindowCount: 2, RequestedWindowOrdinal: 1}
	b.scope.WindowStartTs, b.scope.WindowEndTs = 20, 20.01
	got := proseWallClockOrderedAccounts([]proseWallClockAccount{a, b})
	if len(got) != 2 || got[0].windowScope.RequestedWindowOrdinal != 1 || got[1].windowScope.RequestedWindowOrdinal != 2 {
		t.Fatalf("display sorting replaced user order with timestamp order: %+v", got)
	}
}
