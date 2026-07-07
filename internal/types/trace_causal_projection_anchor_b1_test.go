package types

// B1 anchor-election pins (§10-B1 归因 + §12.3 裁定3, 2026-07-06 用户裁定:
// "存在 target 匹配用户实体的 wakeup_chain path 记录时,锚优先给用户实体线程;
// 第一条 path 先到先得废止") — as corrected by 二轮复核 F1a/F1b/F2/F3/F4.
//
// Core principle: a user entity is what the USER named (runtime_targets
// user-source / ExactTargets / analyzer prose entity), NEVER the model's
// exploration cursor (trace_query_explicit_tool_call Source). The cursor is
// LLM-driven noise and must not drive a structural re-root + banner
// short-circuit.
//
// Pin map:
//   ① q1 shape (VSync path first, user-thread path second, typed user entity)
//     → anchor = user thread, WakeupPathUserElected=true;
//   ② no user-entity signal → first path, byte-stable legacy;
//   ③ entity present but NO matching path → first path, no election flag;
//   ④ multiple matching paths → deterministic first-in-publication tie-break;
//   F1a exploration-cursor RuntimeTargets never elect;
//   F1b cursor frame bundle never dominates (frame subject only corroborates);
//   F2 date/issue-N prose entities never mint a tid handle;
//   F3 bare frame-number prose entities never fake an election;
//   F4 over-cap pid dropped.

import (
	"reflect"
	"strconv"
	"testing"
)

func anchorB1PathRecord(id, target, path string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "wakeup_chain:path",
		Subject:         target,
		Predicate:       "wakeup_chain",
		Object:          path,
		Confidence:      0.82,
	}
}

func anchorB1FrameRecord(id, subject, source string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "frame_target_resolution:" + subject,
		Subject:         subject,
		Predicate:       "frame_target_resolution",
		Object:          source,
		Confidence:      1,
	}
}

func anchorB1Q1Records() []ObservationRecord {
	return []ObservationRecord{
		// The model's exploratory drilldown of the waker published FIRST — the
		// legacy first-wins lane made this the 🎯 root (q1 免责横幅 shape).
		anchorB1PathRecord("path-vsync", "VSyncGenerator-2270",
			"tppmgr-idle-7-192 -> VSyncGenerator-2270"),
		// The user-thread chain published SECOND.
		anchorB1PathRecord("path-user", "oney.hmn.berlin-42591",
			"VSyncGenerator-2270 -> oney.hmn.berlin-42591"),
	}
}

func anchorB1TypedEntities(values ...string) []AnchorUserEntity {
	out := make([]AnchorUserEntity, 0, len(values))
	for _, v := range values {
		out = append(out, AnchorUserEntity{Value: v, TypedLane: true})
	}
	return out
}

func anchorB1CompileSet(t *testing.T, entities []AnchorUserEntity, records ...ObservationRecord) TraceCausalProjection {
	t.Helper()
	set := CompileTraceCausalProjectionSet(ObservationLedger{Records: records, AnchorUserEntities: entities})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	return set.Projections[0]
}

// Pin ①: typed user entity (the analyzer's bare-pid entity "42591") elects the
// user-thread path over the earlier-published VSync path.
func TestTraceCausalProjectionAnchorElectsUserEntityPath(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		anchorB1Q1Records(), []string{"42591"})
	if len(got.WakeupPath) != 2 || got.WakeupPath[1] != "oney.hmn.berlin-42591" {
		t.Fatalf("裁定3: anchor must be the user-entity path end, got %v", got.WakeupPath)
	}
	if !got.WakeupPathUserElected {
		t.Fatalf("裁定3: WakeupPathUserElected must be true on an entity-elected anchor")
	}
}

// Pin ① (production carrier): the same election flows through the ledger's
// typed AnchorUserEntities into the partitioned compile entry.
func TestTraceCausalProjectionSetAnchorElectionViaLedgerEntities(t *testing.T) {
	p := anchorB1CompileSet(t, anchorB1TypedEntities("42591"), anchorB1Q1Records()...)
	if len(p.WakeupPath) != 2 || p.WakeupPath[1] != "oney.hmn.berlin-42591" || !p.WakeupPathUserElected {
		t.Fatalf("ledger-entity election failed: path=%v elected=%v", p.WakeupPath, p.WakeupPathUserElected)
	}
}

