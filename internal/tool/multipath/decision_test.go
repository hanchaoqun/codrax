package multipath

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// fakeOracle is a hand-rolled SymbolOracle that returns whatever
// symbols the test setup pre-loads under each file path. Lets unit
// tests pin Signal 1 behaviour without touching the real repomap.
type fakeOracle struct {
	byFile map[string][]SymbolDef
}

func (f fakeOracle) SymbolsInFile(file string) []SymbolDef {
	return f.byFile[file]
}

const repoFile = "internal/repl/repl.go"

// TestEvaluate_S1_AllSymbolRegionsReadSkips replays the production
// motivating scenario in miniature: a 4 368-line file with four
// answer-bearing symbol regions totalling ~120 lines. Once the LLM
// has read those four regions (with the default ±15 line context
// padding), the gate must skip — even though the file is at <3 %
// total coverage. Pre-2026-05-01 the % floor demanded 1 311 lines
// of pagination here.
func TestEvaluate_S1_AllSymbolRegionsReadSkips(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	// Read each symbol's def-region with the default ±15 context window.
	closure.SetReadRanges(map[string][]types.LineRange{
		repoFile: {
			{Start: 207, End: 237},   // failureTaxonomyBannerLine ±15 around 222
			{Start: 475, End: 506},   // unsettledBanner ±15 around 490
			{Start: 1212, End: 1283}, // printBanner ±15 around 1227-1268
			{Start: 1288, End: 1319}, // memorySummaryLine ±15 around 1303
		},
	})
	closure.SetFileTotalLines(map[string]int{repoFile: 4368})

	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		repoFile: {
			{Name: "failureTaxonomyBannerLine", Line: 222, EndLine: 222},
			{Name: "unsettledBanner", Line: 490, EndLine: 490},
			{Name: "printBanner", Line: 1227, EndLine: 1268},
			{Name: "memorySummaryLine", Line: 1303, EndLine: 1303},
		},
	}}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}

	dec := EvaluateAnchor(repoFile, closure, nil, oracle,
		[]string{"printBanner", "memorySummaryLine", "failureTaxonomyBannerLine", "unsettledBanner"}, nil, cfg)

	if dec.Signal != SignalSymbolAnchored {
		t.Fatalf("Signal = %v, want SignalSymbolAnchored. Reason: %s", dec.Signal, dec.Reason)
	}
	if dec.Action != ActionSkip {
		t.Fatalf("Action = %v, want ActionSkip", dec.Action)
	}
	// Coverage in absolute lines: only ~165 of 4368 (~3.8 %). The new
	// gate must NOT block — that's the whole point of the rewrite.
}

// TestEvaluate_S1_MissingRegionDemandsOnlyMissing verifies surgical
// demand: if one of the question-related symbols' region is unread,
// the demand covers ONLY the uncovered slivers, NOT the whole file.
// No "% of total lines" calculation appears anywhere.
func TestEvaluate_S1_MissingRegionDemandsOnlyMissing(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	// Only printBanner's region is read — the other three symbols are
	// uncovered.
	closure.SetReadRanges(map[string][]types.LineRange{
		repoFile: {
			{Start: 1212, End: 1283},
		},
	})
	closure.SetFileTotalLines(map[string]int{repoFile: 4368})

	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		repoFile: {
			{Name: "failureTaxonomyBannerLine", Line: 222, EndLine: 222},
			{Name: "unsettledBanner", Line: 490, EndLine: 490},
			{Name: "printBanner", Line: 1227, EndLine: 1268},
			{Name: "memorySummaryLine", Line: 1303, EndLine: 1303},
		},
	}}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}

	dec := EvaluateAnchor(repoFile, closure, nil, oracle,
		[]string{"printBanner", "memorySummaryLine", "failureTaxonomyBannerLine", "unsettledBanner"}, nil, cfg)

	if dec.Signal != SignalSymbolDemand {
		t.Fatalf("Signal = %v, want SignalSymbolDemand", dec.Signal)
	}
	if dec.Action != ActionDemandSurgicalRead {
		t.Fatalf("Action = %v, want ActionDemandSurgicalRead", dec.Action)
	}
	// The demand must be SURGICAL: only the uncovered symbol regions,
	// roughly 3 × 31 = 93 lines, NOT 30 % of 4 368 = 1 311 lines.
	totalDemanded := 0
	for _, r := range dec.Demand {
		totalDemanded += r.End - r.Start + 1
	}
	if totalDemanded > 200 {
		t.Errorf("demand totals %d lines — surgical demand should be ~90 lines (three ±15-padded single-line definitions); 30%% of 4368 was the old floor (1311) and must NOT be surfaced", totalDemanded)
	}
	if totalDemanded == 0 {
		t.Errorf("demand must be non-empty when symbol regions are uncovered; got %+v", dec.Demand)
	}
	// Rationale must NOT mention the old "30 %" / "per-file floor"
	// language — guards against accidental regression of the prompt.
	if strings.Contains(dec.Rationale, "%") || strings.Contains(strings.ToLower(dec.Rationale), "floor") {
		t.Errorf("rationale must not mention percentage / floor (old gate language); got: %s", dec.Rationale)
	}
	if !strings.Contains(dec.Rationale, "SURGICAL") {
		t.Errorf("rationale must mark the demand SURGICAL so the LLM does not paginate; got: %s", dec.Rationale)
	}
	if !strings.Contains(dec.Rationale, "read_file") {
		t.Errorf("rationale must include a copy-pasteable read_file invocation; got: %s", dec.Rationale)
	}
}

