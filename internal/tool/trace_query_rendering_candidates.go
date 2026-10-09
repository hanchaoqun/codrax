package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceRenderingCandidatesPredicate = "rendering_candidates_observation"

func traceQueryRenderingCandidatesSchema(schema json.RawMessage) json.RawMessage {
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
	view["description"] = description + " " + tracequery.RenderingCandidatesTeaching
	for _, key := range []string{"pid", "target_scope"} {
		if field, ok := properties[key].(map[string]any); ok {
			description, _ := field["description"].(string)
			field["description"] = description + " rendering_candidates also accepts explicit process scope, using recorded emitter TGID only, never marker payload PID."
		}
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(obj) != nil {
		return schema
	}
	return out.Bytes()
}

func traceQueryRenderingCandidatesObservations(p *tracequery.RenderingCandidatesResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidRenderingCandidates(*p) {
		return nil
	}
	copy := *p
	// Keep complete candidates within a bounded writing lens. Raw result and
	// all population counts remain intact; no name is truncated into a fact.
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
		if len(copy.Candidates) == 0 {
			return nil
		}
		copy.Candidates = copy.Candidates[:len(copy.Candidates)-1]
		copy.OmittedCandidates++
	}
	ref.QueryWindowKnown = true
	ref.QueryWindowStartTs, ref.QueryWindowEndTs = p.Window.StartTs, p.Window.EndTs
	return []types.ObservationRecord{{ID: "trace_query:" + scope + "#rendering_candidates", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingSoft,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		Span:     types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs},
		ClaimKey: TraceRenderingCandidatesPredicate, Predicate: TraceRenderingCandidatesPredicate, Subject: "rendering navigation candidates", Object: p.Status,
		Summary:   "Observed rendering signatures and possible thread roles; advisory navigation, not frame identity or causality",
		RichNotes: []string{types.TraceNoteKeyRenderingCandidates + "=" + string(data)}, ObservedAt: at, Confidence: .5}}
}

func DecodeTraceRenderingCandidates(r types.ObservationRecord) (tracequery.RenderingCandidatesResult, bool) {
	var p tracequery.RenderingCandidatesResult
	ref := r.SourceRef
	if r.Negative || r.Predicate != TraceRenderingCandidatesPredicate || r.ClaimKey != r.Predicate || r.Subject != "rendering navigation candidates" ||
		r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
		r.Role != types.AnswerAggregateRoleSupportingCoverage || r.GroundingPolicy != types.ClaimGroundingSoft ||
		r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact ||
		ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" || !ref.QueryWindowKnown {
		return p, false
	}
	count := 0
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyRenderingCandidates+"=") {
			count++
			raw := json.RawMessage(strings.TrimPrefix(note, types.TraceNoteKeyRenderingCandidates+"="))
			if len(raw) > 64<<10 || !traceRootCauseReportHasUniqueNestedKeys(raw) || json.Unmarshal(raw, &p) != nil {
				return p, false
			}
		}
	}
	return p, count == 1 && tracequery.ValidRenderingCandidates(p) && p.SourcePath == ref.Path && p.Status == r.Object &&
		renderingCandidatesTargetMatchesQuery(p, ref) && p.TargetScope == ref.QueryTargetScope &&
		p.Window.StartTs == ref.QueryWindowStartTs && p.Window.EndTs == ref.QueryWindowEndTs && p.Window.StartTs == r.Span.StartTs && p.Window.EndTs == r.Span.EndTs
}

// The source reference retains the submitted selector. Compare its canonical
// typed grammar with the engine's effective selector, rather than rewriting
// the reference or treating an accepted display spelling as missing evidence.
func renderingCandidatesTargetMatchesQuery(p tracequery.RenderingCandidatesResult, ref types.ObservationSourceRef) bool {
	pid, name := ref.QueryTargetPID, ref.QueryTargetThread
	if selector := threadidentity.Parse(name); selector.Raw != "" {
		if pid <= 0 && selector.HasPID {
			pid = selector.PID
		}
		if selector.Name != "" {
			name = selector.Name
		} else if pid > 0 {
			name = ""
		}
	}
	return p.TargetPID == pid && p.TargetThread == name
}

func TraceRenderingCandidatesText(p tracequery.RenderingCandidatesResult, limit int) string {
	var b strings.Builder
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	fmt.Fprintf(&b, "渲染框架与线程角色线索；查询范围=[%.9f,%.9f%s秒；状态=%s。\n", p.Window.StartTs, p.Window.EndTs, right, p.Status)
	b.WriteString("以下是基于实际标记/线程名的候选，不是框架配置确认、完整帧解析能力或依赖边。多个候选可共存；未发现线索不证明未使用该框架，也不妨碍继续查询帧和卡顿。\n")
	if limit <= 0 {
		limit = 12
	}
	shown := 0
	for _, c := range p.Candidates {
		if shown == limit {
			break
		}
		shown++
		owner, definition := fmt.Sprintf("进程PID=%d", c.OwnerID), "具备线索识别规则，非完整帧解析承诺"
		if c.OwnerScope == "thread" {
			owner = fmt.Sprintf("线程TID=%d（进程归属未知）", c.OwnerID)
		}
		if c.DefinitionStatus == "unsupported_definition" {
			definition = "参考目录缺少管线定义，仅保留观测线索"
		}
		fmt.Fprintf(&b, "- 框架候选=%s；来源=%q；归属=%s；%s。\n", renderingFrameworkLabel(c.Framework), c.SourcePath, owner, definition)
		for _, signal := range c.Signals {
			fmt.Fprintf(&b, "  线索种类=%s；匹配规则=%q；可能角色=%q；匹配行次数=%d（不是线程数或帧数）；示例省略=%d。\n", signal.Kind, signal.Pattern, signal.RoleCandidate, signal.Count, signal.OmittedExamples)
			for _, e := range signal.Examples {
				process := "进程未知"
				if e.TGID > 0 {
					process = fmt.Sprintf("进程PID=%d", e.TGID)
				}
				fmt.Fprintf(&b, "  %.9f秒，线程%q TID=%d / %s，观测名称=%q；来源行=%d（查询行=%d）。\n", e.Ts, e.Thread, e.TID, process, e.Name, e.SourceLine, e.Line)
			}
		}
		fmt.Fprintf(&b, "  线索种类总数=%d，省略=%d。\n", c.TotalSignals, c.OmittedSignals)
	}
	fmt.Fprintf(&b, "候选组合总数=%d，当前展示=%d，省略=%d；次数只用于导航，不表示卡顿严重程度或根因排名。\n", p.TotalCandidates, shown, p.TotalCandidates-shown)
	for _, caveat := range p.Caveats {
		fmt.Fprintf(&b, "- %s\n", caveat)
	}
	return b.String()
}

func renderingFrameworkLabel(id string) string {
	switch id {
	case "HARMONY_ARKUI":
		return "ArkUI"
	case "HARMONY_KMP":
		return "KMP / Compose"
	case "HARMONY_RN":
		return "React Native / 类RN"
	case "HARMONY_FLUTTER":
		return "Flutter"
	case "HARMONY_WEB_PIPELINE":
		return "Web 渲染"
	case "HARMONY_WEBVIEW_GL_FUNCTOR":
		return "WebView GLFunctor"
	case "HARMONY_RENDER_SERVICE":
		return "系统渲染合成服务"
	case "HARMONY_GAME_ENGINE":
		return "游戏引擎"
	}
	return id
}
