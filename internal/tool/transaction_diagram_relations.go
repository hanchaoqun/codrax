package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// An observe edge is a pairing of recorded protocol endpoints, not an
// invocation, wakeup, wait, root-cause or whole-frame relation. Reuse the
// existing provider/recipe/emit/repair/post-validation path for this family.
type transactionDiagramRelationProvider struct{ request *types.RequestModel }

func (p transactionDiagramRelationProvider) Relations(ledger types.ObservationLedger) []RuntimeDiagramRelation {
	type receipt struct{ Source, Query, Payload string }
	keyOf := func(r types.ObservationRecord) receipt {
		return receipt{r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef}
	}
	contents, conflicts := map[receipt]string{}, map[receipt]bool{}
	for _, r := range ledger.Records {
		if f, ok := DecodeTraceTransactionHandoffs(r); ok {
			key := keyOf(r)
			data, _ := json.Marshal(f)
			if old, found := contents[key]; found && old != string(data) {
				conflicts[key] = true
			}
			contents[key] = string(data)
		}
	}
	seen := map[receipt]bool{}
	var out []RuntimeDiagramRelation
	for _, r := range ledger.Records {
		f, ok := DecodeTraceTransactionHandoffs(r)
		key := keyOf(r)
		if !ok || seen[key] || conflicts[key] {
			continue
		}
		seen[key] = true
		if scope := ledger.RuntimeArtifactScopeProfile; scope != nil && scope.HasExplicitTimeWindows() && (!scope.ContainsExplicitTimeWindow(f.Window.StartTs, f.Window.EndTs) || f.Window.EndInclusive) {
			continue
		}
		for _, h := range f.Handoffs {
			if h.Status != "observed_unique_protocol_match" {
				continue
			}
			s, c := h.Submissions[0], h.Consumptions[0]
			if !transactionDiagramMatchesTarget(h, p.request) {
				continue
			}
			identity := func(e tracequery.TransactionEndpoint) string {
				data, _ := json.Marshal(struct {
					Receipt    receipt
					Start, End float64
					Endpoint   tracequery.TransactionEndpoint
				}{key, f.Window.StartTs, f.Window.EndTs, e})
				return fmt.Sprintf("runtime_instance_%x", sha256.Sum256(data))
			}
			from, to := identity(s), identity(c)
			label := func(e tracequery.TransactionEndpoint, role string) string {
				membership := "窗口内"
				if !e.InWindow {
					membership = "窗外关联背景"
				}
				return fmt.Sprintf("%s %s tid=%d %.9fs（%s）", role, e.Thread, e.TID, e.Ts, membership)
			}
			right := ")"
			if f.Window.EndInclusive {
				right = "]"
			}
			out = append(out, RuntimeDiagramRelation{Kind: types.DiagramRelObserve, FromIdentity: from, ToIdentity: to, FromNode: runtimeDiagramNode(from), ToNode: runtimeDiagramNode(to), FromLabel: label(s, "应用提交"), ToLabel: label(c, "服务消费"), ScopeLabel: fmt.Sprintf("仅已发布记录内协议键[tid=%d, seq=%s]对应；不证明整帧/等待/根因；查询=[%.9f,%.9f%s秒", h.TID, h.Sequence, f.Window.StartTs, f.Window.EndTs, right), SupportRefs: []string{fmt.Sprintf("%s:%d", s.SourcePath, s.SourceLine), fmt.Sprintf("%s:%d", c.SourcePath, c.SourceLine)}})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FromIdentity+out[i].ToIdentity < out[j].FromIdentity+out[j].ToIdentity
	})
	return out
}

func transactionDiagramMatchesTarget(h tracequery.TransactionHandoff, rm *types.RequestModel) bool {
	if rm == nil || !runtimeDiagramHasNamedBoundedTarget(rm) {
		return true
	}
	for _, e := range []tracequery.TransactionEndpoint{h.Submissions[0], h.Consumptions[0]} {
		for _, target := range rm.RuntimeTargets {
			if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
				continue
			}
			if target.Kind == types.RuntimeTargetKindProcess {
				if e.TGID > 0 && target.PID == e.TGID {
					return true
				}
				continue
			}
			r := types.ObservationRecord{Subject: fmt.Sprint(e.TID), Object: e.Thread}
			if types.ObservationRecordMatchesUserRuntimeTarget(r, &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{target}}) {
				return true
			}
		}
	}
	return false
}