// Pin ②: no user-entity signal → the FIRST published path stays the anchor,
// election flag stays false, and the nil-entities entry is IDENTICAL to the
// legacy records-only entry (byte-stability of the no-signal lane).
func TestTraceCausalProjectionAnchorFirstPathWithoutEntities(t *testing.T) {
	records := anchorB1Q1Records()
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.WakeupPath) != 2 || got.WakeupPath[1] != "VSyncGenerator-2270" {
		t.Fatalf("legacy lane: first published path must stay the anchor, got %v", got.WakeupPath)
	}
	if got.WakeupPathUserElected {
		t.Fatalf("legacy lane must not claim a user election")
	}
	if !reflect.DeepEqual(got, TraceCausalProjectionFromObservationRecordsForUserEntities(records, nil)) {
		t.Fatalf("nil-entities compile must equal the legacy records-only compile")
	}
}

// Pin ③: a typed entity exists but NO path record's end matches it → first
// path stays (byte-stable), no election flag — the tool layer keeps the
// 免责横幅 exactly as before.
func TestTraceCausalProjectionAnchorNoMatchingPathStaysFirst(t *testing.T) {
	records := []ObservationRecord{
		anchorB1PathRecord("path-vsync", "VSyncGenerator-2270",
			"tppmgr-idle-7-192 -> VSyncGenerator-2270"),
	}
	legacy := TraceCausalProjectionFromObservationRecords(records)
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(records, []string{"42591"})
	if !reflect.DeepEqual(got, legacy) {
		t.Fatalf("no-matching-path lane must stay byte-stable with legacy:\nlegacy=%+v\ngot=%+v", legacy, got)
	}
	if got.WakeupPathUserElected || got.WakeupPath[1] != "VSyncGenerator-2270" {
		t.Fatalf("no-matching-path lane must keep the first path unelected, got %v elected=%v",
			got.WakeupPath, got.WakeupPathUserElected)
	}
}

// Pin ④: multiple matching paths → deterministic tie-break = the FIRST
// matching path in publication order; swapping publication order swaps the
// winner (order-determined, hence deterministic for a fixed ledger).
func TestTraceCausalProjectionAnchorTieBreakFirstMatchingPath(t *testing.T) {
	p1 := anchorB1PathRecord("path-a", "main-42591", "waker-1 -> main-42591")
	p2 := anchorB1PathRecord("path-b", "oney.hmn.berlin-42591", "waker-2 -> oney.hmn.berlin-42591")
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{p1, p2}, []string{"42591"})
	if !got.WakeupPathUserElected || got.WakeupPath[1] != "main-42591" {
		t.Fatalf("tie-break must elect the first matching path, got %v", got.WakeupPath)
	}
	swapped := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{p2, p1}, []string{"42591"})
	if !swapped.WakeupPathUserElected || swapped.WakeupPath[1] != "oney.hmn.berlin-42591" {
		t.Fatalf("tie-break must follow publication order, got %v", swapped.WakeupPath)
	}
}

// F1a (二轮复核 P0, MUTATION pin): the model's exploration cursor
// (RuntimeTargets Source=trace_query_explicit_tool_call) must NEVER elect. q1
// 游走 shape: the model drilled the VSync waker, so the cursor pid is
// VSyncGenerator's — that cursor must not hijack the anchor onto its own
// drilled path nor short-circuit the 免责横幅 with a false ‹用户关注线程›.
// Reverting the F1a exclusion (letting the cursor source through) must red.
func TestTraceCausalProjectionAnchorCursorSourceNeverElects(t *testing.T) {
	rm := &RequestModel{}
	rm.RuntimeTargets = []RuntimeTarget{{
		Kind: RuntimeTargetKindThread, PID: 2270, Thread: "VSyncGenerator",
		Source: RuntimeTargetSourceExplicitToolCall,
	}}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		RequestModel: rm,
		ToolResults:  []ToolResult{{ToolName: "trace_query", Success: true, Observations: anchorB1Q1Records()}},
	})
	for _, entity := range ledger.AnchorUserEntities {
		if entity.Value == "2270" || entity.Value == "VSyncGenerator" {
			t.Fatalf("F1a: the exploration cursor must be excluded from AnchorUserEntities, got %+v", ledger.AnchorUserEntities)
		}
	}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	p := set.Projections[0]
	if p.WakeupPathUserElected {
		t.Fatalf("F1a: a cursor-only ledger must NOT elect (honest 免责横幅 preserved), got path %v", p.WakeupPath)
	}
	if p.WakeupPath[len(p.WakeupPath)-1] != "VSyncGenerator-2270" {
		t.Fatalf("F1a: cursor-only ledger keeps the legacy first path, got %v", p.WakeupPath)
	}
}

