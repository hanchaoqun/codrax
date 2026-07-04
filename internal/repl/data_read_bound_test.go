package repl

// DQA F2 behavior witnesses for the repl-side data-lane bounded reads.
// The structural pin lives in
// internal/dataquery/read_bound_pin_test.go (dataLaneExternalPinFiles);
// these tests witness the runtime behavior the pin protects: oversized
// user-reachable files are refused with the typed
// width.ErrSourceReadOversized (fail-loud, no silent truncation, no
// unbounded slurp).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool/width"
)

// Oversized workflow-checkpoint resume file: the resume path is
// user-supplied (--data-resume), so a mispointed path at a huge artifact
// must refuse typed instead of slurping.
func TestLoadDataTaskWorkflowResumeFileRefusesOversizeTyped(t *testing.T) {
	dataquery.SetDefaultMaxFileBytes(256)
	defer dataquery.SetDefaultMaxFileBytes(0)
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	body := `{"data_rounds":1,"repair_rounds":0,"resume":{"records":[` +
		strings.TrimSuffix(strings.Repeat("{},", 120), ",") + `]}}`
	if len(body) <= 256 {
		t.Fatalf("fixture must exceed the 256-byte bound, got %d bytes", len(body))
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write resume fixture: %v", err)
	}
	_, err := loadDataTaskWorkflowResumeFile(path)
	if err == nil {
		t.Fatal("expected typed oversize refusal, got nil error")
	}
	var oversize *width.ErrSourceReadOversized
	if !errors.As(err, &oversize) {
		t.Fatalf("expected *width.ErrSourceReadOversized in chain, got %T: %v", err, err)
	}
	if oversize.Size != int64(len(body)) || oversize.Cap != 256 {
		t.Fatalf("refusal must carry observed size and cap: got size=%d cap=%d want size=%d cap=256",
			oversize.Size, oversize.Cap, len(body))
	}
	if !strings.Contains(err.Error(), "read data workflow checkpoint") {
		t.Fatalf("refusal must keep the checkpoint-read context, got %q", err.Error())
	}
	// Same file under a sufficient bound loads fine — the refusal above is
	// the bound, not a parse defect in the fixture.
	dataquery.SetDefaultMaxFileBytes(1 << 20)
	if _, err := loadDataTaskWorkflowResumeFile(path); err != nil {
		t.Fatalf("fixture must load once the bound allows it, got %v", err)
	}
}

// mustNotChatAdapter fails the test if the material extractor reaches the
// LLM: the oversize refusal must fire BEFORE the bytes are base64-inflated
// into a provider message.
type mustNotChatAdapter struct{ t *testing.T }

func (a mustNotChatAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	a.t.Fatal("adapter.Chat called for an oversized material — refusal must precede the LLM call")
	return llm.Response{}, errors.New("unreachable")
}
func (a mustNotChatAdapter) ModelID() string               { return "must-not-chat" }
func (a mustNotChatAdapter) MaxContextTokens() int         { return 4096 }
func (a mustNotChatAdapter) MaxOutputTokens() int          { return 0 }
func (a mustNotChatAdapter) RequestTimeout() time.Duration { return 0 }
func (a mustNotChatAdapter) RetryMaxAttempts() int         { return 0 }

// Oversized image material: the user-named non-text material is refused
// typed before any base64 inflation / LLM round-trip.
func TestExtractDataMaterialsRefusesOversizeImageTyped(t *testing.T) {
	dataquery.SetDefaultMaxFileBytes(256)
	defer dataquery.SetDefaultMaxFileBytes(0)
	dir := t.TempDir()
	img := filepath.Join(dir, "chart.png")
	payload := make([]byte, 400)
	if err := os.WriteFile(img, payload, 0600); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}
	extractor := &llmDataMaterialExtractor{adapter: mustNotChatAdapter{t: t}}
	_, err := extractor.ExtractDataMaterials(context.Background(), dir, "",
		[]dataquery.NonTextRequiredMaterial{{Path: "chart.png", Kind: "image"}})
	if err == nil {
		t.Fatal("expected typed oversize refusal, got nil error")
	}
	var oversize *width.ErrSourceReadOversized
	if !errors.As(err, &oversize) {
		t.Fatalf("expected *width.ErrSourceReadOversized in chain, got %T: %v", err, err)
	}
	if oversize.Size != int64(len(payload)) || oversize.Cap != 256 {
		t.Fatalf("refusal must carry observed size and cap: got size=%d cap=%d want size=%d cap=256",
			oversize.Size, oversize.Cap, len(payload))
	}
}
