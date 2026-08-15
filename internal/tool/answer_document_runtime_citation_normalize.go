package tool

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	runtimeCitationRefSentinelRe = regexp.MustCompile("`?citation_ref`?\\s*(?:=|:|：)\\s*`?-1`?")
	runtimeCitationRefMarkedRe   = regexp.MustCompile("`?citation_ref`?\\s*(?:被)?(?:标记|设置|置|设|记)为\\s*`?-1`?")
)

func answerDocumentRuntimeObservationOnly(ctx *types.BusContext) bool {
	plan := answerSurfacePlan(ctx)
	if plan == nil ||
		!plan.RuntimeGroundingDisposition.IsActive() ||
		plan.CurrentStatusDiagnosticRequired ||
		plan.CurrentSourceEvidenceOrigin {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active {
		return true
	}
	return runtimeSourceAuthorityAllowsObservationOnlyRuntimeSurface(authority)
}

func answerDocumentRuntimeArtifactWithoutRequiredCurrentSource(ctx *types.BusContext) bool {
	if answerDocumentRuntimeObservationOnly(ctx) {
		return true
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	// CPD #58 (2026-07-05): typed USER boundary outranks the derived
	// authority projection below. ExcludesCurrentSource is the precise
	// explicit-user-exclusion enum ("不分析代码" + verbatim quotes); when it
	// holds and a runtime artifact is in play, repo-source citations are
	// semantically impossible for this run, so artifact pseudo-citation
	// cleanup is always allowed. This is the same typed signal O-5 wired
	// into the two current-source READ passes (quote normalize + metadata
	// surface terms); the pool-cleanup gate was the missed third face: in
	// the donghu specimen
	// (trace_query_donghu_real_frame_multicausal-20260703-111818) an
	// incidental current-source ledger record set CurrentSourceSatisfied
	// in a typed-exclude run, KeepsCurrentSourceLaneLoadBearing vetoed
	// cleanup, and 4 blob-path pseudo-citations rendered as a source
	// bibliography.
	//
	// LAYERING NOTE: this arm is a DISPLAY-LAYER defense, not the root
	// fix. The CurrentSourceSatisfied pollution root fix LANDED as
	// CSP63-FIX (§29.121, real_trace_campaign_20260705.md): engine
	// blob-session reads no longer mint current-source authority
	// (runtimeSourceAuthorityCurrentSourceRecord blob carve-out via
	// types.IsCodraxBlobSessionPath, internal/types/
	// runtime_source_answer_authority_view.go). This arm STAYS per the
	// original directive: an explicit user boundary must keep outranking
	// derived authority here regardless of how the ledger is repaired —
	// it also still covers non-blob incidental current-source records
	// (the CPD #58 donghu specimen's evidence-lane record family) that
	// the blob carve-out does not address.
	if ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.ExcludesCurrentSource() &&
		types.RuntimeArtifactContextActiveFromBus(ctx) {
		return true
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if runtimeSourceAuthorityAppliesToArtifactCitationCleanup(ctx, authority) {
		return runtimeSourceAuthorityAllowsArtifactCitationCleanup(authority)
	}
	if !ctx.AnalysisIR.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(types.RuntimeArtifactContextActiveFromBus(ctx)) {
		return false
	}
	return !answerDocumentHasCurrentSourceObservationSupport(ctx)
}

func runtimeSourceAuthorityAppliesToArtifactCitationCleanup(ctx *types.BusContext, authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	if authority.HasRuntimeCarrier() || authority.CurrentSourceSatisfied {
		return true
	}
	return false
}

func runtimeSourceAuthorityAllowsArtifactCitationCleanup(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

func runtimeSourceAuthorityAllowsObservationOnlyRuntimeSurface(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return true
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		return false
	}
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

// answerDocumentHasCurrentSourceObservationSupport reads ledger record
// origins DIRECTLY — a raw side-channel that bypasses the guarded
// classifier (runtimeSourceAuthorityCurrentSourceRecord), so the CSP63-FIX
// blob-session carve-out (§29.121) does not filter records at this site.
// Audited 2026-07-17 (CSP63-FIX census): every consumer flow reaches the
// same outcome pre/post-fix — this gate only shapes citation-pool display
// cleanup, never completion authority. Residual-port rule: wiring this
// loop to the guarded classifier is a behavior change and MUST ship with
// its own pin (same family note as the orchestrator evaluation site).
func answerDocumentHasCurrentSourceObservationSupport(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		if record.Origin == types.AnswerEvidenceOriginCurrentSource {
			return true
		}
	}
	return false
}

func answerDocumentHasLoadBearingCurrentSourceLane(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if authority.Active {
		return authority.KeepsCurrentSourceLaneLoadBearing()
	}
	return false
}

func answerDocumentExternalObservationOnly(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if runtimeSourceAuthorityDisablesExternalObservationOnly(authority) {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	contract := types.CompileAnswerIntentContract(rm, &ctx.AnalysisIR.AnswerContract)
	hasExternal := false
	for _, origin := range contract.Origins {
		switch origin {
		case types.AnswerEvidenceOriginCurrentSource:
			// The compiled contract can include current_source as a conservative
			// default for generic/count shapes. Do not let that default alone
			// suppress cleanup when the accepted ledger is purely external; real
			// current-source answers are blocked by the answer surface plan or
			// by current-source ledger records below.
			continue
		case types.AnswerEvidenceOriginUnknown, types.AnswerEvidenceOriginSystemInference:
			continue
		default:
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
				hasExternal = true
			}
		}
	}
	if plan := answerSurfacePlan(ctx); plan != nil {
		if plan.CurrentStatusDiagnosticRequired {
			return false
		}
	}
	// Raw side-channel (CSP63-FIX §29.121 residual port, audited
	// 2026-07-17): this loop reads record.Origin directly instead of the
	// guarded runtimeSourceAuthorityCurrentSourceRecord classifier, so the
	// blob-session carve-out does not apply here. Outcome proven identical
	// pre/post-fix on all flows (display-only sentinel gating); wiring it
	// to the guarded classifier later MUST ship with its own pin.
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		switch record.Origin {
		case types.AnswerEvidenceOriginCurrentSource:
			return false
		case types.AnswerEvidenceOriginUnknown, types.AnswerEvidenceOriginSystemInference:
			continue
		default:
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
				hasExternal = true
			}
		}
	}
	return hasExternal
}

func runtimeSourceAuthorityDisablesExternalObservationOnly(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	return authority.KeepsCurrentSourceLaneLoadBearing()
}

func answerDocumentHasExternalObservationSupport(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	contract := types.CompileAnswerIntentContract(rm, &ctx.AnalysisIR.AnswerContract)
	for _, origin := range contract.Origins {
		if answerEvidenceOriginIsExternalObservationSupport(origin) {
			return true
		}
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		if answerEvidenceOriginIsExternalObservationSupport(record.Origin) {
			return true
		}
	}
	return false
}

func answerEvidenceOriginIsExternalObservationSupport(origin types.AnswerEvidenceOrigin) bool {
	switch origin {
	case types.AnswerEvidenceOriginCurrentSource,
		types.AnswerEvidenceOriginUnknown,
		types.AnswerEvidenceOriginSystemInference:
		return false
	default:
		return types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin)
	}
}

