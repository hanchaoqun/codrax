package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Advisory navigation is rendered separately from the principal evidence pool.
// It cannot supply root-cause candidates, frame links or measurement receipts.
func renderAnswerDocRenderingCandidates(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	type identity struct{ source, scope, payload string }
	keyOf := func(r types.ObservationRecord) identity {
		return identity{r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef}
	}
	contents, conflicts := map[identity]string{}, map[identity]bool{}
	for _, r := range ledger.Records {
		if p, ok := tool.DecodeTraceRenderingCandidates(r); ok {
			key := keyOf(r)
			body, _ := json.Marshal(p)
			if old, exists := contents[key]; exists && old != string(body) {
				conflicts[key] = true
			}
			contents[key] = string(body)
		}
	}
	var b strings.Builder
	seen := map[identity]bool{}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		p, ok := tool.DecodeTraceRenderingCandidates(r)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil {
			rm := &ctx.AnalysisIR.RequestModel
			profile := rm.RuntimeArtifactScopeProfile
			if profile != nil && profile.HasExplicitTimeWindows() && !resourceStackInsideRequestedWindow(profile, tracequery.ResourceStackWindow{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs, EndInclusive: p.Window.EndInclusive}) {
				continue
			}
			if answerDocHasUserRuntimeTarget(rm) {
				p = renderingCandidatesForRequestedTarget(p, rm)
				if len(p.Candidates) == 0 {
					continue
				}
			}
		}
		key := keyOf(r)
		if seen[key] || conflicts[key] {
			continue
		}
		seen[key] = true
		accepted++
		if shown == 4 {
			continue
		}
		text := fmt.Sprintf("查询记录=%q；来源=%q；查询标识=%q；完整数据=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef) + tool.TraceRenderingCandidatesText(p, 16)
		if b.Len()+len(text) > (64<<10)-1024 {
			continue
		}
		if shown == 0 {
			b.WriteString("### 渲染框架与线程角色候选（仅供后续排查导航）\n\n")
		}
		b.WriteString(text)
		shown++
	}
	if accepted > 0 {
		fmt.Fprintf(&b, "另省略查询=%d；不同来源和窗口不合并，不按候选分数禁止其它查询，不将名称匹配升级为执行、帧连接或根因。\n\n", accepted-shown)
	}
	return b.String()
}

// Only the writing lens is narrowed. Counts and bounded examples belong to a
// whole producer candidate; cutting its examples would relabel unseen rows.
func renderingCandidatesForRequestedTarget(p tracequery.RenderingCandidatesResult, rm *types.RequestModel) tracequery.RenderingCandidatesResult {
	kept := make([]tracequery.RenderingCandidate, 0, len(p.Candidates))
	for _, c := range p.Candidates {
		if renderingCandidateMatchesRequestedTarget(c, p, rm) {
			kept = append(kept, c)
		}
	}
	excluded := len(p.Candidates) - len(kept)
	p.Candidates = kept
	p.OmittedCandidates += excluded
	if excluded > 0 {
		p.Caveats = append(append([]string(nil), p.Caveats...), fmt.Sprintf("按用户目标展示完整候选，另排除归属不匹配或不足以证明的候选=%d；总数仍为原查询范围，不是目标范围的完整数量。", excluded))
	}
	return p
}

func renderingCandidateMatchesRequestedTarget(c tracequery.RenderingCandidate, p tracequery.RenderingCandidatesResult, rm *types.RequestModel) bool {
	for _, target := range rm.RuntimeTargets {
		if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		if target.Kind == types.RuntimeTargetKindProcess {
			// The producer groups by known header TGID, never marker PID. That
			// owner applies to all counted rows, including omitted examples.
			if c.OwnerScope == tracequery.TargetScopeProcess && target.PID > 0 && target.PID <= types.RuntimeTargetMaxPID && c.OwnerID == target.PID {
				return true
			}
			continue
		}
		one := &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{target}}
		matches, complete := true, c.OmittedSignals == 0
		for _, signal := range c.Signals {
			complete = complete && signal.OmittedExamples == 0
			for _, e := range signal.Examples {
				tidKnown, tgidKnown := e.TID > 0, e.TGID > 0
				row := types.TraceEventSearchInventoryRow{EmitterTID: e.TID, EmitterTGID: e.TGID, EmitterTIDKnown: &tidKnown, EmitterTGIDKnown: &tgidKnown, Comm: e.Thread}
				matches = matches && traceEventInventoryRowMatchesTarget(row, one)
			}
		}
		if matches && (complete || renderingQueryProvesRequestedTarget(p, target)) {
			return true
		}
	}
	return false
}

func renderingQueryProvesRequestedTarget(p tracequery.RenderingCandidatesResult, target types.RuntimeTarget) bool {
	if p.TargetScope != tracequery.TargetScopeThread {
		return false
	}
	parsed := threadidentity.Parse(target.Thread)
	if target.PID > 0 || parsed.HasPID {
		return p.TargetPID > 0 && types.ObservationRecordMatchesUserRuntimeTarget(types.ObservationRecord{Subject: fmt.Sprint(p.TargetPID)}, &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{target}})
	}
	// A PID-only query does not prove a name remained unchanged on omitted
	// rows. Only the producer's exact name filter supplies that guarantee.
	name := threadidentity.CleanName(target.Thread)
	return p.TargetPID == 0 && name != "" && strings.EqualFold(name, p.TargetThread)
}
