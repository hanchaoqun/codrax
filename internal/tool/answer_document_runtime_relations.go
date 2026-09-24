package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RuntimeDiagramRelation is a display-only, directed instance relation. It is
// not EvidenceItem, a call, a wakeup, or a causal/root-cause permission.
type RuntimeDiagramRelation struct {
	Kind                     types.DiagramRelationKind
	FromIdentity, ToIdentity string
	FromNode, ToNode         string
	FromLabel, ToLabel       string
	// ScopeLabel is reader guidance only; it never participates in authority.
	ScopeLabel  string
	SupportRefs []string
}

// RuntimeDiagramRelationProvider keeps producer validation separate from
// diagram rendering and repair. There is deliberately only one provider today.
type RuntimeDiagramRelationProvider interface {
	Relations(types.ObservationLedger) []RuntimeDiagramRelation
}

type businessTreeDiagramRelationProvider struct{}

func (businessTreeDiagramRelationProvider) Relations(ledger types.ObservationLedger) []RuntimeDiagramRelation {
	type instance struct {
		fact   TraceBusinessTreeFact
		record types.ObservationRecord
		bytes  string
	}
	nodes := map[string]instance{}
	ambiguous := map[string]bool{}
	for _, record := range ledger.Records {
		fact, ok := DecodeTraceBusinessTreeFact(record)
		if !ok || record.Negative {
			continue
		}
		if p := ledger.RuntimeArtifactScopeProfile; p != nil && p.HasExplicitTimeWindows() &&
			(fact.WindowUnavailableReason != "" || !p.ContainsExplicitTimeWindow(fact.Window.StartTs, fact.Window.EndTs)) {
			continue
		}
		key := runtimeBusinessInstanceKey(record, fact, fact.Node.ID)
		data, _ := json.Marshal(fact)
		if prior, exists := nodes[key]; exists && prior.bytes != string(data) {
			ambiguous[key] = true
		}
		nodes[key] = instance{fact, record, string(data)}
	}
	var out []RuntimeDiagramRelation
	for key, child := range nodes {
		n := child.fact.Node
		if ambiguous[key] || n.ParentStatus != "observed_parent" || n.ParentID == "" {
			continue
		}
		parentKey := runtimeBusinessInstanceKey(child.record, child.fact, n.ParentID)
		parent, exists := nodes[parentKey]
		if !exists || ambiguous[parentKey] || parentKey == key {
			continue
		}
		p := parent.fact.Node
		// Reject internally inconsistent ancestry even if individual rows decode.
		// Open parents retain observed nesting but never acquire elapsed time.
		if p.DirectChildCount < 1 || p.StartLine >= n.StartLine || p.ActualStartTs > n.ActualStartTs ||
			(p.EndLine > 0 && n.StartLine >= p.EndLine) ||
			(p.EndLine > 0 && n.EndLine > p.EndLine) ||
			(p.ActualEndTs != nil && n.ActualStartTs > *p.ActualEndTs) ||
			(p.ActualEndTs != nil && n.ActualEndTs != nil && *n.ActualEndTs > *p.ActualEndTs) {
			continue
		}
		out = append(out, RuntimeDiagramRelation{Kind: types.DiagramRelContain,
			FromIdentity: parentKey, ToIdentity: key,
			FromNode: runtimeDiagramNode(parentKey), ToNode: runtimeDiagramNode(key),
			FromLabel: p.Name, ToLabel: n.Name,
			ScopeLabel: fmt.Sprintf("线程=%q；物理源=%q；%s；父起点=%s；子起点=%s",
				traceThreadLabel(n.Thread), n.SourcePath, TraceBusinessTreeWindowText(child.fact),
				runtimeBusinessStartLabel(parent.record, parent.fact), runtimeBusinessStartLabel(child.record, child.fact)),
			SupportRefs: append(append([]string(nil), parent.record.SupportRefs...), child.record.SupportRefs...),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FromIdentity+out[i].ToIdentity < out[j].FromIdentity+out[j].ToIdentity
	})
	return out
}

// SupportRefs already use physical source-local coordinates. Never display a
// composite index's virtual line as a line in its physical source file.
func runtimeBusinessStartLabel(record types.ObservationRecord, fact TraceBusinessTreeFact) string {
	start := 0
	for _, ref := range record.SupportRefs {
		if line, _, ok := traceQueryVirtualSupportRefRange(fact.Node.SourcePath, ref); ok && (start == 0 || line < start) {
			start = line
		}
	}
	if start > 0 {
		return fmt.Sprintf("原始文件第%d行", start)
	}
	return fmt.Sprintf("查询索引 %q 第%d行（物理行未解析）", fact.IndexPath, fact.Node.StartLine)
}

func runtimeBusinessInstanceKey(r types.ObservationRecord, f TraceBusinessTreeFact, instanceID string) string {
	// Pin producer query coordinates, not enrichment fields (capture identity,
	// artifact kind) that ledger compilation may append to the source receipt.
	data, _ := json.Marshal(struct {
		Query, Payload, Index     string
		WindowKnown               bool
		Start, End                float64
		LinesKnown                bool
		LineStart, LineEnd        int
		TargetPID                 int
		TargetThread, TargetScope string
		Physical                  string
		TID                       int
		Instance                  string
	}{r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef, f.IndexPath,
		r.SourceRef.QueryWindowKnown, r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs,
		r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineStart, r.SourceRef.QueryLineEnd,
		r.SourceRef.QueryTargetPID, r.SourceRef.QueryTargetThread, r.SourceRef.QueryTargetScope,
		f.Node.SourcePath, f.Node.Thread.PID, instanceID})
	return fmt.Sprintf("runtime_instance_%x", sha256.Sum256(data))
}

func runtimeDiagramNode(identity string) string {
	return "rt_" + strings.TrimPrefix(identity, "runtime_instance_")
}

// RuntimeDiagramRelations shares precisely the same source/window/target pool
// for authoring, pre-emit, repair and post-finalizer validation. Display budgets
// apply after this lossless authority projection, never to ancestry calculation.
func RuntimeDiagramRelations(ledger types.ObservationLedger, rm *types.RequestModel) []RuntimeDiagramRelation {
	if rm != nil && rm.RuntimeArtifactScopeProfile != nil {
		ledger.RuntimeArtifactScopeProfile = rm.RuntimeArtifactScopeProfile
	}
	if rm != nil && rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() &&
		!rm.RuntimeQuestionProfile.RequestsFactFamily(types.RuntimeQuestionFactOtherObservedValue) {
		named := false
		for _, target := range rm.RuntimeTargets {
			if !types.RuntimeTargetIsExplorationCursorSource(target.Source) && ((target.PID > 0 && target.PID <= types.RuntimeTargetMaxPID) || strings.TrimSpace(target.Thread) != "") {
				named = true
				break
			}
		}
		if named {
			filtered := make([]types.ObservationRecord, 0, len(ledger.Records))
			for _, r := range ledger.Records {
				if types.ObservationRecordMatchesUserRuntimeTarget(r, rm) {
					filtered = append(filtered, r)
				}
			}
			ledger.Records = filtered
		}
	}
	var provider RuntimeDiagramRelationProvider = businessTreeDiagramRelationProvider{}
	return provider.Relations(ledger)
}

func runtimeDiagramRelationsForContext(ctx *types.BusContext) []RuntimeDiagramRelation {
	if ctx == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	return RuntimeDiagramRelations(ledger, rm)
}

func runtimeDiagramRelationProved(rows []RuntimeDiagramRelation, from, to string, kind types.DiagramRelationKind) bool {
	for _, row := range rows {
		if row.Kind == kind && row.FromIdentity == from && row.ToIdentity == to {
			return true
		}
	}
	return false
}

func runtimeDiagramEndpointKnown(rows []RuntimeDiagramRelation, identity string) bool {
	for _, row := range rows {
		if row.FromIdentity == identity || row.ToIdentity == identity {
			return true
		}
	}
	return false
}

func runtimeDiagramBodyRelationProved(rows []RuntimeDiagramRelation, anchors []types.DiagramEdgeAnchor, from, to string, kind types.DiagramRelationKind) bool {
	for _, anchor := range anchors {
		if anchor.FromNode == from && anchor.ToNode == to && anchor.RelationKind == kind && anchor.HasEndpointIdentityPair() &&
			runtimeDiagramRelationProved(rows, anchor.FromIdentity, anchor.ToIdentity, kind) {
			return true
		}
	}
	return false
}

func runtimeDiagramAnchorAliasConflict(rows []RuntimeDiagramRelation, anchor types.DiagramEdgeAnchor) bool {
	for _, endpoint := range [][2]string{{anchor.FromNode, anchor.FromIdentity}, {anchor.ToNode, anchor.ToIdentity}} {
		identity := runtimeDiagramEndpointIdentity(rows, endpoint[0], endpoint[0])
		if identity != endpoint[0] && identity != endpoint[1] {
			return true
		}
	}
	return false
}

// Only a provider-issued exact alias can recover an omitted identity. Reader
// names, labels, timestamps, and arbitrary model aliases are never selectors.
func runtimeDiagramEndpointIdentity(rows []RuntimeDiagramRelation, node, identity string) string {
	if identity != node {
		return identity
	}
	for _, row := range rows {
		if row.FromNode == node {
			return row.FromIdentity
		}
		if row.ToNode == node {
			return row.ToIdentity
		}
	}
	return identity
}

func appendRuntimeDiagramRepairCandidates(allowed []types.AnswerDiagramRelationRepairCandidate, rows []RuntimeDiagramRelation, blockIDs []string, limit int) []types.AnswerDiagramRelationRepairCandidate {
	for _, blockID := range blockIDs {
		for _, row := range rows {
			if len(allowed) >= limit {
				return allowed
			}
			allowed = append(allowed, types.AnswerDiagramRelationRepairCandidate{
				BlockID: blockID, RelationKind: row.Kind, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity,
				FromNodeIDs: []string{row.FromNode}, ToNodeIDs: []string{row.ToNode}, Source: strings.Join(row.SupportRefs, "; "),
			})
		}
	}
	return allowed
}

// RenderRuntimeDiagramRelationRecipes is a small optional authoring capsule.
// Business labels remain reader-facing; exact instance IDs live only in anchor
// metadata. A missing parent or an omitted pair never authorizes an invented edge.
func RenderRuntimeDiagramRelationRecipes(ledger types.ObservationLedger, rm *types.RequestModel) string {
	rows := RuntimeDiagramRelations(ledger, rm)
	if len(rows) == 0 {
		return ""
	}
	const limit = 8
	var b strings.Builder
	b.WriteString("### 可用业务包含图锚 / Available business-nesting diagram anchors\n\nOnly these observed direct parent→child pairs authorize `contain`, not calls, wakeups, precedence or root causes. Optional diagram: use business labels; copy exact identities into edge_anchors. Stable node aliases below allow precise local repair; never bind by name alone. Do not sum parent and child elapsed times.\n")
	for _, row := range rows[:min(limit, len(rows))] {
		anchor := types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind}
		data, _ := json.Marshal(anchor)
		fmt.Fprintf(&b, "- %q → %q；%s；edge_anchor=%s\n", row.FromLabel, row.ToLabel, row.ScopeLabel, data)
	}
	fmt.Fprintf(&b, "- 展示 %d 条已证明直接包含关系，另省略 %d 条；只选所问业务，省略不是无子项或完整树。\n\n", min(limit, len(rows)), max(0, len(rows)-limit))
	return b.String()
}
