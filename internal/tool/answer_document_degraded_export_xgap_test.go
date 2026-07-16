package tool

// XGAP-FIX ④/⑤ pins (§29.104.8, witness 20260715-202022.323-89609): the
// degraded/recovery answer lane must (④) carry the deterministic system
// sections that persistMergedAnswerDocument would have assembled, and (⑤)
// runtime-artifact citation quotes gain an independent verify→disclose arm
// that fires on the healthy persist path too.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMaterializeDeterministicAnswerSectionsForDegradedDoc_ObservationBoard(t *testing.T) {
	ctx := externalPerfBus()
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "sum", Kind: types.BlockSummary, Text: "degraded draft prose",
	}}}

	sections := MaterializeDeterministicAnswerSectionsForDegradedDoc(ctx, doc)
	found := false
	for _, s := range sections {
		if s == "observation_board" {
			found = true
		}
	}
	if !found {
		t.Fatalf("degraded export must materialize the runtime observation board, got %v", sections)
	}
	if len(doc.Blocks) < 2 {
		t.Fatalf("deterministic section must land in the document, blocks=%d", len(doc.Blocks))
	}

	// Idempotence: a second run must not duplicate sections (the
	// materializers carry their own guards).
	blocksAfterFirst := len(doc.Blocks)
	MaterializeDeterministicAnswerSectionsForDegradedDoc(ctx, doc)
	if len(doc.Blocks) != blocksAfterFirst {
		t.Fatalf("second export run duplicated sections: %d → %d blocks", blocksAfterFirst, len(doc.Blocks))
	}
}

func TestMaterializeDeterministicAnswerSectionsForDegradedDoc_NilSafe(t *testing.T) {
	if got := MaterializeDeterministicAnswerSectionsForDegradedDoc(nil, &types.AnswerDocumentV2{}); got != nil {
		t.Fatalf("nil ctx must be inert, got %v", got)
	}
	if got := MaterializeDeterministicAnswerSectionsForDegradedDoc(&types.BusContext{}, nil); got != nil {
		t.Fatalf("nil doc must be inert, got %v", got)
	}
}

// xgapArtifactQuoteFixture writes a fake attached trace blob and returns a
// BusContext whose WorkDir resolves it plus the on-disk path.
func xgapArtifactQuoteFixture(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, types.AttachedTraceBlobBasename)
	lines := []string{
		"# tracer: nop",
		"RenderThread-17597 (17267) [002] d..2 13762.791708: sched_switch: prev_comm=RenderThread",
		"keva-1-17437 (17267) [001] d..2 13762.800001: sched_waking: comm=keva-1 pid=17437",
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{Mutable: types.NewMutableState("trace q"), WorkDir: dir}, path
}