// normalizeRuntimeArtifactCitationRefs removes citation-pool entries that
// would make an attached runtime artifact look like current-repo source proof.
// The visible answer content is preserved; only citation_ref carriers that
// pointed at the artifact-side coordinate are downgraded to -1.
func normalizeRuntimeArtifactCitationRefs(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	return normalizeRuntimeArtifactCitationRefsWithContext(doc, ctx, nil)
}

// normalizeRuntimeArtifactObservationCurrentSourceCitationRefsWithContext keeps
// mixed-origin answers lane-correct at item granularity. A mixed
// runtime-artifact + current-source request legitimately needs repo citations
// for its mechanism explanation, so the document-level citation cleanup must
// retain them. But a block whose typed claims are exclusively runtime
// observations cannot borrow one of those source citations.
//
// The gate is fully typed:
//   - analyzer emitted an active RuntimeArtifactValueProfile;
//   - the block has one or more claim uses and every one is
//     ClaimExternalObservation;
//   - the referenced citation resolves inside the current repository and is
//     not a typed/path-shaped runtime artifact.
//
// Block kind is deliberately not part of the gate: the same measured artifact
// fact may be rendered as a scalar, decision, summary, or list without changing
// its evidence origin. No answer text, user wording, numeric literal, case
// name, or model reasoning participates. The item text and the source citation
// pool entry are kept; only this incompatible item-to-citation edge is
// detached. A sibling mechanism block may continue using the same source
// entry.
func normalizeRuntimeArtifactObservationCurrentSourceCitationRefsWithContext(doc *types.AnswerDocumentV2, ctx *types.BusContext, pctx *preEmitCheckContext) int {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.RuntimeArtifactValueProfile == nil ||
		!ctx.AnalysisIR.RequestModel.RuntimeArtifactValueProfile.Active() {
		return 0
	}
	artifactSpellings := runtimeArtifactCitationPathSet(ctx)
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !answerBlockClaimUsesOnly(*block, types.ClaimExternalObservation) {
			continue
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			kept := make([]int, 0, 1+len(item.CitationRefs))
			detached := false
			for _, ref := range types.AnswerBlockItemCitationRefs(*item) {
				if ref < 0 || ref >= len(doc.Citations) {
					kept = append(kept, ref)
					continue
				}
				cit := doc.Citations[ref]
				if cit.Line <= 0 || cit.Scope == types.ScopeNegative ||
					types.LooksLikeRuntimeArtifactPath(cit.File) ||
					citationFileIsRuntimeArtifact(artifactSpellings, cit.File) {
					kept = append(kept, ref)
					continue
				}
				sourcePath, ok := currentSourceCitationPath(ctx.RepoRoot, cit.File)
				if !ok || !currentSourceCitationFileExists(sourcePath) {
					kept = append(kept, ref)
					continue
				}
				detached = true
				fixed++
			}
			if detached {
				types.SetAnswerBlockItemCitationRefs(item, kept)
				if pctx != nil {
					pctx.recordDetachedCitationItemKind(block.ID, item.ID, item.Label, types.DetachedCitationKindEvidenceOriginMismatch)
				}
			}
		}
	}
	return fixed
}

func currentSourceCitationFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func answerBlockClaimUsesOnly(block types.AnswerBlock, want types.ClaimForm) bool {
	if len(block.ClaimUses) == 0 {
		return false
	}
	for _, claim := range block.ClaimUses {
		if claim.ClaimForm != want {
			return false
		}
	}
	return true
}

// normalizeRuntimeArtifactCitationRefsWithContext is the chain-facing variant:
// when pctx is present, every item whose citation_ref pointed at an
// artifact-spelled pool entry being removed is recorded on the QCE §7.13
// disclosure ferry (runtime_artifact wording lane), so the persist chokepoint
// discloses the removal with wording driven by the item's FINAL presence —
// deleted means disclosed, never silently (CPD #58).
func normalizeRuntimeArtifactCitationRefsWithContext(doc *types.AnswerDocumentV2, ctx *types.BusContext, pctx *preEmitCheckContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	if answerDocumentRuntimeArtifactWithoutRequiredCurrentSource(ctx) {
		// Artifact recognition = path shape ∪ typed attachment spelling
		// (user spelling + reserved blob basenames) — the same two
		// precise lanes the current-source read-skip guards consume, so
		// a trace attached under a non-artifact-shaped name cannot slip
		// past the pool cleanup while being skipped by the readers
		// (CPD #58 lane alignment).
		artifactSpellings := runtimeArtifactCitationPathSet(ctx)
		remove := make(map[int]bool)
		for i, cit := range doc.Citations {
			if types.LooksLikeRuntimeArtifactPath(cit.File) ||
				citationFileIsRuntimeArtifact(artifactSpellings, cit.File) {
				remove[i] = true
			}
		}
		if len(remove) > 0 {
			recordRuntimeArtifactCitationDetachDisclosures(doc, ctx, remove, pctx)
			return dropAnswerDocumentCitationsByIndex(doc, remove, "runtime_artifact_redirected_to_evidence_index")
		}
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || !plan.IsCrashSourcedRootCause() {
		return 0
	}
	remove := make(map[int]bool)
	if plan.RuntimeGroundingDisposition.IsActive() &&
		!plan.CurrentStatusDiagnosticRequired &&
		!plan.CurrentSourceEvidenceOrigin &&
		!answerDocumentHasLoadBearingCurrentSourceLane(ctx) &&
		!answerDocumentHasCurrentSourceObservationSupport(ctx) {
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
	recordRuntimeArtifactCitationDetachDisclosures(doc, ctx, remove, pctx)
	return dropAnswerDocumentCitationsByIndex(doc, remove, "crash_sourced_runtime_grounding_cleanup")
}

// recordRuntimeArtifactCitationDetachDisclosures records persist-time
// disclosure identities for items whose citation_ref points at a pool entry
// being removed AS a runtime-artifact reference. Bounds, stated honestly:
//   - Only ARTIFACT-SPELLED entries ride the runtime_artifact wording lane.
//     The crash-sourced remove-all branch can also drop non-artifact entries;
//     those keep the pass's historical posture (no disclosure) because
//     describing a repo file:line as "attached runtime artifact material"
//     would be false wording.
//   - Pool entries no visible item references are dropped without a
//     disclosure record: the ferry's identity + wording model is item-based
//     ("content kept" vs "item removed"), and an unreferenced entry has no
//     item whose fate could be disclosed. Its only surface was the
//     bibliography list itself, which the removal corrects.
func recordRuntimeArtifactCitationDetachDisclosures(doc *types.AnswerDocumentV2, ctx *types.BusContext, remove map[int]bool, pctx *preEmitCheckContext) {
	if pctx == nil || doc == nil || len(remove) == 0 {
		return
	}
	artifactSpellings := runtimeArtifactCitationPathSet(ctx)
	artifact := make(map[int]bool, len(remove))
	for i := range remove {
		if i < 0 || i >= len(doc.Citations) {
			continue
		}
		file := doc.Citations[i].File
		if types.LooksLikeRuntimeArtifactPath(file) ||
			citationFileIsRuntimeArtifact(artifactSpellings, file) {
			artifact[i] = true
		}
	}
	if len(artifact) == 0 {
		return
	}
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		for ii := range block.Items {
			item := &block.Items[ii]
			for _, ref := range types.AnswerBlockItemCitationRefs(*item) {
				if artifact[ref] {
					pctx.recordDetachedCitationItemKind(block.ID, item.ID, item.Label, types.DetachedCitationKindRuntimeArtifact)
					break
				}
			}
		}
	}
}

// normalizeExternalObservationPseudoCitations removes non-positive line-shaped
// citation carriers when the run has typed external observation support. A
// current-source citation must be a real file:line (or an explicit non-line
// scope such as ScopeFile/ScopeSection); VCS paths, command rows, log lines, and
// trace spans should travel through ObservationSourceRef/ObservationSpan instead
// of masquerading as `file:0` citations. The model-authored answer surface is
// preserved; only invalid citation carriers are detached.
func normalizeExternalObservationPseudoCitations(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || len(doc.Citations) == 0 || !answerDocumentHasExternalObservationSupport(ctx) {
		return 0
	}
	remove := make(map[int]bool)
	for i, cit := range doc.Citations {
		if citationIsPseudoCurrentSourceCarrier(cit) {
			remove[i] = true
		}
	}
	if len(remove) == 0 {
		return 0
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove, "pseudo_current_source_carrier_rejected")
}

func citationIsPseudoCurrentSourceCarrier(cit types.Citation) bool {
	if cit.Line > 0 {
		return false
	}
	switch cit.Scope {
	case "", types.ScopeLine, types.ScopeLineRange:
		return true
	case types.ScopeSection:
		return strings.TrimSpace(cit.SectionPath) == ""
	default:
		return false
	}
}

// dropAnswerDocumentCitationsByIndex removes the selected citation-pool
// entries, remaps surviving citation_refs, and DEBUG-logs every removed
// entry with the caller's typed reason (§29.174 RUN2AUDIT-1 F6: a
// submitted→registered citation delta must be reconstructable from the
// logs entry by entry, not just as an opaque count).
func dropAnswerDocumentCitationsByIndex(doc *types.AnswerDocumentV2, remove map[int]bool, reason string) int {
	if doc == nil || len(remove) == 0 {
		return 0
	}
	if reason == "" {
		reason = "citation_pool_cleanup"
	}
	for i, cit := range doc.Citations {
		if !remove[i] {
			continue
		}
		logging.Debug("[emit_answer_document] citation pool entry dropped: index=%d file=%q line=%d scope=%q reason=%s",
			i, cit.File, cit.Line, string(cit.Scope), reason)
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
			item := &doc.Blocks[bi].Items[ii]
			refs := types.AnswerBlockItemCitationRefs(*item)
			mappedRefs := make([]int, 0, len(refs))
			for _, ref := range refs {
				if remove[ref] {
					changed++
					continue
				}
				if mapped, ok := remap[ref]; ok {
					mappedRefs = append(mappedRefs, mapped)
					if mapped != ref {
						changed++
					}
				}
			}
			types.SetAnswerBlockItemCitationRefs(item, mappedRefs)
		}
	}
	return changed
}