// F1a positive control: a genuine user-source runtime target (Source=
// "user_explicit") DOES elect through the same carrier — the exclusion is
// source-scoped, not a blanket runtime_targets ban.
func TestTraceCausalProjectionAnchorUserSourceRuntimeTargetElects(t *testing.T) {
	rm := &RequestModel{}
	rm.RuntimeTargets = []RuntimeTarget{{Kind: RuntimeTargetKindProcess, PID: 42591, Source: "user_explicit"}}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		RequestModel: rm,
		ToolResults:  []ToolResult{{ToolName: "trace_query", Success: true, Observations: anchorB1Q1Records()}},
	})
	p := CompileTraceCausalProjectionSet(ledger).Projections[0]
	if !p.WakeupPathUserElected || p.WakeupPath[len(p.WakeupPath)-1] != "oney.hmn.berlin-42591" {
		t.Fatalf("user-source runtime target must elect, got %v elected=%v", p.WakeupPath, p.WakeupPathUserElected)
	}
}

// F1b (二轮复核 P0): a frame_target_resolution explicit_query_target subject
// only CORROBORATES — it elects when it agrees with a caller user entity, and
// is IGNORED (a cursor's own frame bundle) when it matches no user entity.
func TestTraceCausalProjectionAnchorFrameCorroboratesOnly(t *testing.T) {
	// Corroborating frame subject + matching caller entity → elects on the
	// canonical-label path even when the caller only gave a bare pid.
	corroborate := append([]ObservationRecord{
		anchorB1FrameRecord("frame", "oney.hmn.berlin-42591", "explicit_query_target"),
	}, anchorB1Q1Records()...)
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(corroborate, []string{"42591"})
	if !got.WakeupPathUserElected || got.WakeupPath[len(got.WakeupPath)-1] != "oney.hmn.berlin-42591" {
		t.Fatalf("F1b: corroborating frame subject must elect the user path, got %v", got.WakeupPath)
	}

	// The cursor's own frame bundle: the frame subject is VSyncGenerator (the
	// drilled waker), matching NO user entity → must NOT drag the anchor onto
	// VSync. The caller entity 42591 still elects its own path directly.
	cursorFrame := append([]ObservationRecord{
		anchorB1FrameRecord("frame", "VSyncGenerator-2270", "explicit_query_target"),
	}, anchorB1Q1Records()...)
	cursor := TraceCausalProjectionFromObservationRecordsForUserEntities(cursorFrame, []string{"42591"})
	if cursor.WakeupPath[len(cursor.WakeupPath)-1] != "oney.hmn.berlin-42591" {
		t.Fatalf("F1b: cursor frame subject must NOT drag the anchor onto VSync, got %v", cursor.WakeupPath)
	}

	// Frame subject present but NO caller entity at all → frame lane inert,
	// legacy first path.
	noCaller := TraceCausalProjectionFromObservationRecords(cursorFrame)
	if noCaller.WakeupPathUserElected || noCaller.WakeupPath[1] != "VSyncGenerator-2270" {
		t.Fatalf("F1b: a frame subject alone (no caller entity) must never elect, got %v elected=%v",
			noCaller.WakeupPath, noCaller.WakeupPathUserElected)
	}
}

