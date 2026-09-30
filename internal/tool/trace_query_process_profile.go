package tool

import (
	"encoding/json"
	"fmt"
	"strings"

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
	out, err := json.Marshal(obj)
	if err != nil {
		return schema
	}
	return out
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
