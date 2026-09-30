package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceProcessProfilePredicate = "process_profile_observation"

func traceQueryProcessProfileSchema(schema json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil {
		return schema
	}
	properties, _ := obj["properties"].(map[string]any)
	view, _ := properties["view"].(map[string]any)
	if view == nil {
		return schema
	}
	description, _ := view["description"].(string)
	view["description"] = description + " " + tracequery.ProcessProfileTeaching
	for _, name := range []string{"pid", "thread"} {
		if field, ok := properties[name].(map[string]any); ok {
			desc, _ := field["description"].(string)
			field["description"] = desc + " For process_profile, one source thread selector (pid or exact thread) is required unless already inherited from a unique typed analysis target; this selects its native process, not only that thread's statistics."
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return schema
	}
	return out
}

// Check only typed call fields, after existing target inheritance. A missing
// selector is a repairable invocation error, not absent process evidence.
func traceQueryProcessProfileInputRepair(p traceQueryParams) *types.ToolResult {
	if tracequery.CanonicalViewName(p.View) != "process_profile" || p.PID.Int() > 0 || strings.TrimSpace(p.Thread) != "" {
		return nil
	}
	hint := "process_profile needs a source thread selector. Retry the same source/window with pid=<observed TID> or thread=<exact observed thread>. If not yet known, locate the requested thread with event_search first. No process census was performed; missing call parameters do not mean missing capture data."
	return &types.ToolResult{ToolName: "trace_query", Success: false, Summary: hint, Timestamp: time.Now(), Repair: &types.ToolRepair{Code: "trace_query_source_thread_required", Fields: []string{"pid", "thread"}, Hint: hint, Metadata: map[string]string{"view": "process_profile", "retry_scope": "same_source_and_window", "selector_semantics": "one_source_thread"}}}
}

func writeTraceProcessProfilePreview(b *strings.Builder, p *tracequery.ProcessProfile) {
	if p == nil {
		return
	}
	b.WriteString("## Process overview / 进程概览\n")
	b.WriteString(TraceProcessProfileText(*p, 12))
	b.WriteByte('\n')
}

// One source/window-bound typed record preserves all bounded producer rows.
// The final-context renderer owns its independent display limit.
func traceQueryProcessProfileObservations(p *tracequery.ProcessProfile, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	return []types.ObservationRecord{{
		ID: "trace_query:" + scope + "#process_profile", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan,
		SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs},
		ClaimKey: TraceProcessProfilePredicate, Predicate: TraceProcessProfilePredicate,
		Subject: fmt.Sprintf("process %d", p.TGID), Object: p.Status,
		Summary:    "Source-bound observed process members, scheduler state accounts and business groups; not a causal ranking",
		RichNotes:  []string{types.TraceNoteKeyProcessProfile + "=" + string(data)},
		ObservedAt: at, Confidence: 1,
	}}
}

func DecodeTraceProcessProfile(record types.ObservationRecord) (tracequery.ProcessProfile, bool) {
	var p tracequery.ProcessProfile
	ref := record.SourceRef
	if record.Predicate != TraceProcessProfilePredicate || record.ClaimKey != TraceProcessProfilePredicate ||
		record.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
		record.Role != types.AnswerAggregateRoleSupportingCoverage || record.GroundingPolicy != types.ClaimGroundingHard ||
		record.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact ||
		ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" || !ref.QueryWindowKnown {
		return p, false
	}
	for _, note := range record.RichNotes {
		if !strings.HasPrefix(note, types.TraceNoteKeyProcessProfile+"=") {
			continue
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(note, types.TraceNoteKeyProcessProfile+"=")), &p) != nil {
			return p, false
		}
		return p, tracequery.ValidProcessProfile(p) && p.SourcePath == ref.Path && p.Window.StartTs == ref.QueryWindowStartTs && p.Window.EndTs == ref.QueryWindowEndTs &&
			p.Window.StartTs == record.Span.StartTs && p.Window.EndTs == record.Span.EndTs && record.Object == p.Status && record.Subject == fmt.Sprintf("process %d", p.TGID)
	}
	return p, false
}

func TraceProcessProfileText(p tracequery.ProcessProfile, limit int) string {
	var b strings.Builder
	if p.Status == "unavailable" {
		fmt.Fprintf(&b, "进程概览未能计量；窗口=[%.9f, %.9f) 秒；reason=%s。成员数、状态及业务分布未知，不能按零处理，也不据此推断原始trace缺数据。\n", p.Window.StartTs, p.Window.EndTs, p.Reason)
		return b.String()
	}
	fmt.Fprintf(&b, "进程 %d；窗口=[%.9f, %.9f) 秒，完整窗口 %.9g 毫秒；status=%s reason=%s；已观测成员=%d，原生展示=%d，原生省略=%d，状态未能计量成员=%d；全来源进程归属未确定线程=%d（不算入本进程）。\n", p.TGID, p.Window.StartTs, p.Window.EndTs, p.WindowMs, p.Status, p.Reason, p.ThreadCount, p.EmittedThreads, p.OmittedThreads, p.UnavailableThreads, p.UnknownMembershipThreads)
	shown := 0
	for _, row := range p.Threads {
		if shown >= limit {
			break
		}
		data, _ := json.Marshal(row)
		fmt.Fprintf(&b, "- thread_account=%s\n", data)
		shown++
	}
	fmt.Fprintf(&b, "上下文展示成员=%d，另省略原生已发布成员=%d；原生统计不受上下文省略影响。\n", shown, len(p.Threads)-shown)
	for _, c := range p.Caveats {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	b.WriteString("回答请使用业务语言：线程运行、等调度、睡眠、IO等待、业务片段；图只表达已观测进程成员/状态/调用者分组，不画成唤醒因果链。\n")
	return b.String()
}
