package tool

// answer_document_projection_runnable_occupier_rn1_test.go — RN-1(b) (§7.9,
// cust_runnable 2026-07-04) consumption pins: the compiled projection promotes
// a "runnable_occupancy" observation onto the subject-matched runnable node as
// the typed OccupierSummary, and the tree renders it as the runnable row's
// tail tag ("同窗占用者: …(cpu·ms)"). The observation itself never becomes a
// node; sleep rows of other subjects never inherit the roster.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runnableOccupierRN1Records() []types.ObservationRecord {
	// Customer flat shape: a runnable-dominant primary row with no wakeup edge.
	runnable := projV3Obs("root-ffrt", "root_cause_primary", "root_cause_primary:ffrt",
		"OS_FFRT_2_3-49706", "runnable_wait", "635.981", 635.981, 1200, 9800,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=runnable")
	sleep := projV3Obs("root-rs", "root_cause_primary", "root_cause_primary:rs",
		"render_service-411", "sleep_wait", "120.000", 120.0, 300, 400,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=s_sleep")
	occupancy := types.ObservationRecord{
		ID: "trace_query:w1#runnable_occupancy:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		Predicate: "runnable_occupancy", ClaimKey: "runnable_occupancy",
		Subject: "OS_FFRT_2_3-49706", Object: "runnable", Value: "2528.000", Unit: "ms",
		Span: types.ObservationSpan{LineStart: 1200, LineEnd: 9800},
		RichNotes: []string{
			"starved_runnable_ms=2528.000",
			"occupier_1=RSHardwareThre-1063:1410.250ms",
			"occupier_2=render_service-411:902.125ms",
			"occupier_3=kworker/u16:3-77:401.000ms",
			"window_ms=3000.000",
			"selected_window=100.000000..103.000000",
		},
	}
	return []types.ObservationRecord{runnable, sleep, occupancy}
}

func TestTraceProjectionCompilesRunnableOccupierSummary(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(runnableOccupierRN1Records())
	var ffrt, rs *types.TraceCausalProjectionNode
	for i := range projection.PrimaryRootCauses {
		switch projection.PrimaryRootCauses[i].Subject {
		case "OS_FFRT_2_3-49706":
			ffrt = &projection.PrimaryRootCauses[i]
		case "render_service-411":
			rs = &projection.PrimaryRootCauses[i]
		}
	}
	if ffrt == nil || rs == nil {
		t.Fatalf("both primary nodes must compile: %+v", projection.PrimaryRootCauses)
	}
	want := "RSHardwareThre-1063:1410.250ms、render_service-411:902.125ms、kworker/u16:3-77:401.000ms"
	if ffrt.OccupierSummary != want {
		t.Fatalf("runnable node must carry the typed occupier roster, got %q", ffrt.OccupierSummary)
	}
	if rs.OccupierSummary != "" {
		t.Fatalf("a non-runnable node must not inherit the roster: %q", rs.OccupierSummary)
	}
	// The observation is a side-channel, never a node of its own.
	total := len(projection.PrimaryRootCauses) + len(projection.OnChainCauses) +
		len(projection.AdjacentCauses) + len(projection.BackgroundCauses) + len(projection.SupportingHops)
	for _, nodes := range [][]types.TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.OnChainCauses, projection.AdjacentCauses,
		projection.BackgroundCauses, projection.SupportingHops,
	} {
		for _, node := range nodes {
			if node.Predicate == "runnable_occupancy" {
				t.Fatalf("runnable_occupancy must never compile into a node (total=%d): %+v", total, node)
			}
		}
	}
	// Subject mismatch attaches nothing.
	records := runnableOccupierRN1Records()
	records[2].Subject = "some-other-thread-1"
	mismatch := types.TraceCausalProjectionFromObservationRecords(records)
	for _, node := range mismatch.PrimaryRootCauses {
		if node.OccupierSummary != "" {
			t.Fatalf("subject-mismatched roster must not attach: %+v", node)
		}
	}
}