func normalizeUnusedCitationPoolEntries(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || len(doc.Citations) == 0 {
		return 0
	}
	used := make(map[int]bool)
	for _, block := range doc.Blocks {
		for _, item := range block.Items {
			for _, ref := range types.AnswerBlockItemCitationRefs(item) {
				if ref >= 0 && ref < len(doc.Citations) {
					used[ref] = true
				}
			}
		}
	}
	markTypedMarkdownRowCitationsUsed(used, doc, ctx)
	if len(used) == 0 || len(used) == len(doc.Citations) {
		return 0
	}
	remove := make(map[int]bool, len(doc.Citations)-len(used))
	for i := range doc.Citations {
		if !used[i] {
			remove[i] = true
		}
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove, "unused_pool_entry_pruned")
}

// normalizeAnswerDocumentItemCitationSets restores the canonical legacy
// primary slot after earlier evidence-origin or alignment passes may have
// detached/rebound it. Additional refs remain explicit typed carriers; this
// pass only deduplicates indexes and promotes the first surviving ref. It does
// not inspect visible text or create a citation.
func normalizeAnswerDocumentItemCitationSets(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	changed := 0
	for bi := range doc.Blocks {
		for ii := range doc.Blocks[bi].Items {
			item := &doc.Blocks[bi].Items[ii]
			beforePrimary := item.CitationRef
			beforeExtra := append([]int(nil), item.CitationRefs...)
			types.SetAnswerBlockItemCitationRefs(item, types.AnswerBlockItemCitationRefs(*item))
			if beforePrimary != item.CitationRef || !slices.Equal(beforeExtra, item.CitationRefs) {
				changed++
			}
		}
	}
	return changed
}