// F2 (二轮复核 P1): a PROSE entity ("2026-07-06" / "issue-42") must NOT be
// mined for a "-<digits>" tid handle. A date whose trailing "-06" would parse
// to tid 6 must not elect a worker-6 path.
func TestTraceCausalProjectionAnchorProseEntityNoTidTail(t *testing.T) {
	prose := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            []ObservationRecord{anchorB1PathRecord("p", "worker-6", "waker -> worker-6")},
		AnchorUserEntities: []AnchorUserEntity{{Value: "2026-07-06", TypedLane: false}},
	}).Projections[0]
	if prose.WakeupPathUserElected {
		t.Fatalf("F2: a prose date must not mint a tid handle, got %v", prose.WakeupPath)
	}
	issue := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            []ObservationRecord{anchorB1PathRecord("p", "svc-42", "w -> svc-42")},
		AnchorUserEntities: []AnchorUserEntity{{Value: "issue-42", TypedLane: false}},
	}).Projections[0]
	if issue.WakeupPathUserElected {
		t.Fatalf("F2: a prose issue-N must not mint a tid handle, got %v", issue.WakeupPath)
	}

	// A TYPED entity with the same "-<digits>" tail DOES elect (the arm is
	// typed-lane only, not removed).
	typed := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            []ObservationRecord{anchorB1PathRecord("p", "worker-6", "w -> worker-6")},
		AnchorUserEntities: anchorB1TypedEntities("main-6"),
	}).Projections[0]
	if !typed.WakeupPathUserElected {
		t.Fatalf("F2: a typed thread label must still match by tid tail, got %v", typed.WakeupPath)
	}
}

// F3 (二轮复核 P2): a bare frame-number PROSE entity must not be treated as a
// pid handle (structural re-root off a display integer). A typed bare pid still
// elects.
func TestTraceCausalProjectionAnchorProseBareIntNoPidHandle(t *testing.T) {
	records := []ObservationRecord{anchorB1PathRecord("p", "renderer-3703298", "w -> renderer-3703298")}
	prose := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            records,
		AnchorUserEntities: []AnchorUserEntity{{Value: "3703298", TypedLane: false}},
	}).Projections[0]
	if prose.WakeupPathUserElected {
		t.Fatalf("F3: a prose bare frame number must not fake a pid election, got %v", prose.WakeupPath)
	}
	typed := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            records,
		AnchorUserEntities: anchorB1TypedEntities("3703298"),
	}).Projections[0]
	if !typed.WakeupPathUserElected {
		t.Fatalf("F3: a typed bare pid must still elect, got %v", typed.WakeupPath)
	}
}

// A prose entity STILL matches through the unambiguous "pid=N" handle and whole
// canonical equality — F2/F3 only close the noisy mining arms, not the precise
// ones.
func TestTraceCausalProjectionAnchorProseEntityPreciseArmsStayOpen(t *testing.T) {
	pidHandle := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            []ObservationRecord{anchorB1PathRecord("p", "pid=42591", "w -> pid=42591")},
		AnchorUserEntities: []AnchorUserEntity{{Value: "pid=42591", TypedLane: false}},
	}).Projections[0]
	if !pidHandle.WakeupPathUserElected {
		t.Fatalf("prose pid=N handle must still elect, got %v", pidHandle.WakeupPath)
	}
	canonical := CompileTraceCausalProjectionSet(ObservationLedger{
		Records:            []ObservationRecord{anchorB1PathRecord("p", "MyThread", "w -> MyThread")},
		AnchorUserEntities: []AnchorUserEntity{{Value: "mythread", TypedLane: false}},
	}).Projections[0]
	if !canonical.WakeupPathUserElected {
		t.Fatalf("prose whole-canonical equality must still elect, got %v", canonical.WakeupPath)
	}
}

// §11-N7 tid-first (TYPED entities): same tid under two comm spellings matches;
// equal comm with a DIFFERENT tid never matches.
func TestTraceCausalProjectionAnchorTidFirstMatching(t *testing.T) {
	records := []ObservationRecord{
		anchorB1PathRecord("path-other", "kworker-9", "irq-1 -> kworker-9"),
		anchorB1PathRecord("path-main", "main-6565", "waker-1 -> main-6565"),
	}
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		records, []string{"com.xs.fm.lite-6565"})
	if !got.WakeupPathUserElected || got.WakeupPath[1] != "main-6565" {
		t.Fatalf("N7 tid equality must elect across comm spellings, got %v", got.WakeupPath)
	}

	mismatch := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{anchorB1PathRecord("path-main", "main-7777", "waker-1 -> main-7777")},
		[]string{"main-6565"})
	if mismatch.WakeupPathUserElected {
		t.Fatalf("same comm with a different tid must never elect (N7 tid-first)")
	}
}