// rn1CollapseContinuations removes the F-2 ↳ continuation-line scaffolding
// (newline + tree rails + "↳ "/alignment spaces) so a wrap-split carrier can
// be matched as its original byte sequence — the wrap contract is
// byte-identical concatenation, which is exactly what this reverses.
func rn1CollapseContinuations(md string) string {
	lines := strings.Split(md, "\n")
	var b strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, "│ \t")
		if wrapped, ok := strings.CutPrefix(trimmed, "↳ "); ok {
			b.WriteString(wrapped)
			continue
		}
		if b.Len() > 0 && trimmed != line && !strings.HasPrefix(trimmed, "├") && !strings.HasPrefix(trimmed, "└") &&
			trimmed != "" && !strings.ContainsAny(trimmed, "█░🎯💤⏳⚙⛓◇▒") {
			// alignment continuation of a wrapped carrier (indent + "  " lane)
			b.WriteString(trimmed)
			continue
		}
		b.WriteString("\n")
		b.WriteString(line)
	}
	return b.String()
}

func TestTraceProjectionRunnableOccupierTailTagZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), runnableOccupierRN1Records(), "")
	collapsed := rn1CollapseContinuations(md)
	if !strings.Contains(collapsed, "同窗占用者: RSHardwareThre-1063:1410.250ms、render_service-411:902.125ms、kworker/u16:3-77:401.000ms(cpu·ms)") {
		t.Fatalf("runnable row must carry the same-window occupier tail note:\n%s", md)
	}
	// The note is a runnable-row tail: it must sit on the FFRT row's
	// continuation lane, before the sleep row.
	ffrt := strings.Index(md, "OS_FFRT_2_3-49706")
	note := strings.Index(md, "同窗占用者")
	sleepRow := strings.Index(md, "render_service-411 · 睡眠等待")
	if ffrt < 0 || note < ffrt || (sleepRow >= 0 && note > sleepRow) {
		t.Fatalf("occupier note must attach to the runnable row (ffrt=%d note=%d sleep=%d):\n%s", ffrt, note, sleepRow, md)
	}
}

func TestTraceProjectionRunnableOccupierTailTagEN(t *testing.T) {
	md := audit730Render(t, audit730Bus("en"), runnableOccupierRN1Records(), "en")
	collapsed := rn1CollapseContinuations(md)
	if !strings.Contains(collapsed, "same-window occupiers: RSHardwareThre-1063:1410.250ms、render_service-411:902.125ms、kworker/u16:3-77:401.000ms (cpu·ms)") {
		t.Fatalf("EN runnable row must carry the occupier tail note:\n%s", md)
	}
	if strings.Contains(md, "同窗占用者") {
		t.Fatalf("EN surface must not carry zh labels:\n%s", md)
	}
}

// Byte-stability control: without a runnable_occupancy observation the tag
// never appears.
func TestTraceProjectionRunnableOccupierTagOnlyWhenPublished(t *testing.T) {
	records := runnableOccupierRN1Records()[:2]
	md := audit730Render(t, audit730Bus(""), records, "")
	for _, banned := range []string{"同窗占用者", "same-window occupiers"} {
		if strings.Contains(md, banned) {
			t.Fatalf("occupier tag must only render when the observation exists (%q leaked):\n%s", banned, md)
		}
	}
}

// Unit pin: the tag builder consumes only the typed OccupierSummary field.
func TestRuntimeTraceProjOccupierTagUnit(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, Subject: "OS_FFRT_2_3-49706",
		StateKind: "runnable", ImpactMS: 635.981,
		OccupierSummary: "RSHardwareThre-1063:1410.250ms、render_service-411:902.125ms",
	}
	tag, ok := runtimeTraceProjOccupierTag(node, true)
	if !ok || tag.Text != "同窗占用者: RSHardwareThre-1063:1410.250ms、render_service-411:902.125ms(cpu·ms)" {
		t.Fatalf("zh occupier tag wrong: ok=%t %q", ok, tag.Text)
	}
	if tag.DropOrder != runtimeTraceProjTagKeep || !tag.NoTruncate || !tag.ContinuationLane {
		t.Fatalf("occupier tag must be Keep + NoTruncate + ContinuationLane: %+v", tag)
	}
	if en, ok := runtimeTraceProjOccupierTag(node, false); !ok ||
		en.Text != "same-window occupiers: RSHardwareThre-1063:1410.250ms、render_service-411:902.125ms (cpu·ms)" {
		t.Fatalf("en occupier tag wrong: %q", en.Text)
	}
	node.OccupierSummary = "  "
	if _, ok := runtimeTraceProjOccupierTag(node, true); ok {
		t.Fatalf("empty roster must not build a tag")
	}
}