// normalizeUnusedContradictedRuntimeArtifactNegativeCitations removes only a
// patch/global-pool citation that is both unreachable from the structured
// answer and self-contradictory: it claims a NegativePattern is absent from a
// bound runtime artifact although a bounded scan of that artifact finds the
// pattern. It deliberately does NOT apply the full-emit unused-pool policy to
// patch documents, whose inherited citation indexes are a pinned contract.
//
// Authority comes from structured citation fields, Codrax's reserved runtime
// blob spelling, the stat-resolved bound artifact, and a positive regex match.
// User request text and rendered/model-authored answer prose are never read.
func normalizeUnusedContradictedRuntimeArtifactNegativeCitations(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	used := make(map[int]bool)
	for _, block := range doc.Blocks {
		for _, item := range block.Items {
			for _, ref := range types.AnswerBlockItemCitationRefs(item) {
				if ref >= 0 && ref < len(doc.Citations) {
					used[ref] = true
				}
			}
		}
	}
	markTypedMarkdownRowCitationsUsed(used, doc, ctx)

	remove := make(map[int]bool)
	matchCache := make(map[string]bool)
	checked := make(map[string]bool)
	for i, cit := range doc.Citations {
		if used[i] || cit.Scope != types.ScopeNegative {
			continue
		}
		pattern := strings.TrimSpace(cit.NegativePattern)
		if pattern == "" || types.ReservedRuntimeArtifactBlobKind(cit.File) == "" {
			continue
		}
		path := resolveRuntimeArtifactCitationReadPath(ctx, cit.File)
		if path == "" {
			continue
		}
		cacheKey := path + "\x00" + pattern
		matched := matchCache[cacheKey]
		if !checked[cacheKey] {
			checked[cacheKey] = true
			matched = runtimeArtifactContainsNegativePattern(path, pattern)
			matchCache[cacheKey] = matched
		}
		if matched {
			remove[i] = true
		}
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove, "unused_runtime_negative_proof_contradicted")
}

