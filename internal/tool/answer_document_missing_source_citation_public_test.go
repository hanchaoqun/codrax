package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

const missingSourceCitationModelText = "trace_capabilities is an ordinary identifier; this model-authored explanation stays unchanged."

func missingSourceCitationPublicCall(t *testing.T, registry *Registry, ctx *types.BusContext, name string, payload any) *types.AnswerDocumentV2 {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(raw)
	result, err := registry.Execute(ctx, name, raw)
	if err != nil || !result.Success {
		t.Fatalf("registered %s failed: %v %+v", name, err, result)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("tool mutated model input JSON")
	}
	return ctx.Mutable.AnswerDocumentV2()
}

func missingSourceCitationPublicBlock(bad, first, last int) map[string]any {
	return map[string]any{"id": "rows", "kind": "bullet_list", "items": []any{
		map[string]any{"id": "missing", "text": "The model's first row.", "citation_ref": bad},
		map[string]any{"id": "first", "text": "The model's second row.", "citation_ref": first},
		map[string]any{"id": "combined", "text": "The model's third row.", "citation_ref": bad, "citation_refs": []int{first, last}},
		map[string]any{"id": "last", "text": "The model's fourth row.", "citation_ref": last},
	}}
}

func TestMissingSourceCitationPublicEmitAndPatch(t *testing.T) {
	for _, target := range []string{"absent.go", "missing-parent/child.go", "not-a-directory/child.go", "trace_capabilities"} {
		for _, patch := range []bool{false, true} {
			name := "emit/" + target
			if patch {
				name = "patch/" + target
			}
			t.Run(name, func(t *testing.T) {
				repo := t.TempDir()
				for _, path := range []string{"first.go", "last.go", "not-a-directory"} {
					if err := os.WriteFile(filepath.Join(repo, path), []byte("package fixture\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				ctx := &types.BusContext{RepoRoot: repo, Language: "en", Mutable: types.NewMutableState("explain")}
				registry := NewRegistry()
				RegisterDefaults(registry)
				registry.Register(&EmitAnswerDocument{})
				registry.Register(&EmitAnswerDocumentPatch{})
				first := types.Citation{File: "first.go", Line: 1, Quote: "package fixture"}
				last := types.Citation{File: "last.go", Line: 1, Quote: "package fixture"}
				missing := types.Citation{File: target, Line: 1, Quote: "PHANTOM_SOURCE_QUOTE"}
				summary := map[string]any{"id": "summary", "kind": "summary", "text": missingSourceCitationModelText}
				var doc *types.AnswerDocumentV2
				if patch {
					missingSourceCitationPublicCall(t, registry, ctx, "emit_answer_document", map[string]any{
						"blocks": []any{summary, missingSourceCitationPublicBlock(0, 0, 1)}, "citations": []types.Citation{first, last},
					})
					doc = missingSourceCitationPublicCall(t, registry, ctx, "emit_answer_document_patch", map[string]any{
						"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{missingSourceCitationPublicBlock(2, 0, 1)},
						"append_citations": []types.Citation{missing},
					})
				} else {
					doc = missingSourceCitationPublicCall(t, registry, ctx, "emit_answer_document", map[string]any{
						"blocks": []any{summary, missingSourceCitationPublicBlock(1, 0, 2)}, "citations": []types.Citation{first, missing, last},
					})
				}
				missingSourceCitationAssertPublic(t, doc, []types.Citation{first, last})
				// The existing reasoning-graph census is refreshed on each patch.
				// Pin the model document and public citation pool, not that audit
				// snapshot's pre-persist counts.
				before, _ := json.Marshal([]any{doc.Blocks, doc.Citations})
				doc = missingSourceCitationPublicCall(t, registry, ctx, "emit_answer_document_patch", map[string]any{
					"unchanged_block_ids": []string{"summary", "rows"},
				})
				after, _ := json.Marshal([]any{doc.Blocks, doc.Citations})
				if !bytes.Equal(before, after) {
					t.Fatalf("no-op patch changed the accepted citation pool or model document:\nbefore=%s\nafter=%s", before, after)
				}
				missingSourceCitationAssertPublic(t, doc, []types.Citation{first, last})
				for _, path := range []string{"first.go", "last.go", "not-a-directory"} {
					content, err := os.ReadFile(filepath.Join(repo, path))
					if err != nil || string(content) != "package fixture\n" {
						t.Fatalf("source bytes changed: %s %q %v", path, content, err)
					}
				}
			})
		}
	}
}

func missingSourceCitationAssertPublic(t *testing.T, doc *types.AnswerDocumentV2, want []types.Citation) {
	t.Helper()
	if doc == nil || !reflect.DeepEqual(doc.Citations, want) {
		t.Fatalf("only existing current-source citations may remain: %+v", doc)
	}
	if blockByID(t, doc, "summary").Text != missingSourceCitationModelText {
		t.Fatal("model explanation was rewritten")
	}
	items := blockByID(t, doc, "rows").Items
	wantTexts := []string{"The model's first row.", "The model's second row.", "The model's third row.", "The model's fourth row."}
	if len(items) != len(wantTexts) {
		t.Fatalf("model row count changed: %+v", items)
	}
	for i, refs := range [][]int{nil, {0}, {0, 1}, {1}} {
		if items[i].Text != wantTexts[i] {
			t.Fatalf("model row %d changed: %+v", i, items[i])
		}
		if got := types.AnswerBlockItemCitationRefs(items[i]); !slices.Equal(got, refs) {
			t.Errorf("row %d retains a dropped or incorrectly remapped ref: %v, want %v", i, got, refs)
		}
	}
	for _, language := range []string{"en", "zh"} {
		before, _ := json.Marshal(doc)
		text := render.RenderAnswerDocumentWithAttachments(doc, nil, language)
		after, _ := json.Marshal(doc)
		if strings.Contains(text, "PHANTOM_SOURCE_QUOTE") || !strings.Contains(text, "first.go:1") || !strings.Contains(text, "last.go:1") {
			t.Fatalf("rendered citation appendix lost source identity or retained the nonexistent quote: %s", text)
		}
		if !strings.Contains(text, missingSourceCitationModelText) || !bytes.Equal(before, after) {
			t.Fatal("render changed model content or accepted document")
		}
	}
}

func TestMissingSourceCitationPublicRealToolNamedFileRemainsLegal(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "trace_capabilities"), []byte("real source content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repo, Mutable: types.NewMutableState("explain")}
	registry := NewRegistry()
	RegisterDefaults(registry)
	registry.Register(&EmitAnswerDocument{})
	registry.Register(&EmitAnswerDocumentPatch{})
	doc := missingSourceCitationPublicCall(t, registry, ctx, "emit_answer_document", map[string]any{
		"blocks": []any{map[string]any{"id": "summary", "kind": "summary", "text": missingSourceCitationModelText,
			"items": []any{map[string]any{"id": "source", "citation_ref": 0}}}},
		"citations": []types.Citation{{File: "trace_capabilities", Line: 1, Quote: "stale quote"}},
	})
	if len(doc.Citations) != 1 || doc.Citations[0].File != "trace_capabilities" || doc.Citations[0].Quote != "real source content" {
		t.Fatalf("a real file named like a tool must keep ordinary source semantics: %+v", doc)
	}
}
