package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// produceStrictDecodeErr returns an actual strict-decode error
// from the json package so tests exercise the real shape rather
// than a hand-crafted error string.
func produceStrictDecodeErr(t *testing.T, payload string) error {
	t.Helper()
	type innerObj struct {
		Form string `json:"form"`
	}
	type wrapper struct {
		Inner innerObj `json:"inner"`
	}
	var w wrapper
	dec := json.NewDecoder(bytes.NewReader([]byte(payload)))
	dec.DisallowUnknownFields()
	err := dec.Decode(&w)
	if err == nil {
		t.Fatalf("expected decode error for payload %q", payload)
	}
	return err
}

func TestRemapStrictDecodeError_NoHintsPassesThrough(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","extra":1}}`)
	got := RemapStrictDecodeError(original, nil)
	if got != original {
		t.Errorf("nil hints must return original err untouched")
	}
}

func TestRemapStrictDecodeError_MismatchedFieldPassesThrough(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","other":1}}`)
	hints := []MisplacedFieldHint{
		{Field: "citation_ref", ContainerNames: []string{"claim_use"}, CorrectPaths: []string{"items[i].citation_ref"}},
	}
	got := RemapStrictDecodeError(original, hints)
	if got != original {
		t.Errorf("hint with non-matching field MUST pass original err through; got %v", got)
	}
}

func TestRemapStrictDecodeError_MatchedFieldRewritesMessage(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","citation_ref":7}}`)
	hints := []MisplacedFieldHint{
		{
			Field:          "citation_ref",
			ContainerNames: []string{"claim_use", "claim_uses"},
			CorrectPaths: []string{
				"items[i].citation_ref",
				"value.citation_ref",
				"boolean.citation_ref",
			},
		},
	}
	got := RemapStrictDecodeError(original, hints)
	if got == original {
		t.Fatal("matched hint MUST wrap the error")
	}
	msg := got.Error()
	for _, want := range []string{
		`field "citation_ref"`,
		"items[i].citation_ref",
		"value.citation_ref",
		"boolean.citation_ref",
		"NOT inside claim_use",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("rewritten message missing %q; got %q", want, msg)
		}
	}
	// Wrapped error MUST still unwrap to the original (errors.Is /
	// errors.Unwrap chain preserved).
	if !errors.Is(got, original) {
		t.Errorf("wrapped err MUST unwrap to original (errors.Is broken)")
	}
}

func TestRemapStrictDecodeError_NilErrorReturnsNil(t *testing.T) {
	got := RemapStrictDecodeError(nil, []MisplacedFieldHint{
		{Field: "x", ContainerNames: []string{"y"}, CorrectPaths: []string{"z"}},
	})
	if got != nil {
		t.Errorf("nil err MUST return nil; got %v", got)
	}
}

func TestExtractUnknownFieldName_RealStrictDecodeError(t *testing.T) {
	err := produceStrictDecodeErr(t, `{"inner":{"form":"x","banana":1}}`)
	got := extractUnknownFieldName(err)
	if got != "banana" {
		t.Errorf("got %q, want %q (err msg=%q)", got, "banana", err.Error())
	}
}

func TestExtractUnknownFieldName_NonStrictErrorReturnsEmpty(t *testing.T) {
	err := errors.New("some other error")
	if got := extractUnknownFieldName(err); got != "" {
		t.Errorf("non-strict err must yield empty; got %q", got)
	}
}

func TestExtractUnknownFieldName_NilReturnsEmpty(t *testing.T) {
	if got := extractUnknownFieldName(nil); got != "" {
		t.Errorf("nil err must yield empty; got %q", got)
	}
}

// ── R4 紧急修 (m1a-20260504-141035 forensic) — 错误信息 sanitize ──