// TestEvaluate_S2_GroundedEvidenceClearsGate locks Signal 2: when
// the LLM has emitted ≥ MinGroundedPerAnchor grounded evidence items
// from the file, the gate skips even with no symbol-anchor coverage.
// This rewards "the LLM has demonstrated localised understanding".
func TestEvaluate_S2_GroundedEvidenceClearsGate(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	// No reads recorded — only the evidence-count signal can save us.
	evidence := []types.EvidenceItem{
		{Source: repoFile, AnchorSymbol: "fnA", LineStart: 100, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
		{Source: repoFile, AnchorSymbol: "fnB", LineStart: 200, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSymbolTable},
	}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}

	// No oracle so Signal 1 cannot fire — we want to be sure we land
	// on Signal 2.
	dec := EvaluateAnchor(repoFile, closure, evidence, nil, nil, nil, cfg)

	if dec.Signal != SignalEvidenceVerified {
		t.Fatalf("Signal = %v, want SignalEvidenceVerified. Reason: %s", dec.Signal, dec.Reason)
	}
	if dec.Action != ActionSkip {
		t.Fatalf("Action = %v, want ActionSkip", dec.Action)
	}
}

// TestEvaluate_S2_GroundedFloorRespected confirms that one grounded
// item is NOT enough at the default floor of 2 — so the engine drops
// to Signal 3 (advisory) for unread, low-evidence files.
func TestEvaluate_S2_GroundedFloorRespected(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	evidence := []types.EvidenceItem{
		{Source: repoFile, AnchorSymbol: "fnA", LineStart: 100, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
	}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}
	dec := EvaluateAnchor(repoFile, closure, evidence, nil, nil, nil, cfg)

	if dec.Signal != SignalOpaqueAdvisory {
		t.Fatalf("Signal = %v, want SignalOpaqueAdvisory (one grounded item is below floor 2)", dec.Signal)
	}
	if dec.Action != ActionAdvisoryHint {
		t.Fatalf("Action = %v, want ActionAdvisoryHint", dec.Action)
	}
}

// TestEvaluate_S3_OpaqueFile_AdvisoryNotBlocking pins the third
// signal: when no symbol matches AND no grounded evidence exists, the
// engine returns ActionAdvisoryHint with a non-blocking rationale.
// The rationale must explicitly state ADVISORY ONLY so the LLM knows
// it's not stalled.
func TestEvaluate_S3_OpaqueFile_AdvisoryNotBlocking(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}
	dec := EvaluateAnchor(repoFile, closure, nil, nil,
		[]string{"someEntityNotInFile"}, nil, cfg)

	if dec.Signal != SignalOpaqueAdvisory {
		t.Fatalf("Signal = %v, want SignalOpaqueAdvisory", dec.Signal)
	}
	if dec.Action != ActionAdvisoryHint {
		t.Fatalf("Action = %v, want ActionAdvisoryHint", dec.Action)
	}
	if len(dec.Demand) != 0 {
		t.Errorf("advisory action must not carry a Demand; got %+v", dec.Demand)
	}
	if !strings.Contains(strings.ToUpper(dec.Rationale), "ADVISORY") {
		t.Errorf("rationale must contain ADVISORY so the LLM knows completion is not blocked; got: %s", dec.Rationale)
	}
}

// TestEvaluate_HasFullyReadShortCircuit preserves the historical
// HasFullyRead bypass — a file with merged ranges covering its full
// line count is exempt from every other signal. Demanding any more
// reads would page past EOF.
func TestEvaluate_HasFullyReadShortCircuit(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		repoFile: {{Start: 1, End: 113}},
	})
	closure.SetFileTotalLines(map[string]int{repoFile: 113})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}

	dec := EvaluateAnchor(repoFile, closure, nil, nil, nil, nil, cfg)
	if dec.Signal != SignalFullyRead {
		t.Fatalf("Signal = %v, want SignalFullyRead", dec.Signal)
	}
	if dec.Action != ActionSkip {
		t.Fatalf("Action = %v, want ActionSkip", dec.Action)
	}
}

// TestEvaluate_NoOracleNoEvidence_FallsToAdvisory ensures the engine
// degrades cleanly when both the symbol oracle is nil AND no
// evidence has been emitted yet — the advisory hint is the right
// answer, not a panic and not a hard demand.
func TestEvaluate_NoOracleNoEvidence_FallsToAdvisory(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}
	dec := EvaluateAnchor(repoFile, closure, nil, nil, nil, nil, cfg)
	if dec.Action != ActionAdvisoryHint {
		t.Fatalf("Action = %v, want ActionAdvisoryHint", dec.Action)
	}
}

// TestEvaluate_S2_TierDegradedNotCounted: the engine deliberately
// only counts Tier 1 (line_text) and Tier 2 (symbol_table) grounding.
// Tier 3+ (snippet fuzzy / fqname / package symbol) are recovery
// tiers that do not warrant the same trust.
func TestEvaluate_S2_TierDegradedNotCounted(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	evidence := []types.EvidenceItem{
		{Source: repoFile, AnchorSymbol: "fnA", GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierFQNameSameFile},
		{Source: repoFile, AnchorSymbol: "fnB", GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSnippetFuzzy},
		{Source: repoFile, AnchorSymbol: "fnC", GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierPackageSymbol},
	}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}
	dec := EvaluateAnchor(repoFile, closure, evidence, nil, nil, nil, cfg)
	// Three Tier 3+ grounded items — but NONE counts toward the
	// symbol-anchored threshold. Engine drops to Signal 3.
	if dec.Signal == SignalEvidenceVerified {
		t.Fatalf("Tier 3+ grounding must NOT clear Signal 2; got Signal = %v", dec.Signal)
	}
}

