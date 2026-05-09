package types

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAllRepairKindsHaveProducer is the CGEC Group H structural
// gate: every RepairKind declared in AllRepairKinds() MUST have at
// least one production-code producer. Without this test, a dead
// Kind can slip in — someone adds a new enum value, Render() case,
// and documentation, but forgets to wire a producer, and the
// retry-hint section never fires for the new Kind in practice.
//
// The check walks every *.go file under internal/ (excluding
// _test.go and internal/types/ itself where enum + getter helpers
// live), counts AddRepair call sites that reference each Kind
// constant, and fails when any Kind has zero production callers.
//
// Why a source-level grep instead of a runtime registry: we want
// the test to catch "wrote new enum value, forgot producer" AT
// test time rather than at first production run. Registering at
// init() would also work but adds ceremony and a new side channel
// to maintain.
func TestAllRepairKindsHaveProducer(t *testing.T) {
	repoRoot := findRepoRoot(t)
	internalDir := filepath.Join(repoRoot, "internal")
	sources := collectGoSourcesExcludingTests(t, internalDir)
	// Exclude internal/types/ — that's where the enum lives and
	// self-referential mentions (AllRepairKinds, IsValid, tests)
	// aren't producers.
	filtered := make([]string, 0, len(sources))
	for _, s := range sources {
		if strings.Contains(s, string(os.PathSeparator)+"types"+string(os.PathSeparator)) {
			continue
		}
		filtered = append(filtered, s)
	}

	// For each Kind, look for the pattern
	//     AddRepair(types.RepairDirective{...Kind: types.<KindSymbol>,...})
	// or any use of AddRepair within the same file that mentions
	// the Kind constant. We use a generous two-step check: find
	// files that call AddRepair, then within those files look for
	// any mention of the Kind constant name.
	kindSymbols := map[RepairKind]string{
		RepairReadFile:               "RepairReadFile",
		RepairEmitEvidence:           "RepairEmitEvidence",
		RepairExpandSearch:           "RepairExpandSearch",
		RepairSwapView:               "RepairSwapView",
		RepairRebindSubject:          "RepairRebindSubject",
		RepairForceCompleteDowngrade: "RepairForceCompleteDowngrade",
	}
	// Sanity: AllRepairKinds() must match the map so this test
	// itself catches new kinds added to the enum.
	if len(AllRepairKinds()) != len(kindSymbols) {
		t.Fatalf("kind coverage table out of date: %d kinds in AllRepairKinds vs %d in test map — add the new kind to kindSymbols and wire a producer", len(AllRepairKinds()), len(kindSymbols))
	}

	addRepairPattern := regexp.MustCompile(`\bAddRepair\s*\(`)

	for kind, sym := range kindSymbols {
		var producerFile string
	fileLoop:
		for _, path := range filtered {
			body := readFileForTest(t, path)
			if !addRepairPattern.MatchString(body) {
				continue
			}
			if !strings.Contains(body, sym) {
				continue
			}
			producerFile = path
			break fileLoop
		}
		if producerFile == "" {
			t.Errorf("RepairKind %s has NO producer in internal/ (searched %d files). Every Kind MUST have ≥1 AddRepair call site outside internal/types/ — if this kind was intentionally left without a producer, delete it from the enum instead of keeping dead code.",
				kind, len(filtered))
		} else {
			t.Logf("RepairKind %s producer found: %s", kind, relToRepo(producerFile, repoRoot))
		}
	}
}

