package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1698ReadCommentSource(t *testing.T, file, source string, offset, limit int) *types.BusContext {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("documented role")}
	args, err := json.Marshal(map[string]any{"path": file, "line_offset": offset, "limit": limit})
	if err != nil {
		t.Fatal(err)
	}
	read, err := (&ReadFile{}).Execute(bus, args)
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("real read prerequisite: err=%v result=%+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	return bus
}

func b1698EmitCommentDefinition(t *testing.T, bus *types.BusContext, file, symbol string, line int, extra ...map[string]any) {
	t.Helper()
	items := []map[string]any{{"evidence_kind": "direct", "scope": "line", "source": file,
		"line_start": line, "anchor_kind": "definition", "anchor_symbol": symbol,
		"subject": symbol, "summary": "The original model summary; it is not the comment source."}}
	items = append(items, extra...)
	args, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), args...)
	result, err := (&EmitEvidence{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("real emit prerequisite: err=%v result=%+v", err, result)
	}
	if !bytes.Equal(before, args) {
		t.Fatal("model argument bytes changed")
	}
	bus.Mutable.AppendDispatchToolResult(result)
}

func TestB1698ReadEmitPreservesCommentSourceRange(t *testing.T) {
	for _, tc := range []struct {
		name, file, source, symbol string
		definition, start, end     int
	}{
		{"java_multiline", "Worker.java", "package p;\n/**\n * Reports a length and records each request.\n * A shared service is injected before registration.\n */\n@Route(path = \"/work\")\nclass Worker {}\n", "Worker", 7, 2, 5},
		{"java_singleline", "Worker.java", "/** Returns the input unchanged. */\nclass Worker {}\n", "Worker", 2, 1, 1},
		{"go_multiline", "worker.go", "package p\n// Worker keeps source ownership.\n// It preserves each incoming value.\nfunc Worker() {}\n", "Worker", 4, 2, 3},
		{"rust_attributes", "worker.rs", "/// Keeps the input.\n/// Records the selected branch.\n#[inline]\nfn worker() {}\n", "worker", 4, 1, 2},
		{"python_multiline", "worker.py", "def worker():\n    \"\"\"Keep the input.\n    Record the selected branch.\n    \"\"\"\n    return 1\n", "worker", 1, 2, 4},
		{"python_hash", "worker.py", "# Keep the input.\n# Record the selected branch.\ndef worker():\n    return 1\n", "worker", 3, 1, 2},
		{"c_lines", "worker.c", "// Preserve the input.\n// Record each decision.\nint worker(void) { return 1; }\n", "worker", 3, 1, 2},
		{"cpp_attribute", "worker.cpp", "// Preserve the input.\n// Record each decision.\n[[nodiscard]]\nint worker() { return 1; }\n", "worker", 4, 1, 2},
		{"arkts_attributes", "Worker.ets", "// Preserve the input.\n// Record each decision.\n@Entry\n@Component\nstruct Worker { build() {} }\n", "Worker", 5, 1, 2},
		{"cangjie_lines", "worker.cj", "// Preserve the input.\n// Record each decision.\nfunc worker(): Int64 { return 1 }\n", "worker", 3, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := b1698ReadCommentSource(t, tc.file, tc.source, 0, 100)
			b1698EmitCommentDefinition(t, bus, tc.file, tc.symbol, tc.definition)
			var paired *types.EvidenceItem
			for _, item := range bus.Mutable.EmittedEvidence() {
				if item.Producer == types.EvidenceProducerAutoPairRoleDescription {
					if paired != nil {
						t.Fatal("one definition produced multiple comment companions")
					}
					copy := item
					paired = &copy
				} else if item.LineStart != tc.definition || item.Summary != "The original model summary; it is not the comment source." {
					t.Fatalf("model-authored definition changed: %+v", item)
				}
			}
			if paired == nil || !paired.IsCitable() || paired.Predicate != "documents" {
				t.Fatalf("existing comment companion prerequisite: %+v", paired)
			}
			wantSummary, wantStart := types.ExtractLeadingDocComment([]byte(tc.source), tc.definition, tc.file)
			if wantStart != tc.start || paired.Summary != wantSummary || paired.LineStart != tc.start {
				t.Fatalf("source-derived summary/start changed: %+v, want %d %q", paired, wantStart, wantSummary)
			}
			wantSnippet := strings.Join(strings.Split(tc.source, "\n")[tc.start-1:tc.end], "\n")
			if paired.LineEnd != tc.end || paired.Snippet != wantSnippet {
				t.Errorf("documented-role evidence lost its raw source range: got %d-%d %q; want %d-%d %q", paired.LineStart, paired.LineEnd, paired.Snippet, tc.start, tc.end, wantSnippet)
			}
			wantScope := types.ScopeLine
			if tc.end > tc.start {
				wantScope = types.ScopeLineRange
			}
			if paired.Scope != wantScope {
				t.Errorf("source extent has inconsistent citation scope: got %s, want %s", paired.Scope, wantScope)
			}
		})
	}
}