// TestEvaluate_EvidenceFromAnchorSymbolPromotedToWanted verifies
// the symbol-relevance set is augmented by symbols already cited in
// evidence: even if the analyzer didn't list the symbol as a primary
// entity, an evidence item citing it (with Source matching the file)
// promotes it into the question-related set so its def-region is
// also checked. Prevents the gate from "forgetting" symbols the LLM
// has already grounded against.
func TestEvaluate_EvidenceFromAnchorSymbolPromotedToWanted(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		repoFile: {{Start: 85, End: 115}}, // covers fnX def-region ±15 around 100
	})
	closure.SetFileTotalLines(map[string]int{repoFile: 1000})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		repoFile: {
			{Name: "fnX", Line: 100, EndLine: 100},
		},
	}}
	// Note: PrimaryEntities is empty — fnX comes ONLY from evidence.
	evidence := []types.EvidenceItem{
		{Source: repoFile, AnchorSymbol: "fnX", LineStart: 100, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
	}
	cfg := Config{MinGroundedPerAnchor: 99, SymbolContextLines: 15} // disable Signal 2
	dec := EvaluateAnchor(repoFile, closure, evidence, oracle, nil, nil, cfg)

	if dec.Signal != SignalSymbolAnchored {
		t.Fatalf("Signal = %v, want SignalSymbolAnchored (evidence-cited symbol must be checked)", dec.Signal)
	}
}

// TestEvaluate_S1_TypedEvidenceCoversLargeSymbolSkipsSpanDemand
// pins the large-symbol convergence path: once the model has emitted
// citable typed evidence naming the demanded symbol at an
// answer-bearing line, L1 must not require reading the whole enclosing
// function/struct body just to satisfy a span gate.
func TestEvaluate_S1_TypedEvidenceCoversLargeSymbolSkipsSpanDemand(t *testing.T) {
	const file = "internal/orchestrator/orchestrator.go"
	closure := types.NewEvidenceClosure("")
	closure.SetFileTotalLines(map[string]int{file: 7000})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		file: {
			{Name: "dispatchStage", Line: 5649, EndLine: 6063},
		},
	}}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Subject:         "dispatchStage",
			Object:          "ag.Execute",
			Source:          file,
			LineStart:       5978,
			AnchorSymbol:    "dispatchStage",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			Scope:           types.ScopeLine,
		},
	}
	cfg := Config{MinGroundedPerAnchor: 99, SymbolContextLines: 15}

	dec := EvaluateAnchor(file, closure, evidence, oracle, []string{"dispatchStage"}, nil, cfg)

	if dec.Signal != SignalEvidenceVerified {
		t.Fatalf("Signal = %v, want SignalEvidenceVerified. Reason: %s", dec.Signal, dec.Reason)
	}
	if dec.Action != ActionSkip {
		t.Fatalf("Action = %v, want ActionSkip", dec.Action)
	}
	if !strings.Contains(dec.Reason, "citable model-authored typed evidence") {
		t.Fatalf("reason should name typed evidence coverage, got: %s", dec.Reason)
	}
}

// TestEvaluate_S1_TypedEvidencePartialCoverageDemandsOnlyUncoveredSymbols
// guards the non-seesaw behavior: evidence for one large symbol
// should not globally waive every other question-related symbol in
// the file. The demand remains surgical and only names uncovered
// symbols.
func TestEvaluate_S1_TypedEvidencePartialCoverageDemandsOnlyUncoveredSymbols(t *testing.T) {
	const file = "internal/orchestrator/orchestrator.go"
	closure := types.NewEvidenceClosure("")
	closure.SetFileTotalLines(map[string]int{file: 7000})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		file: {
			{Name: "Run", Line: 1343, EndLine: 1999},
			{Name: "dispatchStage", Line: 5649, EndLine: 6063},
		},
	}}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Subject:         "dispatchStage",
			Object:          "ag.Execute",
			Source:          file,
			LineStart:       5978,
			AnchorSymbol:    "dispatchStage",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			Scope:           types.ScopeLine,
		},
	}
	cfg := Config{MinGroundedPerAnchor: 99, SymbolContextLines: 15}

	dec := EvaluateAnchor(file, closure, evidence, oracle, []string{"Run", "dispatchStage"}, nil, cfg)

	if dec.Signal != SignalSymbolDemand {
		t.Fatalf("Signal = %v, want SignalSymbolDemand. Reason: %s", dec.Signal, dec.Reason)
	}
	if dec.Action != ActionDemandSurgicalRead {
		t.Fatalf("Action = %v, want ActionDemandSurgicalRead", dec.Action)
	}
	if len(dec.Symbols) != 1 || dec.Symbols[0] != "Run" {
		t.Fatalf("Symbols = %+v, want only Run as uncovered", dec.Symbols)
	}
	for _, r := range dec.Demand {
		if r.Start >= 5600 || r.End >= 5600 {
			t.Fatalf("demand must not include dispatchStage span once typed evidence covers it; got %+v", dec.Demand)
		}
	}
}

