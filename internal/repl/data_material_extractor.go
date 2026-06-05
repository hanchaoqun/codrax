package repl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

type DataMaterialExtractor interface {
	ExtractDataMaterials(ctx context.Context, repoRoot, outputRoot string, materials []dataquery.NonTextRequiredMaterial) ([]dataquery.MaterialExtraction, error)
}

type llmDataMaterialExtractor struct {
	adapter llm.Adapter
}

func NewDataMaterialExtractor(adapter llm.Adapter) DataMaterialExtractor {
	if adapter == nil {
		return nil
	}
	return &llmDataMaterialExtractor{adapter: adapter}
}

var dataMaterialExtractionTool = llm.ToolSchema{
	Name:        "emit_material_extraction",
	Description: "Emit extracted text evidence from one non-text data material. This tool does not execute code.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "source_path": {"type": "string"},
    "text": {
      "type": "string",
      "description": "Readable text and structured facts extracted from the provided material. Preserve numbers, dates, labels, table-like rows, and uncertainty. Do not infer missing facts."
    },
    "confidence": {"type": "string"},
    "notes": {"type": "string"}
  },
  "required": ["source_path", "text"]
}`),
}

const dataMaterialExtractionSystemPrompt = `You extract text evidence from one non-text material for a read-only data-processing workflow.

Hard rules:
- Emit only emit_material_extraction.
- Extract readable text, table-like rows, labels, numeric values, dates, identifiers, and explicit relationships.
- Do not infer missing facts. Mark uncertainty in notes.
- The output text becomes a local text evidence material for deterministic data processing; keep it faithful and compact.`

type dataMaterialExtractionDraft struct {
	SourcePath flexiblePolicyString `json:"source_path"`
	Text       flexiblePolicyString `json:"text"`
	Confidence flexiblePolicyString `json:"confidence"`
	Notes      flexiblePolicyString `json:"notes"`
}

func (e *llmDataMaterialExtractor) ExtractDataMaterials(ctx context.Context, repoRoot, outputRoot string, materials []dataquery.NonTextRequiredMaterial) ([]dataquery.MaterialExtraction, error) {
	if e == nil || e.adapter == nil {
		return nil, errors.New("multimodal material extractor is not configured")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	outputRoot = strings.TrimSpace(outputRoot)
	if outputRoot == "" {
		outputRoot = filepath.Join(absRoot, ".codrax", "data-extract")
	}
	if err := os.MkdirAll(outputRoot, 0700); err != nil {
		return nil, err
	}
	var out []dataquery.MaterialExtraction
	for _, material := range materials {
		kind := strings.TrimSpace(material.Kind)
		if kind != "image" {
			return out, fmt.Errorf("material %s has media kind %s; configured extractor does not support this kind yet", material.Path, kind)
		}
		extraction, err := e.extractOne(ctx, absRoot, outputRoot, material)
		if err != nil {
			return out, err
		}
		out = append(out, extraction)
	}
	return out, nil
}

func (e *llmDataMaterialExtractor) extractOne(ctx context.Context, absRoot, outputRoot string, material dataquery.NonTextRequiredMaterial) (dataquery.MaterialExtraction, error) {
	rel := strings.TrimSpace(material.Path)
	if rel == "" {
		return dataquery.MaterialExtraction{}, errors.New("empty non-text material path")
	}
	abs := rel
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(absRoot, rel)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return dataquery.MaterialExtraction{}, err
	}
	if !pathUnderRoot(absRoot, abs) {
		return dataquery.MaterialExtraction{}, fmt.Errorf("non-text material %s is outside data root", rel)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return dataquery.MaterialExtraction{}, err
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(abs)))
	if mimeType == "" {
		mimeType = "image/png"
	}
	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw)
	resp, err := e.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataMaterialExtractionSystemPrompt},
			{
				Role:    "user",
				Content: fmt.Sprintf("Extract text evidence from source_path=%s for downstream deterministic data processing.", rel),
				ContentParts: []llm.ContentPart{
					{Type: "image_url", ImageURL: dataURL, Detail: "high"},
				},
			},
		},
		[]llm.ToolSchema{dataMaterialExtractionTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	if err != nil {
		return dataquery.MaterialExtraction{}, err
	}
	if len(resp.ToolCalls) == 0 {
		return dataquery.MaterialExtraction{}, errors.New("material extractor returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != dataMaterialExtractionTool.Name {
		return dataquery.MaterialExtraction{}, fmt.Errorf("material extractor returned unexpected tool %q", call.Name)
	}
	var parsed dataMaterialExtractionDraft
	if err := unmarshalReplStructuredToolParams(dataMaterialExtractionTool, call.Params, &parsed, "material extractor"); err != nil {
		return dataquery.MaterialExtraction{}, err
	}
	text := strings.TrimSpace(string(parsed.Text))
	if text == "" {
		return dataquery.MaterialExtraction{}, fmt.Errorf("material extractor returned empty text for %s", rel)
	}
	stamp := time.Now().Format("20060102-150405")
	name := sanitizeDataExtractionName(rel)
	textRel := filepath.ToSlash(filepath.Join(".codrax", "data-extract", stamp+"-"+name+".txt"))
	textAbs := filepath.Join(absRoot, textRel)
	if err := os.MkdirAll(filepath.Dir(textAbs), 0700); err != nil {
		return dataquery.MaterialExtraction{}, err
	}
	if err := os.WriteFile(textAbs, []byte(text+"\n"), 0600); err != nil {
		return dataquery.MaterialExtraction{}, err
	}
	extraction := dataquery.MaterialExtraction{
		SourcePath: rel,
		TextPath:   textRel,
		Kind:       "image",
		Confidence: strings.TrimSpace(string(parsed.Confidence)),
		Notes:      strings.TrimSpace(string(parsed.Notes)),
	}
	logging.Info("[repl/data] material extracted source=%s text=%s confidence=%s notes=%q",
		extraction.SourcePath, extraction.TextPath, extraction.Confidence, oneLineClamp(extraction.Notes, 160))
	return extraction, nil
}

func pathUnderRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func sanitizeDataExtractionName(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	if base == "" || base == "." {
		base = "material"
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		out = "material"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