// TestRemapStrictDecodeError_CannotUnmarshalStringRewritten pins
// the streaming-artefact rewrite: LLM emits blocks[] as a JSON-
// encoded string ("[{...},{...}]") instead of a real array; Go
// strict-decode rejects with `cannot unmarshal string into Go
// struct field ...blocks of type ...`. The remap MUST surface
// LLM-actionable guidance + strip Go type names (R4).
func TestRemapStrictDecodeError_CannotUnmarshalStringRewritten(t *testing.T) {
	original := errors.New(
		`json: cannot unmarshal string into Go struct field emitAnswerDocumentV2Params.blocks of type []tool.emitAnswerBlockV2`)
	got := RemapStrictDecodeError(original, nil)
	if got == original {
		t.Fatal("cannot-unmarshal-string MUST be rewritten")
	}
	msg := got.Error()
	for _, want := range []string{
		`"blocks" field`,
		"native JSON array",
		"not a JSON-encoded string",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("rewritten message missing %q; got: %s", want, msg)
		}
	}
	// R4: Go-internal type names MUST NOT leak.
	for _, banned := range []string{
		"emitAnswerDocumentV2Params",
		"tool.emitAnswerBlockV2",
		"Go struct field",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("R4 leak — Go-internal token %q present in LLM-facing message: %s", banned, msg)
		}
	}
}

func TestStrictDecodeToolRepair_MisplacedField(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","citation_ref":7}}`)
	repair := strictDecodeToolRepair(original, []MisplacedFieldHint{{
		Field:          "citation_ref",
		ContainerNames: []string{"claim_use", "claim_uses"},
		CorrectPaths:   []string{"items[i].citation_ref", "value.citation_ref"},
	}})
	if repair == nil {
		t.Fatal("expected structured repair metadata")
	}
	if repair.Code != "tool_param_misplaced_field" {
		t.Fatalf("repair code = %q", repair.Code)
	}
	for _, want := range []string{"items[i].citation_ref", "value.citation_ref"} {
		if !slices.Contains(repair.Fields, want) {
			t.Fatalf("repair fields missing %q: %+v", want, repair.Fields)
		}
	}
	if repair.Metadata["field"] != "citation_ref" ||
		!strings.Contains(repair.Metadata["invalid_containers"], "claim_use") {
		t.Fatalf("repair metadata incomplete: %+v", repair.Metadata)
	}
}

func TestStrictDecodeToolRepair_UnknownField(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","extra":1}}`)
	repair := strictDecodeToolRepair(original, nil)
	if repair == nil || repair.Code != "tool_param_unknown_field" {
		t.Fatalf("expected unknown-field repair metadata, got %+v", repair)
	}
	if len(repair.Fields) != 1 || repair.Fields[0] != "extra" {
		t.Fatalf("repair fields = %+v", repair.Fields)
	}
	if !strings.Contains(repair.Hint, "without changing the answer facts") {
		t.Fatalf("repair hint should protect model content: %q", repair.Hint)
	}
}

func TestStrictDecodeToolRepair_JSONStringCarrier(t *testing.T) {
	original := errors.New(
		`json: cannot unmarshal string into Go struct field emitAnswerDocumentV2Params.blocks of type []tool.emitAnswerBlockV2`)
	repair := strictDecodeToolRepair(original, nil)
	if repair == nil || repair.Code != "tool_param_json_string_carrier" {
		t.Fatalf("expected JSON-string carrier repair metadata, got %+v", repair)
	}
	if len(repair.Fields) != 1 || repair.Fields[0] != "blocks" {
		t.Fatalf("repair fields = %+v", repair.Fields)
	}
	if !strings.Contains(repair.Hint, "native JSON") ||
		!strings.Contains(repair.Hint, "Preserve the same rows") {
		t.Fatalf("repair hint should explain native JSON without content rewrite: %q", repair.Hint)
	}
}

