package tool

// CSP63-FIX 件B pins (§29.121): engine blob OFFLOAD citations must not enter
// the source bibliography.
//
// Adversarial live witness (run -052241, md:966): a trace_query offload
// reference (`.codrax/blob/<session>/trace_query-*.txt`, ScopeFile) passed
// through the citation cleanup face and rendered into the bibliography. The
// offload spellings (trace_query-*.txt / trace-query-result-*.json — engine
// basenames verbatim from the donghu 0703 store_result log line) are neither
// artifact-shaped (types.LooksLikeRuntimeArtifactPath) nor reserved
// attachment basenames (runtimeArtifactCitationPathSet), so BOTH legacy
// recognition lanes missed them. citationFileIsRuntimeArtifact now carries
// the typed blob-session structural lane (types.IsCodraxBlobSessionPath) as
// the single shared recognition authority.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	csp63OffloadRawTxtCitation  = ".codrax/blob/20260717-052241-000-12345/trace_query-89ea5195.txt"
	csp63OffloadPayloadCitation = "/Users/han/opt/claude/codrax/.codrax/blob/20260717-052241-000-12345/trace-query-result-a3f8207c.json"
)

// —— Pre-fix escape proof: both legacy artifact recognition lanes miss the
// offload spellings (this is exactly how the -052241 bibliography leak
// happened), and the new typed blob-session lane catches them through the
// shared recognition helper.
func TestCSP63_BlobOffloadEscapedLegacyArtifactLanes(t *testing.T) {
	ctx := cpdTypedExcludeTraceBusContext()
	spellings := runtimeArtifactCitationPathSet(ctx)
	for _, offload := range []string{csp63OffloadRawTxtCitation, csp63OffloadPayloadCitation} {
		if types.LooksLikeRuntimeArtifactPath(offload) {
			t.Fatalf("path-shape lane unexpectedly recognizes %q (escape premise broken)", offload)
		}
		clean := strings.ToLower(filepath.ToSlash(filepath.Clean(offload)))
		if spellings[clean] || spellings[filepath.Base(clean)] {
			t.Fatalf("spelling-set lane unexpectedly recognizes %q (escape premise broken)", offload)
		}
		if !citationFileIsRuntimeArtifact(spellings, offload) {
			t.Fatalf("typed blob-session lane must recognize offload citation %q", offload)
		}
		// The lane also holds with an EMPTY spelling set: recognition is
		// structural, not dependent on the run having a spelled attachment.
		if !citationFileIsRuntimeArtifact(nil, offload) {
			t.Fatalf("blob-session recognition must not depend on the spelling set: %q", offload)
		}
	}
	// Negative arms: repository paths and legit attachment spellings keep
	// their exact legacy treatment.
	if citationFileIsRuntimeArtifact(spellings, "internal/llm/openai.go") {
		t.Fatal("repository source path must never classify as runtime artifact")
	}
	if !citationFileIsRuntimeArtifact(spellings, cpdBlobCitationPath) {
		t.Fatal("reserved attachment blob citation must stay recognized (zero change)")
	}
}

// —— End-to-end -052241 shape replay: the offload citations (ScopeFile raw
// ref + line-bearing payload ref) are dropped from the pool alongside the
// attached-blob citation, every detached item is disclosed on the
// runtime_artifact wording lane, and the repository citation is untouched.
func TestCSP63_BlobOffloadCitationsDoNotEnterBibliography(t *testing.T) {
	ctx := cpdTypedExcludeTraceBusContext()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			// Witness shape: offload raw ref cited file-scoped (md:966 form).
			{File: csp63OffloadRawTxtCitation, Scope: types.ScopeFile},
			{File: csp63OffloadPayloadCitation, Line: 12, Quote: "root_cause_rank row"},
			// Reserved attachment blob spelling — legacy lane, zero change.
			{File: cpdBlobCitationPath, Line: 2917},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "hop1", Label: "offload-raw", Text: "trace_query 原始行摘录。", CitationRef: 0},
				{ID: "hop2", Label: "offload-payload", Text: "结果载荷行。", CitationRef: 1},
				{ID: "hop3", Label: "attached", Text: "附件行。", CitationRef: 2},
			},
		}},
	}
	pctx := newPreEmitCheckContext(ctx)
	fixed := normalizeRuntimeArtifactCitationRefsWithContext(doc, ctx, pctx)
	if fixed == 0 {
		t.Fatal("artifact pool cleanup must fire on the offload citations")
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("engine blob citations must not remain in the bibliography pool: %+v", doc.Citations)
	}
	for _, item := range doc.Blocks[0].Items {
		if item.CitationRef != -1 {
			t.Fatalf("item %s citation_ref = %d, want -1", item.ID, item.CitationRef)
		}
		if item.Text == "" {
			t.Fatalf("item %s visible content must be preserved", item.ID)
		}
	}
	recs := pctx.detachedCitationDisclosures()
	if len(recs) != 3 {
		t.Fatalf("disclosure records = %d, want 3 (deleted means disclosed, never silently)", len(recs))
	}
	for _, rec := range recs {
		if rec.Kind != types.DetachedCitationKindRuntimeArtifact {
			t.Fatalf("offload detach must ride the runtime_artifact wording lane, got %q", rec.Kind)
		}
	}
}

// —— Mixed-run ruling unchanged (CPD #58 pin 2 rider): without the typed
// exclude boundary the pool is untouched — the new blob-session recognition
// lane must not detach anything in a mixed runtime+current-source run, even
// when an offload citation is present.
func TestCSP63_BlobOffloadRecognitionDoesNotDetachInMixedRun(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "timeout"}}})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentExplain,
			Scenario:  types.ScenarioArchitectureExplain,
			LogTriage: mut.LogTriage(),
			AnalyzerHints: types.AnalyzerHints{
				RequiredFileHints: []types.RequiredFileHint{{
					Path:       "internal/llm/openai.go",
					Confidence: 0.9,
					Rationale:  "current-source mechanism requested by the user",
				}},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/llm/openai.go", Line: 608},
			{File: csp63OffloadRawTxtCitation, Scope: types.ScopeFile},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Items: []types.AnswerBlockItem{
				{ID: "current", Label: "watchdog", CitationRef: 0},
				{ID: "observed", Label: "运行时观测", CitationRef: 1},
			},
		}},
	}
	pctx := newPreEmitCheckContext(ctx)
	if fixed := normalizeRuntimeArtifactCitationRefsWithContext(doc, ctx, pctx); fixed != 0 {
		t.Fatalf("mixed run must not detach pool entries, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 2 || doc.Blocks[0].Items[0].CitationRef != 0 || doc.Blocks[0].Items[1].CitationRef != 1 {
		t.Fatalf("mixed-run pool/refs must stay untouched: %+v", doc)
	}
}
