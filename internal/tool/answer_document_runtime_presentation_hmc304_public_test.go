package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are ordinary public lookups of the same capture, not hand-authored
// graph credentials. A row-local search correctly leaves the wakee process
// unknown, while the existing chain producer publishes its resolved identity.
// Presentation must not let an opaque query hash hide that available detail.
func TestHMC304PublicWakeupPresentationKeepsCompletePublishedIdentity(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedHitrace: string(data), Mutable: types.NewMutableState("展示这次打开文档的唤醒时序")}
	queries := []map[string]any{
		{"source": "attached_trace", "view": "event_search", "limit": 60},
		{"source": "attached_trace", "view": "wakeup_chain", "pid": 100, "time_start": .999, "time_end": 1.051},
		{"source": "attached_trace", "view": "root_cause_rank", "pid": 100, "time_start": .999, "time_end": 1.051},
		{"source": "attached_trace", "platform": "harmony", "view": "event_search", "time_start": .999, "time_end": 1.051, "limit": 60},
		{"source": "attached_trace", "platform": "harmony", "view": "wakeup_chain", "pid": 100, "time_start": .999, "time_end": 1.051},
		{"source": "attached_trace", "platform": "harmony", "view": "root_cause_rank", "pid": 100, "time_start": .999, "time_end": 1.051},
	}
	for _, q := range queries {
		result := businessRefTestQuery(t, ctx, q)
		if !result.Success {
			t.Fatalf("public query %v: %s", q, result.Summary)
		}
	}
	rows := runtimeDiagramRelationsForContext(ctx)
	before := append([]RuntimeDiagramRelation(nil), rows...)
	known, unknown := 0, 0
	for _, row := range rows {
		if row.threadEndpoints == nil {
			continue
		}
		if row.threadEndpoints.to.TGID > 0 {
			known++
		} else {
			unknown++
		}
	}
	if known == 0 || unknown == 0 {
		t.Fatalf("public fixture must retain both independently valid precisions: known=%d unknown=%d", known, unknown)
	}
	selected := runtimeDiagramPresentationRows(rows)
	if !reflect.DeepEqual(rows, before) {
		t.Fatal("soft presentation changed the complete authority pool")
	}
	if len(selected) != 2 {
		t.Fatalf("want two physical observations, got %+v", selected)
	}
	for _, row := range selected {
		t.Logf("selected query=%s from=%+v to=%+v", row.presentationQuery, row.threadEndpoints.from, row.threadEndpoints.to)
		if row.threadEndpoints.to.TGID <= 0 {
			t.Errorf("presentation hid an existing complete producer identity behind an unknown row: %+v", row.threadEndpoints.to)
		}
	}
	if selected[0].ToNode != selected[1].FromNode {
		t.Errorf("one coherent native query already supplies the worker lane; display selected incompatible aliases: %s != %s", selected[0].ToNode, selected[1].FromNode)
	}
	// The less complete query remains independently valid after selection;
	// selecting a better row never merges its endpoints into another query.
	for _, row := range rows {
		if !runtimeDiagramRelationProved(rows, row.FromIdentity, row.ToIdentity, row.Kind) {
			t.Fatal("soft selection invalidated an original exact pair")
		}
	}
	doc := requestedThreadDocHMC242(selected, true)
	doc.Blocks[0].Text = "记录两次唤醒，线程及进程身份均沿用同一原生查询；唤醒关系不表示它造成了全部等待。"
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !emitted.Success {
		t.Fatalf("public emission rejected its complete native query: %v %s", err, emitted.Summary)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	if accepted == nil || len(accepted.Blocks) < 2 || accepted.Blocks[1].Diagram == nil {
		t.Fatal("public acceptance lost the native graph")
	}
	if got := len(mermaidcompat.RemovableNodeDeclarations(accepted.Blocks[1].Diagram.Body)); got != 3 {
		t.Fatalf("two exact events should display three native participants, got %d", got)
	}
}

func TestHMC304PublicWakeupPresentationNeverInfersMissingProcess(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
storage-80 (2) [002] .... 1.040010: sched_wakeup: comm=worker pid=200 prio=120 target_cpu=002
worker-200 (100) [002] .... 1.045000: sched_wakeup: comm=main pid=100 prio=53 target_cpu=001
`)
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search"})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	rows := runtimeDiagramRelationsForContext(ctx)
	selected := runtimeDiagramPresentationRows(rows)
	if len(selected) != 2 || !reflect.DeepEqual(selected, rows) {
		t.Fatalf("single query lost or changed its exact credentials: %+v", selected)
	}
	if selected[0].threadEndpoints.to.TGID != 0 || selected[1].threadEndpoints.to.TGID != 0 {
		t.Fatal("row-local unknown process was borrowed from another observation")
	}
	if selected[0].ToNode == selected[1].FromNode {
		t.Fatal("matching TID/name merged an unknown and a known process identity")
	}
}

func TestHMC304PublicWakeupPresentationKeepsDifferentProcessesSeparate(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
worker-200 (100) [002] .... 1.040010: sched_wakeup: comm=main pid=10 prio=53 target_cpu=001
worker-200 (300) [002] .... 1.045000: sched_wakeup: comm=main pid=10 prio=53 target_cpu=001
`)
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search"})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	rows := runtimeDiagramRelationsForContext(ctx)
	selected := runtimeDiagramPresentationRows(rows)
	if len(selected) != 2 || selected[0].threadEndpoints.from.TGID != 100 || selected[1].threadEndpoints.from.TGID != 300 {
		t.Fatalf("distinct native process identities were lost: %+v", selected)
	}
	if selected[0].FromNode == selected[1].FromNode || selected[0].FromIdentity == selected[1].FromIdentity {
		t.Fatal("TID/name similarity merged different producer process identities")
	}
	if runtimeDiagramRelationProved(rows, selected[0].FromIdentity, selected[1].ToIdentity, selected[0].Kind) {
		t.Fatal("presentation allowed borrowing another event endpoint")
	}
}

func TestHMC304PresentationCoveragePrecedesIdentityCompleteness(t *testing.T) {
	known := &runtimeDiagramThreadEndpoints{from: types.RuntimeWakeupThread{TGID: 100}, to: types.RuntimeWakeupThread{TGID: 200}}
	rows := []RuntimeDiagramRelation{
		{FromIdentity: "known-a", presentationKey: "a", presentationQuery: "z-narrow", threadEndpoints: known},
		{FromIdentity: "known-b", presentationKey: "b", presentationQuery: "z-narrow", threadEndpoints: known},
		{FromIdentity: "broad-a", presentationKey: "a", presentationQuery: "a-broad"},
		{FromIdentity: "broad-b", presentationKey: "b", presentationQuery: "a-broad"},
		{FromIdentity: "broad-c", presentationKey: "c", presentationQuery: "a-broad"},
	}
	selected := runtimeDiagramPresentationRows(rows)
	if len(selected) != 3 || selected[0].FromIdentity != "broad-a" || selected[1].FromIdentity != "broad-b" || selected[2].FromIdentity != "broad-c" {
		t.Fatalf("identity ranking fragmented a more complete query: %+v", selected)
	}
	// With equal physical coverage, prefer the complete published query as a
	// whole; do not choose each endpoint independently or borrow process IDs.
	rows = rows[:4]
	selected = runtimeDiagramPresentationRows(rows)
	if len(selected) != 2 || selected[0].FromIdentity != "known-a" || selected[1].FromIdentity != "known-b" {
		t.Fatalf("equal-coverage display lost the complete producer query: %+v", selected)
	}
}