// TestEvaluate_S1_RawOrSystemEvidenceDoesNotBypassSymbolDemand makes
// the hard-gate boundary explicit. A grounded-looking item that was
// not produced by an emit_evidence lane can still promote a symbol
// into the relevance set, but it cannot waive the whole-symbol read.
func TestEvaluate_S1_RawOrSystemEvidenceDoesNotBypassSymbolDemand(t *testing.T) {
	const file = "internal/orchestrator/orchestrator.go"
	closure := types.NewEvidenceClosure("")
	closure.SetFileTotalLines(map[string]int{file: 7000})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		file: {{Name: "dispatchStage", Line: 5649, EndLine: 6063}},
	}}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Subject:         "dispatchStage",
			Source:          file,
			LineStart:       5978,
			AnchorSymbol:    "dispatchStage",
			Producer:        "system.summary",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			Scope:           types.ScopeLine,
		},
	}
	cfg := Config{MinGroundedPerAnchor: 99, SymbolContextLines: 15}

	dec := EvaluateAnchor(file, closure, evidence, oracle, nil, nil, cfg)

	if dec.Signal != SignalSymbolDemand {
		t.Fatalf("Signal = %v, want SignalSymbolDemand; system evidence must not waive symbol-span demand", dec.Signal)
	}
	if dec.Action != ActionDemandSurgicalRead {
		t.Fatalf("Action = %v, want ActionDemandSurgicalRead", dec.Action)
	}
}

// TestEvaluate_S1_QualifiedTypedEvidenceCoversDefinitionTail locks
// the cross-language surface normalizer used by the coverage gate:
// qualified symbols such as namespace/class/member names may cover a
// repomap symbol with the same terminal definition name, without
// relying on user-text keyword matching.
func TestEvaluate_S1_QualifiedTypedEvidenceCoversDefinitionTail(t *testing.T) {
	const file = "src/pipeline/Stage.ets"
	closure := types.NewEvidenceClosure("")
	closure.SetFileTotalLines(map[string]int{file: 1000})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		file: {{Name: "execute", Line: 410, EndLine: 560}},
	}}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "PipelineStage.execute",
			Source:          file,
			LineStart:       448,
			AnchorSymbol:    "PipelineStage.execute",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			Scope:           types.ScopeLine,
		},
	}
	cfg := Config{MinGroundedPerAnchor: 99, SymbolContextLines: 15}

	dec := EvaluateAnchor(file, closure, evidence, oracle, []string{"PipelineStage.execute"}, nil, cfg)

	if dec.Signal != SignalEvidenceVerified {
		t.Fatalf("Signal = %v, want SignalEvidenceVerified. Reason: %s", dec.Signal, dec.Reason)
	}
}

// TestEvaluate_LargeFileNoiseFreeBypass replays the canonical
// production trace numbers: a 4 368-line file with four primary
// symbols totalling ~140 lines of definitions. With pad ±15 each,
// the LLM needs ~5 reads of ~30-line surgical chunks (~165 lines
// total) to clear the gate — vs the old gate's 1 311 lines.
//
// This test exists to make the intended improvement load-bearing in
// CI: any future change that re-introduces a percentage-of-total
// rule will fail this.
func TestEvaluate_LargeFileNoiseFreeBypass(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		repoFile: {
			{Start: 207, End: 237},
			{Start: 475, End: 506},
			{Start: 1212, End: 1283},
			{Start: 1288, End: 1319},
		},
	})
	closure.SetFileTotalLines(map[string]int{repoFile: 4368})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		repoFile: {
			{Name: "failureTaxonomyBannerLine", Line: 222, EndLine: 222},
			{Name: "unsettledBanner", Line: 490, EndLine: 490},
			{Name: "printBanner", Line: 1227, EndLine: 1268},
			{Name: "memorySummaryLine", Line: 1303, EndLine: 1303},
		},
	}}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15}

	dec := EvaluateAnchor(repoFile, closure, nil, oracle,
		[]string{"failureTaxonomyBannerLine", "unsettledBanner", "printBanner", "memorySummaryLine"}, nil, cfg)

	// Total absolute coverage: ~165 lines = 3.8 % of file. Old gate
	// would have demanded 1 311 lines. New gate must SKIP because
	// every question-related symbol region is covered.
	if dec.Action != ActionSkip {
		t.Fatalf("large-file noise-free bypass: Action = %v, want ActionSkip — every symbol region is read", dec.Action)
	}
	if dec.Signal != SignalSymbolAnchored {
		t.Fatalf("large-file noise-free bypass: Signal = %v, want SignalSymbolAnchored", dec.Signal)
	}
}

// TestSignalString locks the lower-snake-case labels used in [CGEC]
// log lines so trace inspection can grep cleanly.
func TestSignalString(t *testing.T) {
	cases := map[Signal]string{
		SignalUnknown:          "unknown",
		SignalFullyRead:        "fully_read",
		SignalSymbolAnchored:   "symbol_anchored",
		SignalEvidenceVerified: "evidence_verified",
		SignalSymbolDemand:     "symbol_demand",
		SignalKeywordAnchored:  "keyword_anchored",
		SignalKeywordDemand:    "keyword_demand",
		SignalSmallFileFull:    "small_file_full",
		SignalOpaqueAdvisory:   "opaque_advisory",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Signal(%d).String() = %q, want %q", s, got, want)
		}
	}
}

