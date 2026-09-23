package tool

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMissingSourceCitationConservativeBoundaries(t *testing.T) {
	repo, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "not-a-directory"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"dangling": filepath.Join(repo, "absent-target"),
		"outside":  outside,
		"inside":   filepath.Join(repo, "directory"),
	} {
		if err := os.Symlink(target, filepath.Join(repo, name)); err != nil {
			t.Fatal(err)
		}
	}
	sparseFile(t, repo, "oversized.go", width.SourceReadMaxBytes+1)
	sensitive := filepath.Join(repo, "private-credentials.yaml")
	oldSensitive := types.SensitiveConfigFilePaths()
	types.SetSensitiveConfigFilePaths(append(oldSensitive, sensitive))
	t.Cleanup(func() { types.SetSensitiveConfigFilePaths(oldSensitive) })
	for _, mode := range []string{
		"runtime_attachment", "runtime_shape", "web", "mcp", "negative_pattern", "negative_scope",
		"section_scope", "denied", "source_excluded", "sensitive", "oversized", "directory",
		"outside_relative", "outside_absolute", "dangling", "symlink_parent_outside", "symlink_parent_inside",
		"missing_root", "non_directory_root", "multirepo_denied", "multirepo_alias", "external_claim", "mixed_claim",
	} {
		t.Run(mode, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: repo}
			cit := types.Citation{File: "absent.go", Line: 1, Quote: "the original model quote"}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "row", Kind: types.BlockSummary,
				Text: "original model prose", Items: []types.AnswerBlockItem{{ID: "ref", CitationRef: 0}}}}}
			switch mode {
			case "runtime_attachment":
				ctx.AttachedHitraceSource = cit.File
			case "runtime_shape":
				cit.File = "absent.systrace"
			case "web":
				cit.File = "https://example.invalid/source.go"
			case "mcp":
				cit.File = "mcp://server/document"
			case "negative_pattern":
				cit.NegativePattern = "not-found"
			case "negative_scope":
				cit.Scope = types.ScopeNegative
			case "section_scope":
				cit.Scope, cit.SectionPath = types.ScopeSection, "API"
			case "denied":
				ctx.TypedDenials = types.NewTypedDenialSet()
				ctx.TypedDenials.Add(types.TypedDenial{Class: types.TypedDenialAttachedExtractedUnscoped, Token: cit.File})
			case "source_excluded":
				ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"Only this artifact."},
				}}}
			case "sensitive":
				cit.File = sensitive
			case "oversized", "directory", "dangling":
				cit.File = mode
				if mode == "oversized" {
					cit.File += ".go"
				}
			case "outside_relative":
				cit.File = "../absent.go"
			case "outside_absolute":
				cit.File = filepath.Join(outside, "absent.go")
			case "symlink_parent_outside":
				cit.File = "outside/absent.go"
			case "symlink_parent_inside":
				cit.File = "inside/absent.go"
			case "missing_root":
				ctx.RepoRoot = filepath.Join(repo, "absent-root")
			case "non_directory_root":
				ctx.RepoRoot = filepath.Join(repo, "not-a-directory")
			case "multirepo_denied":
				ctx.MultiGraph = missingSourceCitationActiveSet{types.ActiveSetGateResult{Allowed: false}}
			case "multirepo_alias":
				ctx.MultiGraph = missingSourceCitationActiveSet{types.ActiveSetGateResult{Allowed: true, AutoPrefixed: true, ResolvedPath: "other/absent.go"}}
			case "external_claim", "mixed_claim":
				doc.Blocks[0].ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
				if mode == "mixed_claim" {
					doc.Blocks[0].ClaimUses = append(doc.Blocks[0].ClaimUses, types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact})
				}
			}
			doc.Citations = []types.Citation{cit}
			before, _ := json.Marshal(doc)
			if changed := normalizeInvalidCurrentSourceCitationRows(doc, ctx); changed != 0 {
				t.Fatalf("uncertain/non-source citation was removed: changed=%d doc=%+v", changed, doc)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("existing non-source or unknown-read semantics changed")
			}
		})
	}
}

func TestMissingSourceCitationPermissionFailureIsNotAbsence(t *testing.T) {
	repo := t.TempDir()
	locked := filepath.Join(repo, "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if _, err := os.Lstat(filepath.Join(locked, "absent.go")); !errors.Is(err, fs.ErrPermission) {
		t.Skipf("host cannot reproduce a permission-denied ancestor: %v", err)
	}
	doc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "locked/absent.go", Line: 1, Quote: "retained"}}}
	if changed := normalizeInvalidCurrentSourceCitationRows(doc, &types.BusContext{RepoRoot: repo}); changed != 0 || len(doc.Citations) != 1 {
		t.Fatalf("permission failure is not proof of absence: %+v", doc)
	}
}

type missingSourceCitationActiveSet struct{ result types.ActiveSetGateResult }

func (g missingSourceCitationActiveSet) ResolveActiveSetPath(_ *types.BusContext, _, _ string, _ func(string) bool) types.ActiveSetGateResult {
	return g.result
}

func (g missingSourceCitationActiveSet) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return g.result
}