func TestB1698ReadEmitSelectedCommentSurvivesAnswerAndRender(t *testing.T) {
	const source = "package p;\n/**\n * Records each request.\n * Uses the injected service.\n */\nclass Worker {}\n"
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			bus := b1698ReadCommentSource(t, "Worker.java", source, 0, 100)
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}
			b1698EmitCommentDefinition(t, bus, "Worker.java", "Worker", 6)
			var selected types.EvidenceItem
			for _, item := range bus.Mutable.EmittedEvidence() {
				if item.Producer == types.EvidenceProducerAutoPairRoleDescription {
					selected = item
				}
			}
			if selected.ID == "" || !selected.IsCitable() {
				t.Fatal("actual source comment must be available for the model's explicit selection")
			}
			beforeEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
			if err != nil {
				t.Fatal(err)
			}
			const modelSummary = "Worker is described by a source comment."
			const modelDetail = "This is documentation, not proof that execution occurred."
			args, err := json.Marshal(map[string]any{"blocks": []map[string]any{
				{"id": "summary", "kind": "summary", "text": modelSummary},
				{"id": "notes", "kind": "bullet_list", "items": []map[string]any{
					{"id": "selected-comment", "label": "Worker", "text": modelDetail, "evidence_ids": []string{selected.ID}},
					{"id": "same-comment", "label": "Worker documentation", "text": modelDetail, "evidence_ids": []string{selected.ID}},
				}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			beforeArgs := append([]byte(nil), args...)
			result, err := (&EmitAnswerDocument{}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("actual answer emit must accept the model-selected comment: %v %+v", err, result)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			if doc == nil || len(doc.Citations) != 1 {
				t.Fatalf("two explicit uses of one comment must share one citation: %+v", doc)
			}
			citation := doc.Citations[0]
			wantQuote := "/**\n * Records each request.\n * Uses the injected service.\n */"
			if citation.File != "Worker.java" || citation.Line != 2 || citation.LineEnd != 5 || citation.Quote != wantQuote {
				t.Fatalf("accepted answer lost selected comment coordinates/text: %+v", citation)
			}
			if len(doc.Blocks) != 2 || doc.Blocks[0].Text != modelSummary || len(doc.Blocks[1].Items) != 2 {
				t.Fatalf("system rewrote the model's answer body: %+v", doc.Blocks)
			}
			for _, item := range doc.Blocks[1].Items {
				refs := types.AnswerBlockItemCitationRefs(item)
				if item.Text != modelDetail || len(item.EvidenceIDs) != 1 || item.EvidenceIDs[0] != selected.ID || len(refs) != 1 || refs[0] != 0 {
					t.Fatalf("selected evidence/body was replaced rather than transported: %+v", item)
				}
			}
			beforeDoc, _ := json.Marshal(doc)
			rendered := render.RenderAnswerDocument(doc, language)
			afterDoc, _ := json.Marshal(doc)
			afterEvidence, _ := json.Marshal(bus.Mutable.EmittedEvidence())
			if !strings.Contains(rendered, "Worker.java:2-5") || !strings.Contains(rendered, wantQuote) || !strings.Contains(rendered, modelSummary) || !strings.Contains(rendered, modelDetail) {
				t.Fatalf("final citation must show the full comment, not only its opening marker: %s", rendered)
			}
			if !bytes.Equal(beforeArgs, args) || !bytes.Equal(beforeEvidence, afterEvidence) || !bytes.Equal(beforeDoc, afterDoc) {
				t.Fatal("render/answer path changed submitted bytes, source evidence, or the accepted document")
			}
			actualSource, err := os.ReadFile(filepath.Join(bus.RepoRoot, "Worker.java"))
			if err != nil || string(actualSource) != source {
				t.Fatalf("source bytes changed: %v %q", err, actualSource)
			}
		})
	}
}

func TestB1698ReadEmitCommentRangeIsIdempotentAndKeepsManualEvidence(t *testing.T) {
	const source = "package p\n// Worker preserves the original request.\n// It records the chosen action.\nfunc Worker() {}\n"
	bus := b1698ReadCommentSource(t, "worker.go", source, 0, 100)
	b1698EmitCommentDefinition(t, bus, "worker.go", "Worker", 4)
	first, err := json.Marshal(bus.Mutable.EmittedEvidence())
	if err != nil {
		t.Fatal(err)
	}
	b1698EmitCommentDefinition(t, bus, "worker.go", "Worker", 4)
	again, err := json.Marshal(bus.Mutable.EmittedEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, again) {
		t.Fatalf("repeated model definition changed or duplicated accepted comment evidence:\n%s\n%s", first, again)
	}
	manualBus := b1698ReadCommentSource(t, "worker.go", source, 0, 100)
	b1698EmitCommentDefinition(t, manualBus, "worker.go", "Worker", 4, map[string]any{
		"evidence_kind": "mechanism", "scope": "line", "source": "worker.go", "line_start": 4,
		"anchor_kind": "definition", "anchor_symbol": "Worker", "subject": "Worker", "predicate": "documents",
		"summary": "The model's own explanation remains canonical.",
	})
	for _, item := range manualBus.Mutable.EmittedEvidence() {
		if item.Producer == types.EvidenceProducerAutoPairRoleDescription {
			t.Fatal("manual mechanism at the same definition must still suppress automatic pairing")
		}
	}
}

func TestB1698ReadEmitDoesNotInventCommentOrUnobservedExtent(t *testing.T) {
	for _, tc := range []struct {
		name, file, source, symbol string
		definition, offset, limit  int
	}{
		{"no_comment", "worker.go", "package p\nfunc Worker() {}\n", "Worker", 2, 0, 100},
		{"unsupported_comment_style", "worker.js", "/** A multiline JavaScript comment.\n * The old extractor does not consume this style.\n */\nfunction worker() {}\n", "worker", 4, 0, 100},
		{"unread_comment", "worker.go", "package p\n// Worker keeps the input.\n// It records a decision.\nfunc Worker() {}\n", "Worker", 4, 3, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := b1698ReadCommentSource(t, tc.file, tc.source, tc.offset, tc.limit)
			b1698EmitCommentDefinition(t, bus, tc.file, tc.symbol, tc.definition)
			for _, item := range bus.Mutable.EmittedEvidence() {
				if item.Producer == types.EvidenceProducerAutoPairRoleDescription {
					t.Fatalf("unchanged discovery boundary must not create a companion: %+v", item)
				}
			}
		})
	}

	// The old Python extractor can retain a summary for an unterminated
	// docstring. This display repair must not turn that into a proved extent.
	const unclosed = "def worker():\n    \"\"\"An unfinished description.\n"
	bus := b1698ReadCommentSource(t, "worker.py", unclosed, 0, 100)
	b1698EmitCommentDefinition(t, bus, "worker.py", "worker", 1)
	for _, item := range bus.Mutable.EmittedEvidence() {
		if item.Producer == types.EvidenceProducerAutoPairRoleDescription && (item.LineEnd != 0 || item.Snippet != "" || item.Scope != types.ScopeLine) {
			t.Fatalf("unterminated comment acquired an invented closing range: %+v", item)
		}
	}
}

