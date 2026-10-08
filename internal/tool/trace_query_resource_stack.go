package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceResourceStackPredicate = "resource_stack_observation"

func traceQueryResourceStackSchema(schema json.RawMessage) json.RawMessage {
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
	view["description"] = description + " " + tracequery.ResourceStackTeaching
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(obj) != nil {
		return schema
	}
	return out.Bytes()
}

func traceQueryResourceStackObservations(p *tracequery.ResourceStackResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidResourceStack(*p) {
		return nil
	}
	copy := boundedResourceStackHandoff(*p)
	// This observation has a dedicated inclusive/point envelope; it cannot
	// grant continuous execution or causal-window authority.
	ref.QueryWindowKnown = true
	ref.QueryWindowStartTs, ref.QueryWindowEndTs = p.Window.StartTs, p.Window.EndTs
	data, err := json.Marshal(copy)
	if err != nil {
		return nil
	}
	return []types.ObservationRecord{{ID: "trace_query:" + scope + "#resource_stack", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs}, ClaimKey: TraceResourceStackPredicate, Predicate: TraceResourceStackPredicate, Subject: "native resource stacks", Object: p.Status, Summary: "Same-capture owner-bound resource stack observations, not execution or response causality", RichNotes: []string{types.TraceNoteKeyResourceStack + "=" + string(data)}, ObservedAt: at, Confidence: 1}}
}

func DecodeTraceResourceStack(r types.ObservationRecord) (tracequery.ResourceStackResult, bool) {
	var p tracequery.ResourceStackResult
	ref := r.SourceRef
	if r.Negative || r.Predicate != TraceResourceStackPredicate || r.ClaimKey != r.Predicate || r.Subject != "native resource stacks" || r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Role != types.AnswerAggregateRoleSupportingCoverage || r.GroundingPolicy != types.ClaimGroundingHard || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact || ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" || !ref.QueryWindowKnown || !ref.QueryLineRangeKnown || ref.QueryLineStart != 0 || ref.QueryLineEnd != 0 {
		return p, false
	}
	count := 0
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyResourceStack+"=") {
			count++
			raw := json.RawMessage(strings.TrimPrefix(note, types.TraceNoteKeyResourceStack+"="))
			if !traceRootCauseReportHasUniqueNestedKeys(raw) || json.Unmarshal(raw, &p) != nil {
				return p, false
			}
		}
	}
	return p, count == 1 && tracequery.ValidResourceStack(p) && p.SourcePath == ref.Path && p.Status == r.Object && p.TargetPID == ref.QueryTargetPID && p.TargetThread == ref.QueryTargetThread && p.TargetScope == ref.QueryTargetScope && p.Window.StartTs == ref.QueryWindowStartTs && p.Window.EndTs == ref.QueryWindowEndTs && p.Window.StartTs == r.Span.StartTs && p.Window.EndTs == r.Span.EndTs
}

func TraceResourceStackText(p tracequery.ResourceStackResult, limit int) string {
	var b strings.Builder
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	fmt.Fprintf(&b, "资源调用栈；窗口=[%.9f, %.9f%s秒。\n", p.Window.StartTs, p.Window.EndTs, right)
	if p.Window.EndInclusive {
		b.WriteString("默认观测包络包含最后已观测时刻；不证明完整采集范围，单点不生成时长。\n")
	}
	if p.Status == "unavailable" {
		fmt.Fprintf(&b, "无法展开：%s；不是没有资源活动。\n", p.Reason)
		return b.String()
	}
	if limit <= 0 {
		limit = 8
	}
	shown := 0
	fmt.Fprintf(&b, "匹配资源事件=%d；源帧统计与展示省略分别计数。\n", p.MatchedEvents)
	for _, e := range p.Events {
		if shown >= limit {
			break
		}
		shown++
		fmt.Fprintf(&b, "源事件物理rowid=%d；原始id=%s；\n", e.RowID, resourceScalarText(e.Source.SourceID))
		fmt.Fprintf(&b, "事件行%d；时间=%sns；线程=%s（tid=%d，pid=%d）；操作=%s；栈键=%s；资源大小=%s；地址signed=%s", e.Line, fmt.Sprint(e.TimestampNS), resourceStackEscape(e.Source.Thread), e.Source.TID, e.Source.PID, e.Source.Operation, resourceScalarText(e.Source.CallchainID), resourceScalarText(e.Source.Size), resourceScalarText(e.Source.Address))
		if n, ok := e.Source.Address.Integer(); ok {
			fmt.Fprintf(&b, "，地址位型=0x%016x", uint64(n))
		}
		b.WriteString("；CPU未知，未据此证明执行。\n")
		fmt.Fprintf(&b, "栈记录=%s；源帧数=%d，缺深度数=%d，重复深度数=%d，无效深度数=%d，未知符号数=%d；源帧载体完整=%t，真实展开终点=未知。\n", e.Source.StackStatus, e.Source.FrameCount, e.MissingDepths, e.DuplicateDepths, e.InvalidDepths, e.UnknownSymbols, e.SourceFramesComplete)
		b.WriteString("|源深度|符号|库|IP（signed）|位型|offset|symbol offset|vaddr|物理rowid/原始id|证据行|\n|---:|---|---|---|---|---|---|---|---|---:|\n")
		for _, f := range e.Frames {
			bits := "未知"
			if n, ok := f.IP.Integer(); ok {
				bits = fmt.Sprintf("0x%016x", uint64(n))
			}
			fmt.Fprintf(&b, "|%s|%s|%s|%s|%s|%s|%s|%s|%d / %s|%d|\n", resourceScalarText(f.Depth), resourceTextText(f.Symbol), resourceTextText(f.Library), resourceScalarText(f.IP), bits, resourceScalarText(f.Offset), resourceScalarText(f.SymbolOffset), resourceTextText(f.VAddr), f.RowID, resourceScalarText(f.SourceID), f.Line)
		}
		fmt.Fprintf(&b, "本事件展示省略帧=%d；深度形态=%s；不按最深帧或固定倒数层指定业务根因。\n", e.OmittedFrames, e.DepthStatus)
	}
	fmt.Fprintf(&b, "展示事件=%d，省略事件=%d。资源时间是事件时间，栈帧没有独立执行时长；资源生命周期不等于线程运行/等待，栈关联不证明泄漏或响应根因。\n", shown, p.MatchedEvents-shown)
	return b.String()
}

func resourceScalarText(s tracewire.ResourceScalar) string {
	if s.Status == "known" {
		return s.Value
	}
	return "未知（" + s.Status + "）"
}
func resourceTextText(s tracewire.ResourceText) string {
	if s.Status == "known" {
		if s.Value == "" {
			return "空文本"
		}
		return resourceStackEscape(s.Value)
	}
	return "未知（" + s.Status + "）"
}
func resourceStackEscape(s string) string {
	return strings.NewReplacer("|", "&#124;", "\r", " ", "\n", "<br/>", "<", "&lt;", ">", "&gt;").Replace(s)
}
