package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceTransactionHandoffsPredicate = "transaction_handoffs_observation"

func traceQueryTransactionHandoffsSchema(schema json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil {
		return schema
	}
	props, _ := obj["properties"].(map[string]any)
	view, _ := props["view"].(map[string]any)
	if view == nil {
		return schema
	}
	description, _ := view["description"].(string)
	view["description"] = description + " " + tracequery.TransactionHandoffsTeaching
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	if e.Encode(obj) != nil {
		return schema
	}
	return out.Bytes()
}

func (t *TraceQuery) streamTransactionHandoffs(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, callCaveat string, window traceQueryNormalizedWindow) (types.ToolResult, bool) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewTransactionHandoffs {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	result, err := tracequery.StreamTransactionHandoffs(contextFromBus(ctx), path, q)
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), true
		}
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "Transaction protocol query failed: " + err.Error(), Timestamp: time.Now()}, true
	}
	traceQueryAppendCallCaveats(&result, callCaveat)
	payload, failure := traceQueryMarshalPayload(t.Name(), result)
	if failure != nil {
		return *failure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	preview, rawRef := StoreBlob(ctx, t.Name(), traceQuerySummary(result, p, sourceLabel, payloadRef))
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now, Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now, q), TraceQuerySourceRead: traceQuerySourceReadCandidate(result)}, true
}

func traceQueryTransactionHandoffsObservations(p *tracequery.TransactionHandoffsResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidTransactionHandoffs(*p) {
		return nil
	}
	copy := *p
	var data []byte
	for {
		var err error
		data, err = json.Marshal(copy)
		if err != nil {
			return nil
		}
		if len(data) <= 64<<10 {
			break
		}
		if len(copy.Handoffs) == 0 {
			return nil
		}
		copy.Handoffs = copy.Handoffs[:len(copy.Handoffs)-1]
		copy.OmittedKeys++
	}
	ref.QueryWindowKnown = true
	ref.QueryWindowStartTs, ref.QueryWindowEndTs = p.Window.StartTs, p.Window.EndTs
	return []types.ObservationRecord{{ID: "trace_query:" + scope + "#transaction_handoffs", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs}, ClaimKey: TraceTransactionHandoffsPredicate, Predicate: TraceTransactionHandoffsPredicate, Subject: "application transaction protocol observations", Object: p.Status, Summary: "Exact published-source protocol-key observations; not complete frame, waiting or root-cause proof", RichNotes: []string{types.TraceNoteKeyTransactionHandoffs + "=" + string(data)}, ObservedAt: at, Confidence: 1}}
}

func DecodeTraceTransactionHandoffs(r types.ObservationRecord) (tracequery.TransactionHandoffsResult, bool) {
	var p tracequery.TransactionHandoffsResult
	ref := r.SourceRef
	if r.Negative || r.Predicate != TraceTransactionHandoffsPredicate || r.ClaimKey != r.Predicate || r.Subject != "application transaction protocol observations" || r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Role != types.AnswerAggregateRoleSupportingCoverage || r.GroundingPolicy != types.ClaimGroundingHard || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact || ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" || !ref.QueryWindowKnown {
		return p, false
	}
	count := 0
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyTransactionHandoffs+"=") {
			count++
			raw := json.RawMessage(strings.TrimPrefix(note, types.TraceNoteKeyTransactionHandoffs+"="))
			if len(raw) > 64<<10 || !traceRootCauseReportHasUniqueNestedKeys(raw) || json.Unmarshal(raw, &p) != nil {
				return p, false
			}
		}
	}
	selector := tracequery.RenderingCandidatesResult{TargetPID: p.TargetPID, TargetThread: p.TargetThread}
	return p, count == 1 && tracequery.ValidTransactionHandoffs(p) && p.SourcePath == ref.Path && p.Status == r.Object && renderingCandidatesTargetMatchesQuery(selector, ref) && p.TargetScope == ref.QueryTargetScope && p.Window.StartTs == ref.QueryWindowStartTs && p.Window.EndTs == ref.QueryWindowEndTs && r.Span.StartTs == p.Window.StartTs && r.Span.EndTs == p.Window.EndTs
}

func TraceTransactionHandoffsText(p tracequery.TransactionHandoffsResult) string {
	var b strings.Builder
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	fmt.Fprintf(&b, "应用事务提交与消费；查询窗口=[%.9f,%.9f%s秒；状态=%s。\n", p.Window.StartTs, p.Window.EndTs, right, p.Status)
	b.WriteString("协议键在当前完整已发布记录内核对；身份依据为实际发射线程、记录的进程归属及未观察到跨代冲突，不是原始数据库内部线程/进程ID证明。转换时被拒绝或未发布的行不在此总体内。以下关联不等于整帧完成、线程等待、GPU执行或响应根因；窗外端点只作关联背景，不计入窗口事件。\n")
	if p.Status == "unavailable" {
		fmt.Fprintf(&b, "无法建立完整记录统计：%s。\n", p.Reason)
		return b.String()
	}
	fmt.Fprintf(&b, "窗口内提交事件=%d，消费事件=%d；同一消费包含多键时事件只计一次，键分支分别保留；非法协议事件=%d。\n", p.WindowSubmissionEvents, p.WindowConsumptionEvents, p.MalformedProtocolEvents)
	labels := map[string]string{"observed_unique_protocol_match": "观测内唯一交接（协议键匹配）", "ambiguous": "有歧义，不选首个", "missing_submission": "未观察到提交", "missing_consumption": "未观察到消费", "identity_unverified": "身份关联未确认", "order_unverified": "先后关系未确认"}
	for _, h := range p.Handoffs {
		fmt.Fprintf(&b, "- 事务[tid=%d, seq=%s]：%s；全已发布记录提交=%d/消费=%d；窗口内提交=%d/消费=%d。%s\n", h.TID, h.Sequence, labels[h.Status], h.SubmissionCount, h.ConsumptionCount, h.WindowSubmissions, h.WindowConsumptions, h.Reason)
		for i, es := range [][]tracequery.TransactionEndpoint{h.Submissions, h.Consumptions} {
			role := "提交"
			if i == 1 {
				role = "消费"
			}
			for _, e := range es {
				membership := "窗外关联背景"
				if e.InWindow {
					membership = "窗口内"
				}
				fmt.Fprintf(&b, "  %s %.9f秒，线程%q tid=%d，记录pid=%d，来源行=%d，%s；标记=%q。\n", role, e.Ts, e.Thread, e.TID, e.TGID, e.Line, membership, e.Name)
			}
		}
		fmt.Fprintf(&b, "  展示省略提交=%d/消费=%d；省略不改变键的歧义结论。\n", h.OmittedSubmissions, h.OmittedConsumptions)
	}
	fmt.Fprintf(&b, "事务键总数=%d，当前展示=%d，省略=%d；只可画已匹配提交→消费，不得把同批多事务画成互相调用，也不得拼接邻近帧或GPU阶段。\n", p.TotalKeys, len(p.Handoffs), p.OmittedKeys)
	return b.String()
}
