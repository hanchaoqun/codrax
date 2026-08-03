package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/operation"
)

func TestTrimCommandOperationMaterialUTF8TailIsBoundedToOnePartialRune(t *testing.T) {
	validPrefix := bytes.Repeat([]byte("a"), commandOperationMaterialMaxSourceBytes-4)
	partial := append(append([]byte(nil), validPrefix...), 0xf0, 0x9f, 0x98)
	got, ok := trimCommandOperationMaterialUTF8Tail(partial)
	if !ok || !bytes.Equal(got, validPrefix) {
		t.Fatalf("partial final rune was not trimmed exactly: ok=%t bytes=%d want=%d", ok, len(got), len(validPrefix))
	}

	invalidBody := append([]byte(nil), validPrefix...)
	invalidBody[7] = 0xff
	invalidBody = append(invalidBody, []byte("tail")...)
	if got, ok := trimCommandOperationMaterialUTF8Tail(invalidBody); ok || got != nil {
		t.Fatalf("malformed body escaped bounded tail validation: ok=%t bytes=%d", ok, len(got))
	}
}

func TestCommandOperationMaterialPagesPublishContiguousCompleteReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual.html")
	body := "BEGIN " + strings.Repeat("完整章节内容 ", 4000) + " TAIL-SENTINEL"
	if err := os.WriteFile(path, []byte("<!doctype html><html><body><article>"+body+"</article></body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	records := commandOperationAttachMaterialPages([]commandOperationResultRecord{{
		Result: operation.CommandOperationResult{Status: operation.StatusExecuted, PayloadRef: path},
	}})
	pages := records[0].MaterialPages
	if len(pages) < 2 {
		t.Fatalf("pages=%d, want multiple bounded pages", len(pages))
	}
	for i, page := range pages {
		if page.Ordinal != i+1 || page.StartRune != i*commandOperationMaterialPageRunes {
			t.Fatalf("page[%d] range metadata=%+v", i, page)
		}
		if page.CoverageReceiptRef == "" || page.SourceTruncated || page.PagesTruncated {
			t.Fatalf("page[%d] lost complete receipt: %+v", i, page)
		}
		if i > 0 && page.StartRune != pages[i-1].EndRune {
			t.Fatalf("page gap: previous=%+v current=%+v", pages[i-1], page)
		}
	}
	last := pages[len(pages)-1]
	if last.EndRune != last.VisibleRunes || !strings.Contains(last.Content, "TAIL-SENTINEL") {
		t.Fatalf("last page does not close source: %+v", last)
	}
	rendered := renderCommandOperationRecordsForPrompt(records)
	for _, want := range []string{"material_coverage_ledger", "coverage_receipt_ref=material-coverage:v1:", "range_runes=[", "TAIL-SENTINEL"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered material missing %q", want)
		}
	}
}

func TestCommandOperationMaterialPagesWithholdReceiptAtPageCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.txt")
	content := strings.Repeat("abcdef", commandOperationMaterialPageRunes*commandOperationMaterialMaxPages/2)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	pages := commandOperationBuildMaterialPages(path)
	if len(pages) != commandOperationMaterialMaxPages {
		t.Fatalf("pages=%d want cap=%d", len(pages), commandOperationMaterialMaxPages)
	}
	if !pages[0].PagesTruncated || pages[0].CoverageReceiptRef != "" {
		t.Fatalf("truncated page set must not mint coverage receipt: %+v", pages[0])
	}
}

func TestCommandOperationCoverageAcceptsReceiptButRejectsIndividualPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("manual section ", 800)), 0o600); err != nil {
		t.Fatal(err)
	}
	records := commandOperationAttachMaterialPages([]commandOperationResultRecord{{
		Result: operation.CommandOperationResult{Status: operation.StatusExecuted, PayloadRef: path},
	}})
	pages := records[0].MaterialPages
	if len(pages) < 2 || pages[0].CoverageReceiptRef == "" {
		t.Fatalf("fixture did not produce complete multi-page coverage: %+v", pages)
	}
	valid := operationEvaluationDraft{
		Status:                 flexiblePolicyString(operation.EvalComplete),
		MaterialCoverageStatus: flexiblePolicyString(operation.MaterialCoverageComplete),
		CoverageMaterialRefs:   flexiblePolicyStringList{pages[0].CoverageReceiptRef},
	}
	if err := validateCommandOperationEvaluationCoverage(valid, records, 1); err != nil {
		t.Fatalf("complete receipt rejected: %v", err)
	}
	invalid := valid
	invalid.CoverageMaterialRefs = flexiblePolicyStringList{pages[0].Ref}
	if err := validateCommandOperationEvaluationCoverage(invalid, records, 1); err == nil {
		t.Fatal("individual page must not authorize whole-source completion")
	}
}

func TestOperationFinalAnswerAcceptsSystemMaterialCoverageReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("manual section ", 800)), 0o600); err != nil {
		t.Fatal(err)
	}
	records := commandOperationAttachMaterialPages([]commandOperationResultRecord{{
		Result: operation.CommandOperationResult{Status: operation.StatusExecuted, PayloadRef: path},
	}})
	answer := operationFinalReportWithRecordStatus("zh", "模型结论", records)
	if strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("complete system coverage receipt should suppress false caveat: %s", answer)
	}
	if answer != "模型结论" {
		t.Fatalf("system must preserve model answer byte-for-byte, got %q", answer)
	}
}