// TestEvaluate_L3_KeywordAnchorAllCovered: non-symbolic file (no
// repomap symbols) where every grep-match line ±KeywordContextLines
// is already inside the closure's read ranges → bypass with
// SignalKeywordAnchored. Replays the canonical config-file scenario:
// codrax.yaml with three grep hits at lines 50, 100, 150; LLM has
// read 35-65, 85-115, 135-165 (three 31-line windows around each
// keyword). Old gate would still demand 30% of file lines — this
// test pins that the new L3 instead grants bypass.
func TestEvaluate_L3_KeywordAnchorAllCovered(t *testing.T) {
	const yamlFile = "codrax.yaml"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		yamlFile: {
			{Start: 35, End: 65},
			{Start: 85, End: 115},
			{Start: 135, End: 165},
		},
	})
	closure.SetFileTotalLines(map[string]int{yamlFile: 1000})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	dec := EvaluateAnchor(yamlFile, closure, nil, nil, nil, []int{50, 100, 150}, cfg)

	if dec.Signal != SignalKeywordAnchored {
		t.Fatalf("Signal = %v, want SignalKeywordAnchored. Reason: %s", dec.Signal, dec.Reason)
	}
	if dec.Action != ActionSkip {
		t.Fatalf("Action = %v, want ActionSkip", dec.Action)
	}
}

// TestEvaluate_L3_KeywordAnchorMissingDemandsSlivers: same setup as
// above but the LLM has only read line 35-65 (around the first
// keyword); the other two anchor windows are uncovered. Demand must
// be SURGICAL — only the missing slivers around keywords 100 and
// 150, NOT a full re-read of the file.
func TestEvaluate_L3_KeywordAnchorMissingDemandsSlivers(t *testing.T) {
	const yamlFile = "codrax.yaml"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		yamlFile: {
			{Start: 35, End: 65}, // covers anchor 50 ±15
		},
	})
	closure.SetFileTotalLines(map[string]int{yamlFile: 1000})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	dec := EvaluateAnchor(yamlFile, closure, nil, nil, nil, []int{50, 100, 150}, cfg)

	if dec.Signal != SignalKeywordDemand {
		t.Fatalf("Signal = %v, want SignalKeywordDemand", dec.Signal)
	}
	if dec.Action != ActionDemandSurgicalRead {
		t.Fatalf("Action = %v, want ActionDemandSurgicalRead", dec.Action)
	}
	totalDemanded := 0
	for _, r := range dec.Demand {
		totalDemanded += r.End - r.Start + 1
	}
	// Two missing 31-line windows (around 100 and 150) = ~62 lines
	// total. Anything above ~80 means we're regressing toward the
	// old "% of file" behavior.
	if totalDemanded > 80 {
		t.Errorf("demand totals %d lines — surgical demand should be ~62 lines (2 missing 31-line keyword windows), NOT a percentage of the 1000-line file", totalDemanded)
	}
	if totalDemanded == 0 {
		t.Errorf("demand must be non-empty when anchor windows are uncovered; got %+v", dec.Demand)
	}
	if !strings.Contains(dec.Rationale, "SURGICAL") {
		t.Errorf("rationale must mark the demand SURGICAL; got: %s", dec.Rationale)
	}
	if !strings.Contains(dec.Rationale, "read_file") {
		t.Errorf("rationale must include a copy-pasteable read_file; got: %s", dec.Rationale)
	}
}

// TestEvaluate_L4_SmallFileFullReadDemand: non-symbolic small file
// (e.g. a 120-line README.md) with no symbols, no evidence, no grep
// history. The LLM has read line 1-30 only; the answer might be at
// line 100. Old behaviour would advisory-only and let the LLM ship
// without reading lines 31-120 — REGRESSION. New L4 demands the
// unread slivers in full, bounded by file size.
func TestEvaluate_L4_SmallFileFullReadDemand(t *testing.T) {
	const readme = "README.md"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		readme: {{Start: 1, End: 30}},
	})
	closure.SetFileTotalLines(map[string]int{readme: 120})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	dec := EvaluateAnchor(readme, closure, nil, nil, nil, nil, cfg)

	if dec.Signal != SignalSmallFileFull {
		t.Fatalf("Signal = %v, want SignalSmallFileFull. Reason: %s", dec.Signal, dec.Reason)
	}
	if dec.Action != ActionDemandSurgicalRead {
		t.Fatalf("Action = %v, want ActionDemandSurgicalRead", dec.Action)
	}
	if len(dec.Demand) != 1 || dec.Demand[0].Start != 31 || dec.Demand[0].End != 120 {
		t.Fatalf("expected exactly one demand range [31, 120], got %+v", dec.Demand)
	}
	if !strings.Contains(dec.Rationale, "small-file threshold") {
		t.Errorf("rationale must mention the small-file threshold so the LLM understands the policy; got: %s", dec.Rationale)
	}
}

// TestEvaluate_L4_SmallFileAlreadyFullyRead: a small file that has
// already been fully read must short-circuit at L0 (HasFullyRead
// bypass), not produce an empty-demand L4 decision.
func TestEvaluate_L4_SmallFileAlreadyFullyRead(t *testing.T) {
	const readme = "README.md"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		readme: {{Start: 1, End: 120}},
	})
	closure.SetFileTotalLines(map[string]int{readme: 120})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	dec := EvaluateAnchor(readme, closure, nil, nil, nil, nil, cfg)

	if dec.Signal != SignalFullyRead {
		t.Fatalf("Signal = %v, want SignalFullyRead", dec.Signal)
	}
}

