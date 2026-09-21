package tool

// answer_document_mutation_runtime_xerr1.go — XERR1-FIX 件1 互指 display arm
// (§29.104.3/.4, 2026-07-15; E6/E7 账目关系先例).
//
// A converged payload-less blocking_span row (typed basis wait_segments) whose
// Σ(sleep+D+iowait) carries a SLEEP component can cross-reference a sleep
// account of the same thread. This is evidence navigation, not proof that the
// two accounts include the same physical members. A sleep account may omit
// segments inside its location envelope (for example, a capped CPU-bucket
// inventory). Keep both values and references; the actual component relation
// remains unproven and the totals must not be added directly.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjMarkBlockingWaitSleepRelations stamps the 件1 mutual
// pointers. The unchanged typed selector is a bounded navigation rule, not
// physical containment authority:
//
//   - blocking side: BlockingValueBasis==wait_segments ∧ BlockingWaitSleepMS>0
//     ∧ a valid published interval [StartTs,EndTs] ∧ payload-LESS
//     (BlockingKind==""). XERR1-EXT (§29.104.17 裁定⑤): payload-typed rows now
//     carry the basis too, but their converged Σ windows on the fold
//     VALUE-WINNER interval, which is NOT on the wire — the location selector
//     below reads the published [StartTs,EndTs] (the fold survivor's display
//     interval), so on a folded payload row it could select the WRONG interval
//     (宁漏勿假指: payload rows skip the pair entirely);
//   - sleep side: same canonical subject ∧ sleep state family (registry lane,
//     never word-face compare) ∧ wall-clock row ∧ compatible query windows ∧
//     a typed location envelope containing the blocking row's interval.
//     This does not prove sleep-member containment; missing endpoints skip
//     navigation rather than inventing a matching range;
//   - ≥2 containing sleep seats → ambiguous, skip whole (禁猜).
func runtimeTraceProjMarkBlockingWaitSleepRelations(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	for _, blocking := range all {
		bn := blocking.Node
		if !blocking.HasData || strings.TrimSpace(blocking.EvidenceTag) == "" {
			continue
		}
		if strings.TrimSpace(bn.BlockingValueBasis) != tracequery.BlockingValueBasisWaitSegments ||
			bn.BlockingWaitSleepMS <= 0 || strings.TrimSpace(bn.BlockingKind) != "" {
			continue
		}
		if !types.TraceCausalProjectionWindowPresent(bn.StartTs, bn.EndTs) {
			continue
		}
		var match *runtimeTraceProjTreeRow
		ambiguous := false
		for _, other := range all {
			on := other.Node
			if other == blocking || !other.HasData || strings.TrimSpace(other.EvidenceTag) == "" {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(on.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(bn.Subject) {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(on) != string(tracequery.CausalLaneWakeupChain) ||
				!runtimeTraceProjSMR1WallClockRow(on) {
				continue
			}
			if !runtimeTraceProjSMR1WindowsCompatible(on, bn) {
				continue
			}
			if !types.TraceCausalProjectionWindowPresent(on.StartTs, on.EndTs) ||
				on.StartTs > bn.StartTs || on.EndTs < bn.EndTs {
				continue
			}
			if match != nil {
				ambiguous = true
				break
			}
			match = other
		}
		if match == nil || ambiguous {
			continue
		}
		blocking.BlockingWaitSleepRef = strings.TrimSpace(match.EvidenceTag)
		if match.BlockingWaitSleepPeerRef == "" {
			match.BlockingWaitSleepPeerRef = strings.TrimSpace(blocking.EvidenceTag)
		}
	}
}