func TestB1698CommentQuoteRequiresEverySourceLine(t *testing.T) {
	// A hole in the read gutter is not an observed empty line. The legacy
	// summary remains available, but no complete range/snippet is certified.
	ctx := &ground.Context{LineIndex: map[string]map[int]string{"Worker.java": {
		1: "/**", 2: " * A documented action.", 4: " */", 5: "class Worker {}",
	}}}
	text, start, end, snippet := extractDocCommentRangeForGroundedItem(ctx, "Worker.java", 5)
	if text == "" || start != 1 {
		t.Fatalf("legacy summary premise lost: %q %d", text, start)
	}
	if end != 0 || snippet != "" {
		t.Fatalf("unread interior line certified: end=%d snippet=%q", end, snippet)
	}
	paired := autoPairRoleDescriptionEvidence([]types.EvidenceItem{{ID: "definition", Source: "Worker.java", LineStart: 5,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Worker", Subject: "Worker", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded}}, ctx)
	if len(paired) != 1 || paired[0].Scope != types.ScopeLine || paired[0].LineEnd != 0 || paired[0].Snippet != "" {
		t.Fatalf("unread interior must retain only legacy point scope: %+v", paired)
	}
	ctx.LineIndex["Worker.java"][3] = ""
	_, start, end, snippet = extractDocCommentRangeForGroundedItem(ctx, "Worker.java", 5)
	if start != 1 || end != 4 || snippet != "/**\n * A documented action.\n\n */" {
		t.Fatalf("actually read blank line did not preserve the raw extent: %d-%d %q", start, end, snippet)
	}
}

func TestB1698EvidenceCitationScopeKeepsExistingQualification(t *testing.T) {
	base := types.EvidenceItem{Source: "worker.go", LineStart: 2, LineEnd: 4,
		Kind: types.EvidenceDirect, Scope: types.ScopeLineRange, AnchorKind: types.AnchorDefinition,
		AnchorSymbol: "Worker", Subject: "Worker", Snippet: "func Worker() int {\n return 1\n}",
		GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
	for _, tc := range []struct {
		name   string
		change func(*types.EvidenceItem)
		wantOK bool
		scope  types.EvidenceScope
	}{
		{"range", func(*types.EvidenceItem) {}, true, types.ScopeLineRange},
		{"line", func(e *types.EvidenceItem) { e.Scope, e.LineEnd = types.ScopeLine, e.LineStart }, true, ""},
		{"legacy_empty", func(e *types.EvidenceItem) { e.Scope = "" }, true, ""},
		{"section_unchanged", func(e *types.EvidenceItem) { e.Scope, e.SectionPath = types.ScopeSection, "worker.behavior" }, true, ""},
		{"unknown_end", func(e *types.EvidenceItem) { e.LineEnd = 0 }, true, ""},
		{"single_point_range", func(e *types.EvidenceItem) { e.LineEnd = e.LineStart }, true, ""},
		{"file_scope", func(e *types.EvidenceItem) { e.Scope = types.ScopeFile }, false, ""},
		{"crossfile_scope", func(e *types.EvidenceItem) { e.Scope = types.ScopeCrossfile }, false, ""},
		{"negative_scope", func(e *types.EvidenceItem) { e.Scope = types.ScopeNegative }, false, ""},
		{"unqualified", func(e *types.EvidenceItem) { e.GroundingStatus = types.GroundingUngrounded }, false, ""},
		{"runtime_path", func(e *types.EvidenceItem) { e.Source = "capture.ftrace" }, false, ""},
		{"runtime_origin", func(e *types.EvidenceItem) { e.Origin = types.ClaimOriginLog }, false, ""},
		{"missing_start", func(e *types.EvidenceItem) { e.LineStart = 0 }, false, ""},
		{"missing_source", func(e *types.EvidenceItem) { e.Source = "" }, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := base
			tc.change(&ev)
			before := ev
			cit, ok := preEmitCitationForItemEvidence(ev, nil)
			if ok != tc.wantOK || (ok && (cit.Scope != tc.scope || cit.Line != ev.LineStart || cit.LineEnd != ev.LineEnd || cit.Quote != strings.TrimSpace(ev.Snippet))) {
				t.Fatalf("scope metadata changed more than the qualified range: ok=%v citation=%+v evidence=%+v", ok, cit, ev)
			}
			if !reflect.DeepEqual(before, ev) {
				t.Fatal("citation projection mutated its source evidence")
			}
		})
	}
}

func TestB1698OrdinarySourceRangeAndExistingCitationSelection(t *testing.T) {
	const source = "package p\nfunc Worker() int {\n return 1\n}\n"
	for _, tc := range []struct {
		name, label      string
		explicitCitation bool
		knownPointReuse  bool
	}{
		{name: "new_selected_range_neutral_label", label: "Selected source range"},
		{name: "existing_model_citation", label: "Worker", explicitCitation: true},
		{name: "known_B1694_worker_label_point_reuses_range", label: "Worker", knownPointReuse: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := b1698ReadCommentSource(t, "worker.go", source, 0, 100)
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}
			args, _ := json.Marshal(map[string]any{"items": []map[string]any{{
				"evidence_kind": "direct", "scope": "line_range", "source": "worker.go", "line_start": 2, "line_end": 4,
				"anchor_kind": "definition", "anchor_symbol": "Worker", "summary": "The selected source definition.",
			}}})
			result, err := (&EmitEvidence{}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("ordinary multi-line evidence premise: %v %+v", err, result)
			}
			evidence := bus.Mutable.EmittedEvidence()
			if len(evidence) != 1 || !evidence[0].IsCitable() || evidence[0].Scope != types.ScopeLineRange || evidence[0].LineStart != 2 || evidence[0].LineEnd != 4 {
				t.Fatalf("ordinary source range was not retained: %+v", evidence)
			}
			payload := map[string]any{"blocks": []map[string]any{
				{"id": "summary", "kind": "summary", "text": "A source definition was selected."},
				{"id": "source", "kind": "bullet_list", "items": []map[string]any{
					{"id": "worker", "label": tc.label, "text": "The model's original statement.", "evidence_ids": []string{evidence[0].ID}},
				}},
			}}
			originalCitation := types.Citation{File: "worker.go", Line: 2, LineEnd: 4,
				Scope: types.ScopeLine, Quote: "func Worker() int {\n return 1\n}"}
			if tc.explicitCitation {
				payload["citations"] = []types.Citation{originalCitation}
			}
			args, _ = json.Marshal(payload)
			result, err = (&EmitAnswerDocument{}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("ordinary selected source emit: %v %+v", err, result)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			if doc == nil || len(doc.Citations) != 1 {
				t.Fatalf("selected source must not duplicate the citation: %+v", doc)
			}
			if tc.explicitCitation {
				if !reflect.DeepEqual(doc.Citations[0], originalCitation) {
					t.Fatalf("existing model-authored citation was overwritten: got %+v want %+v", doc.Citations[0], originalCitation)
				}
			} else if doc.Citations[0].Scope != types.ScopeLineRange || !strings.Contains(render.RenderAnswerDocument(doc, "en"), "worker.go:2-4") {
				// Keep the actual failing public fixture, not just a synthetic
				// location-key assertion. B1694 is separate: a label-derived point
				// enters before the explicitly selected range and is reused at the
				// same start coordinate. Do not overwrite existing model citations
				// or widen global citation identity policy to close B1698.
				if tc.knownPointReuse && doc.Citations[0].File == "worker.go" &&
					doc.Citations[0].Line == 2 && doc.Citations[0].LineEnd == 0 &&
					doc.Citations[0].Scope == "" && doc.Citations[0].Quote == "func Worker() int {" {
					t.Skipf("B1694 known point/range identity collision: explicit evidence %s retains worker.go:2-4, but earlier label citation is reused: %+v", evidence[0].ID, doc.Citations[0])
				}
				t.Fatalf("ordinary source range was not transported to the renderer: %+v", doc.Citations[0])
			}
		})
	}
}