// TestEvaluate_L5_LargeOpaqueFile_Advisory: above SmallFileThreshold,
// no symbols, no evidence, no grep history → opaque advisory (non-
// blocking). This is the only case left where the LLM may legitimately
// proceed without reading more.
func TestEvaluate_L5_LargeOpaqueFile_Advisory(t *testing.T) {
	const bigDoc = "docs/architecture.md"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		bigDoc: {{Start: 1, End: 50}},
	})
	closure.SetFileTotalLines(map[string]int{bigDoc: 2000}) // > 300 threshold
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	dec := EvaluateAnchor(bigDoc, closure, nil, nil, nil, nil, cfg)

	if dec.Signal != SignalOpaqueAdvisory {
		t.Fatalf("Signal = %v, want SignalOpaqueAdvisory", dec.Signal)
	}
	if dec.Action != ActionAdvisoryHint {
		t.Fatalf("Action = %v, want ActionAdvisoryHint (must NOT block completion)", dec.Action)
	}
	if !strings.Contains(strings.ToUpper(dec.Rationale), "ADVISORY") {
		t.Errorf("rationale must contain ADVISORY; got: %s", dec.Rationale)
	}
}

// TestEvaluate_L4_DisabledByZeroThreshold: setting SmallFileThreshold=0
// must disable L4 — small files then fall to L5 advisory.
func TestEvaluate_L4_DisabledByZeroThreshold(t *testing.T) {
	const readme = "README.md"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		readme: {{Start: 1, End: 30}},
	})
	closure.SetFileTotalLines(map[string]int{readme: 120})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 0}

	dec := EvaluateAnchor(readme, closure, nil, nil, nil, nil, cfg)

	if dec.Signal != SignalOpaqueAdvisory {
		t.Fatalf("Signal = %v, want SignalOpaqueAdvisory (L4 disabled)", dec.Signal)
	}
	if dec.Action != ActionAdvisoryHint {
		t.Fatalf("Action = %v, want ActionAdvisoryHint", dec.Action)
	}
}

// TestEvaluate_LayerOrdering_KeywordBeatsSmallFile: when both L3 and
// L4 would fire on the same file, L3 wins (more specific signal).
// Setup: small README with grep anchors. L3 demands the slivers
// around the anchors; L4 would demand the whole unread file. We
// must see L3's narrower demand, not L4's broader one.
func TestEvaluate_LayerOrdering_KeywordBeatsSmallFile(t *testing.T) {
	const readme = "README.md"
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		readme: {{Start: 35, End: 65}},
	})
	closure.SetFileTotalLines(map[string]int{readme: 200})
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	// Single keyword anchor at line 50, already covered ±15. L3
	// should fire as SignalKeywordAnchored / Skip — L4 must NOT
	// override and demand reading the rest of the 200-line file.
	dec := EvaluateAnchor(readme, closure, nil, nil, nil, []int{50}, cfg)

	if dec.Signal != SignalKeywordAnchored {
		t.Fatalf("Signal = %v, want SignalKeywordAnchored (L3 must beat L4)", dec.Signal)
	}
	if dec.Action != ActionSkip {
		t.Fatalf("Action = %v, want ActionSkip", dec.Action)
	}
}

// TestExtractKeywordAnchors_BasicMatchLines verifies the grep
// summary parser: only `path:lineno:content` shape is treated as a
// match, content must contain at least one entity token, and lines
// are deduped + sorted per file.
func TestExtractKeywordAnchors_BasicMatchLines(t *testing.T) {
	history := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "" +
			"[grep: 4 matching files]\n" +
			"codrax.yaml:50:cgec_multi_path_min_grounded_per_anchor: 2\n" +
			"codrax.yaml:100:  multi_path_symbol_context_lines: 15\n" +
			"codrax.yaml:50:cgec_multi_path_min_grounded_per_anchor: 2\n" + // dup
			"codrax.yaml-99-  # context line, not a match\n" + // dash separator → skipped
			"docs/architecture.md:200:multi-path symbol-anchored gate\n" +
			"unrelated.go:10:func main()\n" + // content does not match any entity
			"--\n",
		},
	}
	got := ExtractKeywordAnchors(history, []string{"multi_path", "multi-path"}, "")
	wantYaml := []int{50, 100}
	wantDoc := []int{200}
	if !equalIntSlice(got["codrax.yaml"], wantYaml) {
		t.Errorf("codrax.yaml lines = %v, want %v", got["codrax.yaml"], wantYaml)
	}
	if !equalIntSlice(got["docs/architecture.md"], wantDoc) {
		t.Errorf("docs/architecture.md lines = %v, want %v", got["docs/architecture.md"], wantDoc)
	}
	if _, ok := got["unrelated.go"]; ok {
		t.Errorf("unrelated.go must not appear (no entity match in content); got %+v", got)
	}
}

// TestExtractKeywordAnchors_SkipsFailedAndShortEntities: failed
// grep results contribute nothing; entity tokens shorter than 3
// chars are dropped to avoid noise (e.g. "go" matching every Go
// function signature).
func TestExtractKeywordAnchors_SkipsFailedAndShortEntities(t *testing.T) {
	history := []types.ToolResult{
		{ToolName: "grep", Success: false, Summary: "codrax.yaml:50:something\n"},
	}
	got := ExtractKeywordAnchors(history, []string{"something"}, "")
	if len(got) != 0 {
		t.Errorf("failed grep must produce no anchors; got %+v", got)
	}
	got = ExtractKeywordAnchors([]types.ToolResult{{ToolName: "grep", Success: true, Summary: "a.go:5:go func(){}\n"}}, []string{"go"}, "")
	if len(got) != 0 {
		t.Errorf("entity 'go' is < 3 chars and must be dropped; got %+v", got)
	}
}

