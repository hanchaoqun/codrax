package repl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func (r *REPL) prepareDataTaskNonTextMaterials(ctx context.Context, candidates *[]dataquery.CandidateFile, plan dataquery.TaskPlan) string {
	if candidates == nil {
		return ""
	}
	required := dataquery.FindNonTextRequiredMaterials(plan.CoverageContract, *candidates)
	if len(required) == 0 {
		return ""
	}
	var withText []string
	var needsExtraction []dataquery.NonTextRequiredMaterial
	for _, item := range required {
		if len(item.TextEvidencePaths) > 0 {
			withText = append(withText, fmt.Sprintf("%s has related text evidence %s", item.Path, strings.Join(item.TextEvidencePaths, ", ")))
			continue
		}
		needsExtraction = append(needsExtraction, item)
	}
	if len(withText) > 0 {
		return "data material coverage needs repair: required non-text material(s) must not be read directly by the deterministic runner; use the related text evidence path(s) instead: " + strings.Join(withText, "; ")
	}
	if len(needsExtraction) == 0 {
		return ""
	}
	if r.dataMaterialExtractor == nil {
		var parts []string
		for _, item := range needsExtraction {
			parts = append(parts, fmt.Sprintf("%s (%s)", item.Path, item.Kind))
		}
		return "data material coverage incomplete: required non-text material(s) need text extraction before deterministic processing: " + strings.Join(parts, ", ") + "; configure providers.yaml agents.multimodal_material_extractor or provide extracted text evidence"
	}
	outputRoot := filepath.Join(r.repoRoot, ".codrax", "data-extract")
	extractions, err := r.dataMaterialExtractor.ExtractDataMaterials(ctx, r.repoRoot, outputRoot, needsExtraction)
	if err != nil {
		return "data material extraction failed: " + err.Error()
	}
	if len(extractions) == 0 {
		return "data material extraction produced no text evidence for required non-text material(s)"
	}
	var paths []string
	for _, extraction := range extractions {
		if strings.TrimSpace(extraction.TextPath) == "" {
			continue
		}
		paths = append(paths, extraction.TextPath)
		size := int64(0)
		if info, err := os.Stat(filepath.Join(r.repoRoot, extraction.TextPath)); err == nil {
			size = info.Size()
		}
		*candidates = append(*candidates, dataquery.CandidateFile{
			Path:             extraction.TextPath,
			Size:             size,
			Kind:             "text",
			ExtractionStatus: "extracted_text",
			InspectError:     strings.TrimSpace(extraction.Notes),
		})
	}
	if len(paths) == 0 {
		return "data material extraction produced no usable text evidence path"
	}
	return "data material extraction completed for required non-text material(s); repair the plan to use extracted text evidence path(s) in input_paths and coverage_contract: " + strings.Join(paths, ", ")
}