// F-3 (统一复核 2026-07-04) pins: an RN-4 comm-truncation placeholder leaking
// into the occupier roster ("<...>-49706:…") rewrites through the single R1
// wording helper on the DISPLAY surface only — real names (colons included)
// stay verbatim, and the producer-side roster field keeps the raw token.
func TestRuntimeTraceProjOccupierRosterPlaceholderRewrite(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, Subject: "OS_FFRT_2_3-6565",
		StateKind: "runnable", ImpactMS: 635.981,
		OccupierSummary: "<...>-49706:1410.250ms、binder:486_1-10803:902.125ms",
	}
	tag, ok := runtimeTraceProjOccupierTag(node, true)
	if !ok || tag.Text != "同窗占用者: 线程名未记录(tid 49706):1410.250ms、binder:486_1-10803:902.125ms(cpu·ms)" {
		t.Fatalf("zh roster must rewrite the placeholder and keep real names verbatim: %q", tag.Text)
	}
	if strings.Contains(tag.Text, "<...>") {
		t.Fatalf("the raw placeholder must never render: %q", tag.Text)
	}
	if en, ok := runtimeTraceProjOccupierTag(node, false); !ok ||
		en.Text != "same-window occupiers: unnamed thread (tid 49706):1410.250ms、binder:486_1-10803:902.125ms (cpu·ms)" {
		t.Fatalf("en roster rewrite wrong: %q", en.Text)
	}
	// The typed field itself is untouched (display-only rewrite — audit keys
	// and producer notes keep the verbatim token).
	if node.OccupierSummary != "<...>-49706:1410.250ms、binder:486_1-10803:902.125ms" {
		t.Fatalf("OccupierSummary must stay verbatim: %q", node.OccupierSummary)
	}
	// Non-placeholder lookalikes never rewrite: name with a non-digit tail or
	// without the exact "<...>-" prefix.
	for _, roster := range []string{
		"<...>-49706x:10.000ms", // tail not pure digits
		"a<...>-49706:10.000ms", // prefix not exact
		"<...>-49706",           // no ms value member (no ':' split)
	} {
		got := runtimeTraceProjOccupierRosterDisplay(roster, true)
		if got != roster {
			t.Fatalf("lookalike %q must stay verbatim, got %q", roster, got)
		}
	}
}

// F-3 end-to-end: the placeholder occupier renders rewritten on the fence
// while the raw producer roster stays in the ledger record.
func TestTraceProjectionOccupierPlaceholderEndToEnd(t *testing.T) {
	records := runnableOccupierRN1Records()
	records[2].RichNotes = []string{
		"starved_runnable_ms=2528.000",
		"occupier_1=<...>-49706:1410.250ms",
		"occupier_2=render_service-411:902.125ms",
		"window_ms=3000.000",
		"selected_window=100.000000..103.000000",
	}
	md := audit730Render(t, audit730Bus(""), records, "")
	collapsed := rn1CollapseContinuations(md)
	if !strings.Contains(collapsed, "同窗占用者: 线程名未记录(tid 49706):1410.250ms、render_service-411:902.125ms(cpu·ms)") {
		t.Fatalf("placeholder occupier must render rewritten:\n%s", md)
	}
	if strings.Contains(md, "<...>-49706") {
		t.Fatalf("raw placeholder must not leak into the render:\n%s", md)
	}
}
