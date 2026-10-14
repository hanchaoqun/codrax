package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativePresentationMixedWindowQueryDoesNotBorrowTreeCompletenessHMC240(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
	boundary := "document-worker-11 (10) [000] .... 3.000000:"
	io := "document-worker-11 (10) [000] .... 1.600000: android_fs_datawrite_start: entry_name=report.db offset=0 bytes=4096 cmdline=app pid=11 i_size=8192 ino=0xb9b8e\n" +
		"document-worker-11 (10) [000] .... 1.610000: android_fs_datawrite_end: ino=0xb9b8e offset=0 bytes=4096 ret=4096 latency_us=700\n" +
		"document-worker-11 (10) [000] .... 1.620000: mm_filemap_add_to_page_cache: dev 260:136 ino 0xb9b8e page=0 pfn=1 ofs=0\n"
	if !strings.Contains(ctx.AttachedHitrace, boundary) {
		t.Fatal("fixture insertion point absent")
	}
	ctx.AttachedHitrace = strings.Replace(ctx.AttachedHitrace, boundary, io+boundary, 1)
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"window_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !result.Success {
		t.Fatalf("public mixed query: %v %s", err, result.Summary)
	}
	const synthesis = "文件 report.db 的写入观测含 4096 字节、一次完成；页缓存观测与采样权重分开解释。"
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}, InvestigationNotes: []string{synthesis, "独立范围说明保持不变"}})
	ctx.Mutable.SetInvestigationComplete(synthesis)
	ctx.Mutable.SetInvestigationResultKind("resolved")
	ledger := answerDocObservationLedger(ctx)
	var fileKey, treeKey string
	for _, record := range ledger.Records {
		if record.Predicate == tool.TracePerfSamplingTreePredicate {
			treeKey = nativePresentationQueryKey(record)
		}
		if record.Predicate == "file_io_by_inode" {
			fileKey = nativePresentationQueryKey(record)
		}
		if record.SourceRef.QueryScopeID != "" {
			t.Logf("predicate=%s queryKey=%s", record.Predicate, nativePresentationQueryKey(record))
		}
	}
	if fileKey == "" || treeKey == "" || fileKey != treeKey {
		t.Fatal("fixture must publish same-query file IO and sampling tree")
	}
	var onlyTree answerDocNativePresentationReceipt
	text := renderAnswerDocPerfSamplingTrees(ctx, ledger, &onlyTree)
	if !strings.Contains(text, "total_weight=35") || strings.Contains(text, "report.db") {
		t.Fatal("fixture must fully show sampling but not file-IO facts")
	}
	if onlyTree.completeFor(ctx) {
		t.Error("sampling-only presentation declared complete for an independently observed file-IO family")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, synthesis) {
		t.Error("actual finalizer removed a non-sampling advisory using only same-query tree completeness")
	}
	if !strings.Contains(prompt, "独立范围说明保持不变") {
		t.Fatal("independent notes disappeared")
	}
}

func TestNativePresentationEventMirrorsRequireExactMemberHMC240(t *testing.T) {
	ctx, _ := eventPublicationHMC240Context(t)
	ledger := answerDocObservationLedger(ctx)
	var publication answerDocNativePresentationReceipt
	renderAnswerDocTraceEventInventoryPublicationViewWithPublication(ledger, nil, &publication)
	var mirror types.ObservationRecord
	for _, record := range ledger.Records {
		if record.Predicate == "trace_mark" {
			mirror = record
			break
		}
	}
	if key := publication.eventMirrorKey(mirror); key == "" || !publication.queries[key] {
		t.Fatal("exact original event mirror must reuse its complete inventory")
	}
	for name, change := range map[string]func(*types.ObservationRecord){
		"source":   func(r *types.ObservationRecord) { r.SourceRef.Path += ":other" },
		"query":    func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID += ":other" },
		"clock":    func(r *types.ObservationRecord) { r.SourceRef.TimeDomain += ":other" },
		"line":     func(r *types.ObservationRecord) { r.Span.LineStart += 100; r.Span.LineEnd += 100 },
		"time":     func(r *types.ObservationRecord) { r.Span.StartTs += .01; r.Span.EndTs += .01 },
		"interval": func(r *types.ObservationRecord) { r.Span.EndTs += .01 },
		"event_type": func(r *types.ObservationRecord) {
			r.Predicate = "file_io_by_inode"
			r.ClaimKey = "evidence_fact:" + r.Predicate
		},
		"not_mirror": func(r *types.ObservationRecord) { r.ClaimKey = "independent_fact" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := mirror
			change(&changed)
			if key := publication.eventMirrorKey(changed); key != "" {
				t.Fatal("unpublished point or family borrowed event inventory completeness")
			}
		})
	}
}