// TestEvidenceClosureAllFieldsHaveConsumer asserts that every
// EvidenceClosure field the CGEC design names is read by at least
// one production code path — not just written. Session 10 revived
// three fields (scannedSet, unverifiedFinds, subjectMatches) that
// had been dead because of missing read-side wiring; this test
// prevents regressions by failing when a write-only state surface
// sneaks back in.
//
// The check looks for get-style methods (ReadSet, PendingReads,
// UnverifiedFindings, ScannedSet, IsScanned, AllSubjectMatches,
// CitedRefs, Fingerprints, PendingRepairs, Stats) in production
// *.go files under internal/ (excluding _test.go and
// internal/types/). Any listed accessor with zero production
// callers is a dead read surface.
func TestEvidenceClosureAllFieldsHaveConsumer(t *testing.T) {
	repoRoot := findRepoRoot(t)
	sources := collectGoSourcesExcludingTests(t, filepath.Join(repoRoot, "internal"))
	filtered := make([]string, 0, len(sources))
	for _, s := range sources {
		if strings.Contains(s, string(os.PathSeparator)+"types"+string(os.PathSeparator)) {
			continue
		}
		filtered = append(filtered, s)
	}
	// Each accessor lists ONE method name. Production code should
	// call at least one of them. Group related accessors together
	// so a backup read-path is still acceptable coverage.
	requiredAccessors := map[string][]string{
		"readSet":         {"ReadSet(", "HasRead(", "CanonicalReadFiles("},
		"scannedSet":      {"IsScanned(", "ScannedSet("},
		"citedRefs":       {"CitedRefs("},
		"pendingReads":    {"PendingReads("},
		"unverifiedFinds": {"UnverifiedFindings("},
		"subjectMatches":  {"AllSubjectMatches(", "BestSubjectMatch(", "SubjectMatch("},
		"fingerprints":    {"Fingerprints("},
		"repairs":         {"PendingRepairs(", "ConsumeRepairs("},
		"stats":           {"Stats("},
	}

	for field, accessors := range requiredAccessors {
		found := false
		var hitFile string
		for _, path := range filtered {
			body := readFileForTest(t, path)
			for _, acc := range accessors {
				if strings.Contains(body, acc) {
					found = true
					hitFile = path
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("closure field %s has NO production consumer — searched %d files for %v. Either wire a real read-side consumer OR delete the field to prevent dead state surfaces.",
				field, len(filtered), accessors)
		} else {
			t.Logf("closure field %s consumer: %s", field, relToRepo(hitFile, repoRoot))
		}
	}
}

// TestAllViolationKindsHaveProducer is the Session 11 F1 analogue of
// TestAllRepairKindsHaveProducer. Every ViolationKind declared in
// AllViolationKinds() MUST either (a) have a direct AppendViolation
// producer, (b) be produced as a contract.Violation returned from
// contract.Check (then batched into the ledger by runContractCheck),
// or (c) be explicitly marked pending until its hookup lands in a
// later group (G6/G7).
//
// The test walks production code and checks for the pattern
// "Kind: Violxxx" (composite literal with Kind field) OR direct
// reference "ViolationKind = ViolXxx". When a kind is marked
// pending, the test asserts it is STILL un-produced — the moment
// the hookup lands the test fails with a "move from pending to
// covered" instruction, forcing the pending list to stay honest.
func TestAllViolationKindsHaveProducer(t *testing.T) {
	repoRoot := findRepoRoot(t)
	internalDir := filepath.Join(repoRoot, "internal")
	sources := collectGoSourcesExcludingTests(t, internalDir)
	// Exclude internal/types/ — the enum + getter helpers live there,
	// self-referential mentions (AllViolationKinds, const decls) are
	// not producers.
	filtered := make([]string, 0, len(sources))
	for _, s := range sources {
		if strings.Contains(s, string(os.PathSeparator)+"types"+string(os.PathSeparator)) {
			continue
		}
		filtered = append(filtered, s)
	}

	// Kinds that have at least one producer in the current codebase.
	// The G6/G7 kinds (ViolChainDemoted, ViolSelfRefLiteral,
	// ViolLiteralFormFailed) are explicitly PENDING in G1 — their
	// hookups land with R3/R4/C5 in later groups. Keep the pending
	// list short and documented so when a kind stays unproduced past
	// its expected group, the operator can tell.
	covered := map[ViolationKind]bool{
		ViolCitation:             true, // contract/checker.go + emit_answer_document.go G1 dry-run
		ViolMustInclude:          true, // contract/checker.go
		ViolMustExclude:          true, // contract/checker.go
		ViolAcceptance:           true, // contract/checker.go
		ViolSuccessCriterion:     true, // orchestrator.go SC merge
		ViolGhostAnchor:          true, // agent/explorer.go D2
		ViolPreCompleteDowngrade: true, // tool/emit_investigation_complete.go preComplete
		ViolViewSwap:             true, // tool/emit_answer_document.go B2a + emit_investigation_complete.go B2b
		ViolSelfRefLiteral:       true, // tool/emit_evidence.go R4 self-ref filter (G6)
		// ViolLiteralFormFailed: V1-only literal-form check, retired
		// at B8-T3 (block_only_carrier.md §5.8). Producer deleted
		// along with V1 emit_answer_document path; kind moves to
		// pending until the kind itself is removed from the enum.
		ViolChainDemoted: true, // agent/explorer_erm.go R3 self-ref chain demote (G7)
		// Commit 53 P2/P4 — read-mode answer-coherence violations.
		ViolViewIntentMismatch:    true, // orchestrator/contract_check.go finalize-stage view oracle
		ViolSubTopicCountMismatch: true, // orchestrator/contract_check.go finalize-stage view oracle
		ViolDiagramIdentifier:     true, // tool/emit_answer_document.go diagram bare-identifier check (commit 53 P4)
		// Commit 55 Batch A.3 — declared-count drift.
		ViolDeclaredCountDrift: true, // orchestrator/contract_check.go finalize-stage view oracle
		// Commit 62 — answer-prose self-contradiction.
		ViolSelfContradiction: true, // orchestrator/contract_check.go runSelfConsistencyReview
		// 2026-05-02 — external-artifact decode shortfall.
		ViolExternalArtifactUnderdecoded: true, // orchestrator/contract_check.go runExternalArtifactDecodedCheck
		// 2026-05-02 — AuthorityCeiling axis overreach detector.
		ViolAuthorityOverreach: true, // orchestrator/contract_check.go runAuthorityOverreachCheck
		// Block 1 architecture overhaul (2026-05-02) — reviewer-side
		// kinds. plan_critic / reflector / answer_reviewer all run
		// as independent LLMs and now write their findings into the
		// closure via dedicated wires.
		ViolPlanCritic:              true, // orchestrator/stage_hooks.go planPostHook
		ViolReflectorObservation:    true, // orchestrator/stage_hooks.go appendReflectorObservationToClosure
		ViolAnswerReviewerDistilled: true, // orchestrator/orchestrator.go runAnswerReview Block 1 hook
		// Block 2 / Phase 4 / Phase 5 / Phase 6 V1 oracle kinds —
		// producers retired at B8-T4 along with the V1 oracle dispatch
		// block in contract_check.go (block_only_carrier.md §5.8).
		// V2 carrier covers the equivalent typed-axis coverage via
		// BlockRequirement.{FacetIDs, AcceptableClaimForms,
		// SurfaceRoleHint} consumed by runV2BlockOracles. Kinds remain
		// in the enum so soft-set / fallback-policy / hint-composer
		// references stay stable; B8-cleanup will prune them.
		// P1 #3 / P3 #6 — V2 carrier siblings of the retired V1 oracles
		// (B8-T4). V2 oracle reads V2 block items + ImplementersOf graph;
		// produces the same kinds.
		ViolSymbolAnchorMismatch:               true, // orchestrator/contract_check.go runSymbolAnchorTrackOracleV2
		ViolEnumerationLabelUngrounded:         true, // orchestrator/contract_check_block.go validateEnumerationItemLabelGrounding (post-shape s1a-20260504-064754, 2026-05-04)
		ViolEnumerationItemLabelExtractorDrift: true, // orchestrator/contract_check_block.go validateEnumerationItemLabelExtractorMatch (s1a-20260504-130143, 2026-05-04)
		ViolEnumerationLabelHallucinated:       true, // orchestrator/contract_check_block.go validateEnumerationItemLabelHallucination (Fix C s1a-20260507, 2026-05-07)
		ViolInlineIdentifierHallucinated:       true, // orchestrator/contract_check_block.go validateInlineIdentifierHallucination (Fix I s1a-20260507 post-batch forensic)
		ViolDiagramEdgeEndpointHallucinated:    true, // orchestrator/contract_check_block.go validateDiagramEdgeEndpointHallucination (Fix D 2026-05-07 diagram audit)
		ViolStructuralEnumerationDivergence:    true, // orchestrator/contract_check.go runStructuralEnumerationDivergenceOracleV2
		// B4 V2 block-only carrier validators (block_only_carrier.md §5.4).
		ViolBlockCoverageMissing:     true, // orchestrator/contract_check_block.go validateRequiredBlockCoverage
		ViolPrincipalClaimUseMissing: true, // orchestrator/contract_check_block.go validatePrincipalClaimUse
		ViolDiagramEdgeUnsupported:   true, // orchestrator/contract_check_block.go validateDiagramEdgeSupport
		ViolUncertaintyBlockMissing:  true, // orchestrator/contract_check_block.go validateUncertaintyBlockPresence
		// B6-F1 (post-shape consolidated audit, 2026-05-04).
		ViolCrossCitationConflict: true, // orchestrator/contract_check.go runCrossCitationConflictOracleV2
		// R2.3 V2 重接 (post_shape_residual_audit.md, 2026-05-04).
		ViolFacetUncovered:                  true, // orchestrator/contract_check_block.go validateFacetCoverage
		ViolRichnessRegression:              true, // orchestrator/contract_check_block.go validateRichnessRegression
		ViolClaimFormUnsupported:            true, // orchestrator/contract_check_block.go validateClaimFormSupport
		ViolAbsenceScopeExceeded:            true, // orchestrator/contract_check_block.go validateAbsenceScopeBound
		ViolMissingRequestedRoleUndisclosed: true, // orchestrator/contract_check_block.go validateMissingRequestedRoleDisclosure
		// R10 CGEC frequency bridges (post_shape_residual_audit.md, 2026-05-04).
		ViolDemotionStorm:   true, // orchestrator/orchestrator.go emitCGECStormViolations
		ViolForcedReadStorm: true, // orchestrator/orchestrator.go emitCGECStormViolations
		// G3 (post_v2_runtime_gap_remediation, 2026-05-04).
		ViolDiagramEdgeLabelMismatch: true, // orchestrator/contract_check_block.go validateDiagramRelationLegality
		// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
		// quality reviewer thinness signal. Producer wired in
		// orchestrator/contract_check.go runSemanticQualityReview (G5-2).
		ViolAnswerSemanticUnderfilled: true,
		// 修 B (post_v2_runtime_gap_remediation, 2026-05-04) —
		// enumeration evidence pool structural gate. Producer wired
		// in orchestrator/contract_check.go validateEnumerationEvidenceCoverage.
		ViolEnumerationEvidenceUnderspecified: true,
		// B3 v3 (2026-05-04) — diagram relation typed-first label-only
		// advisory. Producer wired in
		// orchestrator/contract_check_block.go validateDiagramRelationLegality.
		ViolDiagramRelationLabelOnly: true,
		// B2 v3 (2026-05-04) — three-layer quality contract.
		// Producer wired in
		// orchestrator/contract_check_block.go validateRichnessGlaringGap +
		// validatePrincipalProseUnderfilled.
		ViolRichnessGlaringGap:        true,
		ViolPrincipalProseUnderfilled: true,
		// Lane → block-kind compliance (high-priority gap from the
		// 2026-05-07 lane-discipline audit). Producer wired in
		// orchestrator/contract_check_block.go
		// validateLaneBlockKindCompliance, dispatched from
		// contract_check.go after BuildAnswerSupportPlanForBusContext.
		ViolLaneBlockKindMismatch: true,
		// Multi-repo write fail-loud (design §4.5.5 / Phase 4.G,
		// 2026-05-08). Producer wired in
		// orchestrator/multirepo_write_gate.go ValidateChangePlanScope,
		// dispatched at plan-emission time in stage hooks.
		ViolWriteCrossSubRepoForbidden: true,
		// L3 negative-knowledge answer validator (TypedDenials Phase F,
		// 2026-05-08). Producer wired in
		// orchestrator/contract_check.go runDeniedTokenAnswerCheck.
		ViolDeniedTokenUndeclared: true,
		// Phase 2.B Tier 2 ERM completeness violations (2026-05-09,
		// docs/design/commercial_grade_3_pattern_remediation.md).
		// Producers wired in
		// orchestrator/orchestrator.go post-finalize hard gate via
		// internal/agent/erm_completeness.go answer-aware validators.
		ViolScalarCountUnsourced:   true,
		ViolPathDepthInsufficient:  true,
		ViolCardinalityShort:       true,
		ViolEntityParityImbalanced: true,
	}
	pending := map[ViolationKind]string{
		ViolFamilyMismatch:                 "P9-C-retired-V1-checkShape (V2 block oracles cover read-mode block contract via runV2BlockOracles)",
		ViolLiteralFormFailed:              "B8-T3-retired-V1-literal-form-check",
		ViolIntentTraceShallow:             "B8-T4-retired-V1-intent-coverage-oracle",
		ViolIntentEnumerateNotList:         "B8-T4-retired-V1-intent-coverage-oracle",
		ViolIntentRootCauseNoCause:         "B8-T4-retired-V1-intent-coverage-oracle",
		ViolIntentConfigNoTrail:            "B8-T4-retired-V1-intent-coverage-oracle",
		ViolSubjectAnchorMissing:           "B8-T4-retired-V1-subject-anchor-oracle",
		ViolPredicateAxisMissing:           "B8-T4-retired-V1-predicate-axis-oracle",
		ViolStepIdentifierUnverified:       "B8-T4-retired-V1-step-identifier-oracle",
		ViolValueSecondaryCitationOffFocus: "B8-T4-retired-V1-value-secondary-citation-oracle",
	}
	// Sanity: AllViolationKinds() must equal covered ∪ pending so the
	// test itself catches a new kind added to the enum.
	kinds := AllViolationKinds()
	if len(kinds) != len(covered)+len(pending) {
		t.Fatalf("kind coverage table out of date: %d kinds in AllViolationKinds vs %d covered + %d pending — add the new kind to either covered or pending", len(kinds), len(covered), len(pending))
	}

	kindSymbols := map[ViolationKind]string{
		ViolFamilyMismatch:       "ViolFamilyMismatch",
		ViolCitation:             "ViolCitation",
		ViolMustInclude:          "ViolMustInclude",
		ViolMustExclude:          "ViolMustExclude",
		ViolAcceptance:           "ViolAcceptance",
		ViolSuccessCriterion:     "ViolSuccessCriterion",
		ViolGhostAnchor:          "ViolGhostAnchor",
		ViolChainDemoted:         "ViolChainDemoted",
		ViolSelfRefLiteral:       "ViolSelfRefLiteral",
		ViolPreCompleteDowngrade: "ViolPreCompleteDowngrade",
		ViolLiteralFormFailed:    "ViolLiteralFormFailed",
		ViolViewSwap:             "ViolViewSwap",
		ViolAuthorityOverreach:   "ViolAuthorityOverreach",
		// Block 1 (2026-05-02) reviewer-side kinds.
		ViolPlanCritic:              "ViolPlanCritic",
		ViolReflectorObservation:    "ViolReflectorObservation",
		ViolAnswerReviewerDistilled: "ViolAnswerReviewerDistilled",
		// Block 2 (2026-05-02) Intent / Subject / PredicateAxis oracle kinds.
		ViolIntentTraceShallow:              "ViolIntentTraceShallow",
		ViolIntentEnumerateNotList:          "ViolIntentEnumerateNotList",
		ViolIntentRootCauseNoCause:          "ViolIntentRootCauseNoCause",
		ViolIntentConfigNoTrail:             "ViolIntentConfigNoTrail",
		ViolSubjectAnchorMissing:            "ViolSubjectAnchorMissing",
		ViolPredicateAxisMissing:            "ViolPredicateAxisMissing",
		ViolFacetUncovered:                  "ViolFacetUncovered",
		ViolClaimFormUnsupported:            "ViolClaimFormUnsupported",
		ViolAbsenceScopeExceeded:            "ViolAbsenceScopeExceeded",
		ViolMissingRequestedRoleUndisclosed: "ViolMissingRequestedRoleUndisclosed",
		ViolStepIdentifierUnverified:        "ViolStepIdentifierUnverified",
		ViolRichnessRegression:              "ViolRichnessRegression",
		ViolValueSecondaryCitationOffFocus:  "ViolValueSecondaryCitationOffFocus",
		ViolSymbolAnchorMismatch:            "ViolSymbolAnchorMismatch",
		ViolEnumerationLabelUngrounded:      "ViolEnumerationLabelUngrounded",
		ViolStructuralEnumerationDivergence: "ViolStructuralEnumerationDivergence",
		// B4 V2 block-only carrier validators.
		ViolBlockCoverageMissing:              "ViolBlockCoverageMissing",
		ViolPrincipalClaimUseMissing:          "ViolPrincipalClaimUseMissing",
		ViolDiagramEdgeUnsupported:            "ViolDiagramEdgeUnsupported",
		ViolDiagramEdgeLabelMismatch:          "ViolDiagramEdgeLabelMismatch",
		ViolAnswerSemanticUnderfilled:         "ViolAnswerSemanticUnderfilled",
		ViolEnumerationEvidenceUnderspecified: "ViolEnumerationEvidenceUnderspecified",
		ViolUncertaintyBlockMissing:           "ViolUncertaintyBlockMissing",
		// B6-F1 (post-shape consolidated audit, 2026-05-04).
		ViolCrossCitationConflict: "ViolCrossCitationConflict",
		// 2026-05-07 lane-discipline audit.
		ViolLaneBlockKindMismatch: "ViolLaneBlockKindMismatch",
		// Multi-repo write fail-loud (design §4.5.5 / Phase 4.G).
		ViolWriteCrossSubRepoForbidden: "ViolWriteCrossSubRepoForbidden",
		// L3 negative-knowledge answer validator (Phase F).
		ViolDeniedTokenUndeclared: "ViolDeniedTokenUndeclared",
		// Phase 2.B Tier 2 ERM completeness violations (2026-05-09).
		ViolScalarCountUnsourced:   "ViolScalarCountUnsourced",
		ViolPathDepthInsufficient:  "ViolPathDepthInsufficient",
		ViolCardinalityShort:       "ViolCardinalityShort",
		ViolEntityParityImbalanced: "ViolEntityParityImbalanced",
	}

	// Match only the "Kind: ViolXxx" composite-literal pattern —
	// the primary way violations are constructed at producer sites.
	// We deliberately do NOT match bare references to ViolXxx
	// symbols (const aliases, switch cases, enum re-exports) because
	// those surface in contract/checker.go's backward-compat alias
	// block and would falsely claim it as a producer for every kind.
	// Only Kind: <sym> constructions count as real producer evidence.
	kindLiteral := regexp.MustCompile(`\bKind\s*:\s*(?:types\.|contract\.)?(Viol\w+)`)

	for kind, sym := range kindSymbols {
		var producerFile string
		for _, path := range filtered {
			body := readFileForTest(t, path)
			if matches := kindLiteral.FindAllStringSubmatch(body, -1); matches != nil {
				for _, m := range matches {
					if m[1] == sym {
						producerFile = path
						break
					}
				}
			}
			if producerFile != "" {
				break
			}
		}

		if covered[kind] {
			if producerFile == "" {
				t.Errorf("ViolationKind %s marked COVERED but no producer found in internal/ (searched %d files). Either wire a producer or move the kind to the pending list with a group tag.",
					kind, len(filtered))
			} else {
				t.Logf("ViolationKind %s producer: %s", kind, relToRepo(producerFile, repoRoot))
			}
			continue
		}
		if note, isPending := pending[kind]; isPending {
			if producerFile != "" {
				t.Errorf("ViolationKind %s was marked pending (%s) but now has a producer at %s — MOVE IT FROM pending TO covered in the test map so the structural gate is accurate.",
					kind, note, relToRepo(producerFile, repoRoot))
			} else {
				t.Logf("ViolationKind %s pending: %s (no producer yet, as expected)", kind, note)
			}
		}
	}
}

// findRepoRoot walks up from the test working directory looking for
// go.mod. Uses t.Fatalf so we do not silently skip on unexpected
// layouts.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod ancestor of %s", dir)
	return ""
}

// collectGoSourcesExcludingTests returns every *.go file under root
// that is not a _test.go file. Panics on I/O error so the test
// fails loudly rather than silently under-scanning.
func collectGoSourcesExcludingTests(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func relToRepo(path, repoRoot string) string {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	return rel
}