func runtimeArtifactContainsNegativePattern(path, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 1<<20)
	var scanned int64
	for scanned < artifactQuoteCheckScanByteCap {
		line, readErr := reader.ReadString('\n')
		scanned += int64(len(line))
		if scanned > artifactQuoteCheckScanByteCap {
			return false
		}
		if re.MatchString(line) {
			return true
		}
		if readErr != nil {
			return false
		}
	}
	return false
}

// markTypedMarkdownRowCitationsUsed keeps citations that are carried by an
// accepted enumeration row already rendered inside a model-authored Markdown
// table. Markdown tables intentionally keep Items empty to preserve arbitrary
// multi-column layouts, so item-only reachability would otherwise delete their
// row citations. The row membership and coordinates come from the typed answer
// surface plan; table text only proves that the corresponding accepted row is
// already visible and never creates citation authority.
func markTypedMarkdownRowCitationsUsed(used map[int]bool, doc *types.AnswerDocumentV2, ctx *types.BusContext) {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil || len(doc.Citations) == 0 {
		return
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return
	}
	sets := types.CompileEnumerationCitationSupportSets(&ctx.AnalysisIR.RequestModel, plan)
	if len(sets) == 0 {
		return
	}
	for bi, block := range doc.Blocks {
		if block.Kind != types.BlockTable ||
			len(principalEnumerationMarkdownTableRows(block.Text)) == 0 {
			continue
		}
		rows := principalEnumerationAllRows(sets)
		if scoped, _ := principalEnumerationPruneRowsForBlockAtWithMode(doc, bi, sets); len(scoped) > 0 {
			rows = scoped
		}
		for _, row := range rows {
			if !row.HasCitation ||
				strings.TrimSpace(row.Source) == "" ||
				row.LineStart <= 0 ||
				!principalEnumerationBlockCoversRow(block, doc, row) {
				continue
			}
			surface := types.AnswerSourceLocationSurface{
				File:      row.Source,
				LineStart: row.LineStart,
				LineEnd:   row.LineEnd,
			}
			for ci, citation := range doc.Citations {
				if types.AnswerSourceLocationSurfaceMatchesCitation(surface, citation) {
					used[ci] = true
				}
			}
		}
	}
}

// These compatibility entrypoints intentionally do nothing. Older shipping
// code scanned and rewrote model-visible prose (or removed decision blocks) in
// an attempt to sanitize answer shape. Answer ownership now forbids that: the
// system may adjust typed citation metadata or publish a separately marked
// fact supplement, but it must not edit the model's visible wording.
func normalizeRuntimeArtifactVisibleCitationSentinels(_ *types.AnswerDocumentV2, _ *types.BusContext) int {
	return 0
}

func normalizeRuntimeObservationOnlyDecisionBlocks(_ *types.AnswerDocumentV2, _ *types.AnswerSemanticView, _ *types.BusContext) int {
	return 0
}

func normalizeExternalObservationVisibleCitationSentinels(_ *types.AnswerDocumentV2, _ *types.BusContext) int {
	return 0
}