func TestFailStrictDecode_AttachesRepairAndSanitizedSummary(t *testing.T) {
	original := errors.New(
		`json: cannot unmarshal string into Go struct field emitAnswerDocumentV2Params.blocks of type []tool.emitAnswerBlockV2`)
	res, err := failStrictDecode("emit_answer_document", time.Now(), original, nil)
	if err != nil {
		t.Fatalf("failStrictDecode should return tool-level failure without Go error: %v", err)
	}
	if res.Success {
		t.Fatal("strict decode failure must not succeed")
	}
	if res.Repair == nil || res.Repair.Code != "tool_param_json_string_carrier" {
		t.Fatalf("missing repair metadata: %+v", res)
	}
	for _, banned := range []string{"emitAnswerDocumentV2Params", "tool.emitAnswerBlockV2", "Go struct field"} {
		if strings.Contains(res.Summary, banned) {
			t.Fatalf("summary leaked internal decode token %q: %s", banned, res.Summary)
		}
	}
}

func TestFailStrictDecodeWithError_AttachesRepairAndReturnsError(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","extra":1}}`)
	res, err := failStrictDecodeWithError("emit_perf_trace", time.Now(), original, nil)
	if err == nil {
		t.Fatal("failStrictDecodeWithError must preserve the historical non-nil error return")
	}
	if res.Success {
		t.Fatal("strict decode failure must not succeed")
	}
	if res.Repair == nil || res.Repair.Code != "tool_param_unknown_field" {
		t.Fatalf("missing repair metadata: %+v", res)
	}
	if !strings.Contains(res.Summary, `unknown field "extra"`) {
		t.Fatalf("summary should preserve sanitized decode failure: %s", res.Summary)
	}
}

func TestFailStrictDecodeWithErrorMessage_PreservesReminderAndRepair(t *testing.T) {
	original := produceStrictDecodeErr(t, `{"inner":{"form":"x","extra":1}}`)
	res, err := failStrictDecodeWithErrorMessage("emit_change_plan", time.Now(), original, nil, "emit_change_plan rejected: ", ". REQUIRED schema: {...}")
	if err == nil {
		t.Fatal("expected non-nil decode error")
	}
	if res.Repair == nil || res.Repair.Code != "tool_param_unknown_field" {
		t.Fatalf("missing repair metadata: %+v", res.Repair)
	}
	if !strings.HasPrefix(res.Summary, "emit_change_plan rejected: invalid params:") ||
		!strings.Contains(res.Summary, "REQUIRED schema") {
		t.Fatalf("summary should preserve tool-specific prefix and reminder: %s", res.Summary)
	}
}

// TestRemapStrictDecodeError_SanitizeStripsGoTypeNames pins the
// fall-through path: even when no hint matches and no pattern
// rewrites, R4 sanitization MUST strip Go type names.
func TestRemapStrictDecodeError_SanitizeStripsGoTypeNames(t *testing.T) {
	// Generic Go decode error that doesn't match unknown-field or
	// cannot-unmarshal-string patterns — sanitize fallback applies.
	original := errors.New(
		`some custom error mentioning emitAnswerDocumentV2Params and tool.emitAnswerBlockV2 in prose`)
	got := RemapStrictDecodeError(original, nil)
	msg := got.Error()
	for _, banned := range []string{
		"emitAnswerDocumentV2Params",
		"tool.emitAnswerBlockV2",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("R4 leak — Go-internal token %q present after sanitize: %s", banned, msg)
		}
	}
	if !strings.Contains(msg, "the schema's internal payload type") {
		t.Errorf("sanitize MUST replace Go names with neutral marker; got %s", msg)
	}
}

// TestRemapStrictDecodeError_CleanErrorPassesThrough pins the
// no-op path: an error with no Go type names and no rewrite
// pattern returns unchanged.
func TestRemapStrictDecodeError_CleanErrorPassesThrough(t *testing.T) {
	original := errors.New("plain LLM-friendly error")
	got := RemapStrictDecodeError(original, nil)
	if got != original {
		t.Errorf("clean err MUST return verbatim; got %v", got)
	}
}