func TestVerifyRuntimeArtifactCitationQuotes_MismatchDisclosedMatchKept(t *testing.T) {
	ctx, path := xgapArtifactQuoteFixture(t)
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s", Kind: types.BlockSummary, Text: "prose"}},
		Citations: []types.Citation{
			{
				// Verbatim fragment of line 2 → matched, no disclosure.
				File:  path,
				Line:  2,
				Quote: "13762.791708: sched_switch: prev_comm=RenderThread",
			},
			{
				// Model-authored prose summary impersonating a quote
				// (the witness shape) → mismatch.
				File:  path,
				Line:  3,
				Quote: "keva-1 线程在窗口内被唤醒并参与优先级反转",
			},
		},
	}
	flagged := verifyRuntimeArtifactCitationQuotes(doc, ctx)
	if flagged != 1 {
		t.Fatalf("expected exactly 1 mismatch disclosure, got %d", flagged)
	}
	var caveat string
	for _, c := range doc.Caveats {
		if strings.HasPrefix(strings.TrimSpace(c), artifactQuoteMismatchCaveatPrefixZH) {
			caveat = c
		}
	}
	if caveat == "" {
		t.Fatalf("mismatch must ride the disclosure caveat lane, caveats=%v", doc.Caveats)
	}
	if !strings.Contains(caveat, types.AttachedTraceBlobBasename+":3") {
		t.Fatalf("caveat must name the mismatched file:line, got %q", caveat)
	}
	// 修补轮 件D wording pin (2026-07-16): the disclosure claims only the
	// proven fact — quote ≠ cited line text (paraphrase OR line drift) —
	// and never the over-claim "该摘录为模型转述".
	if !strings.Contains(caveat, "该摘录与所引工件行原文不符（可能为转述或行号错位）；请以工件行原文为准。") {
		t.Fatalf("zh disclosure must state mismatch-with-possible-causes wording, got %q", caveat)
	}
	if strings.Contains(caveat, "该摘录为模型转述") {
		t.Fatalf("zh disclosure must not over-claim paraphrase as proven, got %q", caveat)
	}
	// Detection → disclosure only: the quote itself is never rewritten and
	// the matched citation is untouched.
	if !strings.Contains(doc.Citations[1].Quote, "优先级反转") {
		t.Fatalf("mismatched quote must not be rewritten, got %q", doc.Citations[1].Quote)
	}

	// Idempotence: re-running reconciles the caveat instead of stacking.
	verifyRuntimeArtifactCitationQuotes(doc, ctx)
	count := 0
	for _, c := range doc.Caveats {
		if strings.HasPrefix(strings.TrimSpace(c), artifactQuoteMismatchCaveatPrefixZH) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("disclosure caveat must upsert, got %d copies", count)
	}
}

func TestVerifyRuntimeArtifactCitationQuotes_ENWordingNoOverclaim(t *testing.T) {
	// 修补轮 件D EN lane pin (2026-07-16).
	ctx, path := xgapArtifactQuoteFixture(t)
	ctx.Language = "en"
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s", Kind: types.BlockSummary, Text: "prose"}},
		Citations: []types.Citation{{
			File:  path,
			Line:  3,
			Quote: "a model summary that does not appear on the cited line",
		}},
	}
	if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 1 {
		t.Fatalf("expected 1 disclosure, got %d", got)
	}
	var caveat string
	for _, c := range doc.Caveats {
		if strings.HasPrefix(strings.TrimSpace(c), artifactQuoteMismatchCaveatPrefixEN) {
			caveat = c
		}
	}
	if caveat == "" {
		t.Fatalf("EN disclosure caveat missing, caveats=%v", doc.Caveats)
	}
	if !strings.Contains(caveat, "they may be paraphrased or cite a shifted line number") {
		t.Fatalf("EN disclosure must state the possible causes, got %q", caveat)
	}
	if strings.Contains(caveat, "model paraphrases, not verbatim artifact lines") {
		t.Fatalf("EN disclosure must not over-claim paraphrase as proven, got %q", caveat)
	}
}

func TestVerifyRuntimeArtifactCitationQuotes_ScanCapFailOpen(t *testing.T) {
	// 修补轮 件F pin (2026-07-16): a citation whose line lies BEYOND the
	// 64MiB stream wall gets no positive line witness → no verdict → ZERO
	// disclosure (fail-open), matching the beyond-EOF lane. The cap is
	// injected through artifactQuoteCheckScanByteCap instead of writing a
	// real 64MiB artifact.
	if artifactQuoteCheckScanByteCap != 64<<20 {
		t.Fatalf("default scan cap must stay 64MiB, got %d", artifactQuoteCheckScanByteCap)
	}
	ctx, path := xgapArtifactQuoteFixture(t)
	old := artifactQuoteCheckScanByteCap
	// Fixture line 1 is 14 bytes ("# tracer: nop\n"): the scan crosses the
	// 16-byte cap while reading line 2 and aborts before line 3.
	artifactQuoteCheckScanByteCap = 16
	defer func() { artifactQuoteCheckScanByteCap = old }()

	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s", Kind: types.BlockSummary, Text: "prose"}},
		Citations: []types.Citation{{
			File:  path,
			Line:  3,
			Quote: "mismatching quote that would be disclosed if the line were read",
		}},
	}
	if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 0 {
		t.Fatalf("beyond-cap citation must stay unverified (fail-open), got %d disclosure(s)", got)
	}
	for _, c := range doc.Caveats {
		trimmed := strings.TrimSpace(c)
		if strings.HasPrefix(trimmed, artifactQuoteMismatchCaveatPrefixZH) ||
			strings.HasPrefix(trimmed, artifactQuoteMismatchCaveatPrefixEN) {
			t.Fatalf("beyond-cap citation must produce zero disclosure, got %q", c)
		}
	}

	// Restore the cap: the same citation is now verified and disclosed —
	// proving the cap (not some other guard) suppressed the verdict above.
	artifactQuoteCheckScanByteCap = old
	if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 1 {
		t.Fatalf("with the cap restored the mismatch must be disclosed, got %d", got)
	}
}