// TestParseGrepMatchLine_Shapes locks the parser against the
// canonical match-line shape AND adversarial inputs (Windows paths,
// content with embedded colons, files-only output).
func TestParseGrepMatchLine_Shapes(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantPath string
		wantLine int
		wantOK   bool
	}{
		{name: "canonical", line: "a/b.go:42:hello", wantPath: "a/b.go", wantLine: 42, wantOK: true},
		{name: "content has colon", line: "x.yaml:7:url: https://example", wantPath: "x.yaml", wantLine: 7, wantOK: true},
		{name: "files-only", line: "a/b.go", wantOK: false},
		{name: "context dash", line: "a/b.go-5-content", wantOK: false},
		{name: "no lineno", line: "a/b.go:hello", wantOK: false},
		{name: "lineno zero", line: "a/b.go:0:hello", wantOK: false},
		{name: "windows-style", line: "C:/proj/x.go:42:hello", wantPath: "C:/proj/x.go", wantLine: 42, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, n, _, ok := parseGrepMatchLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
			if n != tc.wantLine {
				t.Errorf("line = %d, want %d", n, tc.wantLine)
			}
		})
	}
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestExtractKeywordAnchors_AbsolutePathRepoRelative locks the
// multi-platform canonicalisation contract: a grep result returning
// absolute POSIX paths must collapse onto the same map key as the
// repo-relative anchor lookup downstream. Pre-fix the lookup missed
// silently when grep returned absolute paths against a Phase1Ranking
// holding repo-relative entries.
func TestExtractKeywordAnchors_AbsolutePathRepoRelative(t *testing.T) {
	const repoRoot = "/home/chatpp/codrax"
	history := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "" +
			"/home/chatpp/codrax/codrax.yaml:50:cgec_multi_path_min_grounded_per_anchor: 2\n" +
			"/home/chatpp/codrax/codrax.yaml:100:multi_path_symbol_context_lines: 15\n",
		},
	}
	got := ExtractKeywordAnchors(history, []string{"multi_path"}, repoRoot)
	want := map[string][]int{"codrax.yaml": {50, 100}}
	if len(got) != 1 || !equalIntSlice(got["codrax.yaml"], want["codrax.yaml"]) {
		t.Fatalf("absolute POSIX path must canonicalise to repo-relative; got %+v want %+v", got, want)
	}
}

// TestExtractKeywordAnchors_LeadingDotSlashStripped: grep results
// with `./relative` prefix must collapse to the same key as plain
// `relative`.
func TestExtractKeywordAnchors_LeadingDotSlashStripped(t *testing.T) {
	history := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "./codrax.yaml:50:something\n"},
	}
	got := ExtractKeywordAnchors(history, []string{"something"}, "")
	if !equalIntSlice(got["codrax.yaml"], []int{50}) {
		t.Fatalf("./prefix must be stripped; got %+v", got)
	}
}

// TestExtractKeywordAnchors_WindowsBackslashSlashed: grep results
// with Windows-style backslash separators must convert to POSIX
// slash form. Defensive — codrax's grep tool typically emits POSIX
// even on Windows, but defense in depth at canonicaliser time
// guarantees the gate is correct on every host OS.
func TestExtractKeywordAnchors_WindowsBackslashSlashed(t *testing.T) {
	history := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "internal\\repl\\repl.go:42:something\n"},
	}
	got := ExtractKeywordAnchors(history, []string{"something"}, "")
	if !equalIntSlice(got["internal/repl/repl.go"], []int{42}) {
		t.Fatalf("windows backslashes must convert to POSIX; got %+v", got)
	}
}

// TestEvaluate_DemandFlowsThroughToCallerVerbatim pins the
// load-bearing contract that runForcedReads + the rendered hint
// rely on: Decision.Demand is the EXACT line ranges
// EvidenceClosure.MissingContextRegions returned, not a derived /
// re-rounded slice. Caller code copies these verbatim into
// PendingRead.LineRanges so the surgical recovery path works.
func TestEvaluate_DemandFlowsThroughToCallerVerbatim(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.SetReadRanges(map[string][]types.LineRange{
		repoFile: {{Start: 1212, End: 1283}},
	})
	closure.SetFileTotalLines(map[string]int{repoFile: 4368})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		repoFile: {
			{Name: "failureTaxonomyBannerLine", Line: 222, EndLine: 222},
			{Name: "printBanner", Line: 1227, EndLine: 1268},
		},
	}}
	cfg := Config{MinGroundedPerAnchor: 2, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300}

	dec := EvaluateAnchor(repoFile, closure, nil, oracle,
		[]string{"failureTaxonomyBannerLine", "printBanner"}, nil, cfg)

	if dec.Action != ActionDemandSurgicalRead {
		t.Fatalf("Action = %v, want ActionDemandSurgicalRead", dec.Action)
	}
	// Expect demand around failureTaxonomyBannerLine (222 ±15 = [207, 237]).
	// printBanner [1212, 1283] is already covered.
	if len(dec.Demand) != 1 {
		t.Fatalf("expected 1 demand range (only failureTaxonomyBannerLine missing); got %+v", dec.Demand)
	}
	got := dec.Demand[0]
	if got.Start != 207 || got.End != 237 {
		t.Errorf("demand = [%d, %d], want [207, 237]", got.Start, got.End)
	}
}