// A single-element path has no chain to root a tree on: it is never electable,
// but it keeps its legacy first-candidate anchor slot when nothing else
// matches.
func TestTraceCausalProjectionAnchorSingleElementPathNotElectable(t *testing.T) {
	records := []ObservationRecord{
		anchorB1PathRecord("path-single", "oney.hmn.berlin-42591", "oney.hmn.berlin-42591"),
		anchorB1PathRecord("path-user", "oney.hmn.berlin-42591",
			"VSyncGenerator-2270 -> oney.hmn.berlin-42591"),
	}
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(records, []string{"42591"})
	if !got.WakeupPathUserElected || len(got.WakeupPath) != 2 {
		t.Fatalf("the ≥2-element matching path must win over a single-element first candidate, got %v", got.WakeupPath)
	}
	legacy := TraceCausalProjectionFromObservationRecordsForUserEntities(records[:1], []string{"42591"})
	if legacy.WakeupPathUserElected || len(legacy.WakeupPath) != 1 {
		t.Fatalf("a lone single-element path keeps the legacy anchor slot unelected, got %v elected=%v",
			legacy.WakeupPath, legacy.WakeupPathUserElected)
	}
}

// Ledger carrier: CompileObservationLedger populates AnchorUserEntities from
// the typed request model in 裁定3 priority order with typed/prose provenance —
// RuntimeTargets user-source pid/thread (typed) first, then ExactTargets
// (typed), then AnalyzerHints.Entities (prose); cursor targets excluded (F1a);
// never RawRequest.
func TestCompileObservationLedgerAnchorEntities(t *testing.T) {
	rm := &RequestModel{}
	rm.RuntimeTargets = []RuntimeTarget{
		{Kind: RuntimeTargetKindProcess, PID: 42591, Thread: "main", Source: "user_explicit"},
		{Kind: RuntimeTargetKindThread, PID: 2270, Thread: "VSyncGenerator", Source: RuntimeTargetSourceExplicitToolCall},
	}
	rm.AnalyzerHints.ExactTargets = []string{"oney.hmn.berlin"}
	rm.AnalyzerHints.Entities = []string{"berlin.systrace", "42591"}
	ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: rm})
	want := []AnchorUserEntity{
		{Value: "42591", TypedLane: true},
		{Value: "main", TypedLane: true},
		{Value: "oney.hmn.berlin", TypedLane: true},
		{Value: "berlin.systrace", TypedLane: false},
		{Value: "42591", TypedLane: false},
	}
	if !reflect.DeepEqual(ledger.AnchorUserEntities, want) {
		t.Fatalf("AnchorUserEntities: want %+v got %+v", want, ledger.AnchorUserEntities)
	}
	if empty := CompileObservationLedger(ObservationLedgerInput{}); empty.AnchorUserEntities != nil {
		t.Fatalf("no request model → no anchor entities, got %v", empty.AnchorUserEntities)
	}
}

// F4: an over-cap pid is a parse artifact, never a runtime-target handle — it
// is dropped from the typed pid entity (the thread label, if any, still rides
// the typed lane).
func TestCompileObservationLedgerAnchorEntitiesPidCap(t *testing.T) {
	rm := &RequestModel{}
	rm.RuntimeTargets = []RuntimeTarget{{
		Kind: RuntimeTargetKindThread, PID: RuntimeTargetMaxPID + 1, Thread: "worker", Source: "user_explicit",
	}}
	ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: rm})
	overCap := strconv.Itoa(RuntimeTargetMaxPID + 1)
	for _, entity := range ledger.AnchorUserEntities {
		if entity.Value == overCap {
			t.Fatalf("F4: over-cap pid must be dropped, got %+v", ledger.AnchorUserEntities)
		}
	}
	if len(ledger.AnchorUserEntities) != 1 || ledger.AnchorUserEntities[0].Value != "worker" {
		t.Fatalf("F4: only the thread label should survive an over-cap pid, got %+v", ledger.AnchorUserEntities)
	}
}