func TestVerifyRuntimeArtifactCitationQuotes_NegativeLanes(t *testing.T) {
	ctx, path := xgapArtifactQuoteFixture(t)

	t.Run("non-artifact citation ignored", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Citations: []types.Citation{{
			File:  "internal/tool/blob.go",
			Line:  10,
			Quote: "completely unrelated prose",
		}}}
		if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 0 {
			t.Fatalf("repo-source citations are out of this arm's scope, got %d", got)
		}
	})

	t.Run("line beyond artifact is fail-open", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Citations: []types.Citation{{
			File:  path,
			Line:  999,
			Quote: "anything",
		}}}
		if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 0 {
			t.Fatalf("no positive line witness → no mismatch verdict, got %d", got)
		}
	})

	t.Run("empty quote ignored", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Citations: []types.Citation{{
			File: path,
			Line: 2,
		}}}
		if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 0 {
			t.Fatalf("quoteless artifact citations have nothing to verify, got %d", got)
		}
	})

	t.Run("arbitrary absolute file never read", func(t *testing.T) {
		secret := filepath.Join(t.TempDir(), "trace_notes.trace")
		if err := os.WriteFile(secret, []byte("line1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		doc := &types.AnswerDocumentV2{Citations: []types.Citation{{
			File:  secret,
			Line:  1,
			Quote: "does not match",
		}}}
		// Path-shape says artifact (.trace) but the basename is not a
		// reserved blob basename nor the attached spelling → the resolver
		// refuses (this arm must not become a generic file reader).
		if got := verifyRuntimeArtifactCitationQuotes(doc, ctx); got != 0 {
			t.Fatalf("non-reserved artifact spellings must stay unread, got %d", got)
		}
	})
}

func TestPersistMergedAnswerDocument_DisclosesArtifactQuoteMismatch(t *testing.T) {
	// Healthy-path pin: the arm is wired at the persist chokepoint.
	ctx, path := xgapArtifactQuoteFixture(t)
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "sum", Kind: types.BlockSummary, Text: "healthy path summary",
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "row", CitationRef: 0}},
		}},
		Citations: []types.Citation{{
			File:  path,
			Line:  3,
			Quote: "模型转述的摘要而非工件行原文",
		}},
	}
	res, err := persistMergedAnswerDocument(ctx, "emit_answer_document", types.MutationReplaceAll, "xgap test", doc, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist must accept the document (disclosure, never reject): res=%+v err=%v", res, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	if persisted == nil {
		t.Fatal("persist did not land the document")
	}
	found := false
	for _, c := range persisted.Caveats {
		if strings.HasPrefix(strings.TrimSpace(c), artifactQuoteMismatchCaveatPrefixZH) {
			found = true
		}
	}
	if !found {
		t.Fatalf("healthy persist path must disclose the artifact quote mismatch, caveats=%v", persisted.Caveats)
	}
}