// TestEvaluate_RepoRootCanonicalisesEvidenceSource: when an evidence
// item's Source is absolute (e.g. forced-read result), Cfg.RepoRoot
// must let questionRelatedSymbols match it against the repo-relative
// anchor file. Without RepoRoot threading, a Source like
// "/home/chatpp/codrax/foo.go" would fail to match anchor "foo.go"
// and the symbol-augmentation path would silently lose its credit.
func TestEvaluate_RepoRootCanonicalisesEvidenceSource(t *testing.T) {
	const repoRoot = "/home/chatpp/codrax"
	const file = "internal/repl/repl.go"
	closure := types.NewEvidenceClosure(repoRoot)
	closure.SetReadRanges(map[string][]types.LineRange{
		file: {{Start: 85, End: 115}},
	})
	closure.SetFileTotalLines(map[string]int{file: 1000})
	oracle := fakeOracle{byFile: map[string][]SymbolDef{
		file: {{Name: "fnX", Line: 100, EndLine: 100}},
	}}
	// Evidence carries an absolute Source path, plain anchor lookup
	// would miss without canonicalisation.
	evidence := []types.EvidenceItem{
		{Source: "/home/chatpp/codrax/internal/repl/repl.go", AnchorSymbol: "fnX",
			LineStart: 100, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
	}
	cfg := Config{MinGroundedPerAnchor: 99, SymbolContextLines: 15, KeywordContextLines: 15, SmallFileThreshold: 300, RepoRoot: repoRoot}

	dec := EvaluateAnchor(file, closure, evidence, oracle, nil, nil, cfg)

	if dec.Signal != SignalSymbolAnchored {
		t.Fatalf("Signal = %v, want SignalSymbolAnchored — RepoRoot must let the absolute-Source evidence promote fnX into the wanted set", dec.Signal)
	}
}

// TestActionString locks Action labels.
func TestActionString(t *testing.T) {
	cases := map[Action]string{
		ActionSkip:               "skip",
		ActionDemandSurgicalRead: "demand_surgical_read",
		ActionAdvisoryHint:       "advisory_hint",
	}
	for a, want := range cases {
		if got := a.String(); got != want {
			t.Errorf("Action(%d).String() = %q, want %q", a, got, want)
		}
	}
}

// TestRenderSymbolDemandRationale_OffsetIsZeroBased pins the
// 2026-05-10 fix: read_file's `offset` parameter is 0-based but the
// missing-range LineRange is 1-based. Customer-reported phantom
// forced-read loop on context.go:1322 traced to this renderer
// emitting `offset=1322` for the 1-based demand `1322-1322`; the
// LLM read line 1323 instead and the demand persisted forever.
//
// Lock: emit `offset=line-1` (via types.LineToReadFileOffset) so
// the LLM's read actually fetches the demanded line.
func TestRenderSymbolDemandRationale_OffsetIsZeroBased(t *testing.T) {
	missing := []types.LineRange{{Start: 1322, End: 1322}}
	got := renderSymbolDemandRationale(
		"internal/types/context.go",
		[]string{"BusContext"},
		[]SymbolDef{{Name: "BusContext", Line: 1320, EndLine: 1380}},
		missing,
		2,
	)
	// Missing-range listing remains 1-based (LLM-readable form).
	if !strings.Contains(got, "Missing line ranges to read: 1322-1322") {
		t.Errorf("missing-range listing must be 1-based for human readability: %q", got)
	}
	// Suggested read_file invocation MUST use 0-based offset.
	if !strings.Contains(got, "offset=1321") {
		t.Errorf("suggested offset must be 0-based (1322 → 1321): %q", got)
	}
	if strings.Contains(got, "offset=1322") {
		t.Errorf("buggy 1-based offset must NOT be emitted: %q", got)
	}
	if !strings.Contains(got, "limit=1") {
		t.Errorf("limit must equal range length (1322-1322 = 1 line): %q", got)
	}
}

func TestRenderKeywordDemandRationale_OffsetIsZeroBased(t *testing.T) {
	missing := []types.LineRange{{Start: 50, End: 65}}
	got := renderKeywordDemandRationale(
		"a.go",
		[]int{55},
		missing,
		3,
	)
	if !strings.Contains(got, "offset=49") {
		t.Errorf("0-based offset (50 → 49) missing: %q", got)
	}
	if strings.Contains(got, "offset=50") {
		t.Errorf("buggy 1-based offset must NOT appear: %q", got)
	}
	if !strings.Contains(got, "limit=16") { // 65 - 50 + 1
		t.Errorf("limit math (65 - 50 + 1 = 16) wrong: %q", got)
	}
}

func TestRenderSmallFileFullRationale_OffsetIsZeroBased(t *testing.T) {
	missing := []types.LineRange{{Start: 1, End: 12}}
	got := renderSmallFileFullRationale(
		"docker-compose.yml",
		20, 60,
		missing,
	)
	if !strings.Contains(got, "offset=0") {
		t.Errorf("first line (1) → offset 0 missing: %q", got)
	}
	if strings.Contains(got, "offset=1,") || strings.Contains(got, "offset=1)") {
		t.Errorf("1-based offset 1 must NOT appear (would skip line 1): %q", got)
	}
	if !strings.Contains(got, "limit=12") {
		t.Errorf("limit math (12 - 1 + 1 = 12) wrong: %q", got)
	}
}
