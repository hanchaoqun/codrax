package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnswerDocumentV2_PreservesSelectedCitationWithoutObservedSupportingMember(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	loggerSource := strings.Repeat("// pad\n", 35) + "sink_->write(line);\n"
	if err := os.WriteFile(filepath.Join(repo, "src", "logger.cpp"), []byte(loggerSource), 0o644); err != nil {
		t.Fatalf("write logger fixture: %v", err)
	}
	registrySource := strings.Repeat("// pad\n", 14) + "static std::unique_ptr<Sink> create();\n"
	if err := os.WriteFile(filepath.Join(repo, "src", "registry.cpp"), []byte(registrySource), 0o644); err != nil {
		t.Fatalf("write registry fixture: %v", err)
	}

	mu := types.NewMutableState("supporting aggregate pipeline")
	mu.SetRepoRoot(repo)
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "runtime path", Value: "1",
		Role: types.AnswerAggregateRoleSupportingCoverage, Members: []string{"sink_->write"},
		SupportRefs: []string{"sink_->write @ src/logger.cpp:36"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu, RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}},
	}
	raw := json.RawMessage(`{
		"blocks": [
			{"id":"summary","kind":"summary","surface_role":"principal","text":"The logger delegates output through its sink."},
			{"id":"path","kind":"ordered_list","items":[{"id":"sink","label":"sink_->write","text":"virtual dispatch call","citation_ref":0}]}
		],
		"citations": [{"file":"src/registry.cpp","line":15,"quote":"static std::unique_ptr<Sink> create();"}]
	}`)

	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil {
		t.Fatalf("emit transport error: %v", err)
	}
	if !res.Success {
		t.Fatalf("emit rejected: %s", res.Summary)
	}
	doc := mu.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) < 2 || len(doc.Blocks[1].Items) != 1 {
		t.Fatalf("accepted document missing path item: %+v", doc)
	}
	item := doc.Blocks[1].Items[0]
	if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
		t.Fatalf("exact supporting citation was not retained: item=%+v citations=%+v", item, doc.Citations)
	}
	cit := doc.Citations[item.CitationRef]
	// The source file exists, but no read/typed observation established this
	// member. A model aggregate cannot relocate the model-selected citation.
	// Actual Read -> Emit operation-site binding is covered by B1700's public
	// call/return positive matrix, without manufacturing Grounded here.
	if cit.File != "src/registry.cpp" || cit.Line != 15 || len(doc.Citations) != 1 {
		t.Fatalf("unobserved supporting member rewrote selected citation: %+v", doc.Citations)
	}
	if item.Label != "sink_->write" || item.Text != "virtual dispatch call" || !item.CitationRefsModelSubmitted {
		t.Fatalf("model content/selection ownership changed: %+v", item)
	}
}
