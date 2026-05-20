package tool

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocumentRuntimeObservationOnly(ctx *types.BusContext) bool {
	plan := answerSurfacePlan(ctx)
	return plan != nil &&
		plan.RuntimeGroundingDisposition.IsActive() &&
		!plan.CurrentStatusDiagnosticRequired
}

// normalizeRuntimeArtifactCitationRefs removes citation-pool entries that
// would make an attached runtime artifact look like current-repo source proof.
// The visible answer content is preserved; only citation_ref carriers that
// pointed at the artifact-side coordinate are downgraded to -1.
func normalizeRuntimeArtifactCitationRefs(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || !plan.IsCrashSourcedRootCause() {
		return 0
	}
	remove := make(map[int]bool)
	if plan.RuntimeGroundingDisposition.IsActive() && !plan.CurrentStatusDiagnosticRequired {
		for i := range doc.Citations {
			remove[i] = true
		}
	} else {
		for i, cit := range doc.Citations {
			if _, _, ok := citationMatchesDriftedArtifactFrame(plan, cit); ok {
				remove[i] = true
			}
		}
	}
	if len(remove) == 0 {
		return 0
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove)
}

func dropAnswerDocumentCitationsByIndex(doc *types.AnswerDocumentV2, remove map[int]bool) int {
	if doc == nil || len(remove) == 0 {
		return 0
	}
	oldLen := len(doc.Citations)
	remap := make(map[int]int, oldLen)
	next := make([]types.Citation, 0, oldLen-len(remove))
	for i, cit := range doc.Citations {
		if remove[i] {
			continue
		}
		remap[i] = len(next)
		next = append(next, cit)
	}
	changed := oldLen - len(next)
	doc.Citations = next
	for bi := range doc.Blocks {
		for ii := range doc.Blocks[bi].Items {
			ref := doc.Blocks[bi].Items[ii].CitationRef
			if ref < 0 {
				continue
			}
			if remove[ref] {
				doc.Blocks[bi].Items[ii].CitationRef = -1
				changed++
				continue
			}
			if mapped, ok := remap[ref]; ok && mapped != ref {
				doc.Blocks[bi].Items[ii].CitationRef = mapped
				changed++
			}
		}
	}
	return changed
}
