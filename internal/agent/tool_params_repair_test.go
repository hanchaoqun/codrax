package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRepairToolParamsJSON_S5aProductionRegression pins the exact
// LLM-corrupted shape that motivated Fix G — a customer-reported
// REPL status-line case where the analyzer LLM emitted a grep call
// with a trailing extra `}` (the model closing its outer thought at
// the same indent as the params object), causing strict
// json.Unmarshal to fail with "invalid character '}' after
// top-level value".
func TestRepairToolParamsJSON_S5aProductionRegression(t *testing.T) {
	raw := json.RawMessage(`{
"files_only": false,
"include": "*.go",
"pattern": "fmt\\.Fprintf.*\\n|Fprintf.*status|String.*status"
}
}`)
	repaired, ok := repairToolParamsJSON(raw)
	if !ok {
		t.Fatalf("expected repair to apply on trailing `}`; got no-op\n  input: %s", raw)
	}
	var probe struct {
		FilesOnly bool   `json:"files_only"`
		Include   string `json:"include"`
		Pattern   string `json:"pattern"`
	}
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("repaired payload must parse strictly; got %v\n  repaired: %s", err, repaired)
	}
	if probe.Include != "*.go" {
		t.Errorf("repaired include = %q, want *.go", probe.Include)
	}
}

// TestRepairToolParamsJSON_AlreadyValid confirms valid input is
// returned unchanged with ok=false (no repair applied).
func TestRepairToolParamsJSON_AlreadyValid(t *testing.T) {
	raw := json.RawMessage(`{"files_only":true,"pattern":"foo"}`)
	repaired, ok := repairToolParamsJSON(raw)
	if ok {
		t.Errorf("valid input should return ok=false; got repaired=%s", repaired)
	}
	if string(repaired) != string(raw) {
		t.Errorf("valid input should return unchanged; got %s, want %s", repaired, raw)
	}
}

// TestRepairToolParamsJSON_TrailingComma pins the streaming-tokeniser
// artefact pattern — `,` immediately before `}`.
func TestRepairToolParamsJSON_TrailingComma(t *testing.T) {
	raw := json.RawMessage(`{"files_only":true,"pattern":"foo",}`)
	repaired, ok := repairToolParamsJSON(raw)
	if !ok {
		t.Fatalf("trailing comma should be repaired; got no-op")
	}
	var probe map[string]interface{}
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("repaired payload must parse; got %v", err)
	}
	if probe["pattern"] != "foo" {
		t.Errorf("repaired pattern = %v, want foo", probe["pattern"])
	}
}

// TestRepairToolParamsJSON_TrailingCommaInArray pins the array-form
// of trailing comma — `,` before `]`.
func TestRepairToolParamsJSON_TrailingCommaInArray(t *testing.T) {
	raw := json.RawMessage(`{"includes":["a","b","c",]}`)
	repaired, ok := repairToolParamsJSON(raw)
	if !ok {
		t.Fatalf("trailing comma in array should be repaired; got no-op")
	}
	var probe struct {
		Includes []string `json:"includes"`
	}
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("repaired payload must parse; got %v", err)
	}
	if len(probe.Includes) != 3 {
		t.Errorf("repaired includes len = %d, want 3", len(probe.Includes))
	}
}

// TestRepairToolParamsJSON_StringContainsBraceStaysIntact confirms
// the byte walker is string-aware: a `}` inside a quoted string is
// NOT counted as trailing garbage.
func TestRepairToolParamsJSON_StringContainsBraceStaysIntact(t *testing.T) {
	// Pattern contains a literal `}` and then valid trailing — fully valid.
	raw := json.RawMessage(`{"pattern":"foo}bar","files_only":true}`)
	repaired, ok := repairToolParamsJSON(raw)
	if ok {
		t.Errorf("string-internal `}` must not trigger repair; got repaired=%s", repaired)
	}
	if string(repaired) != string(raw) {
		t.Errorf("input with string-internal `}` should pass through unchanged")
	}
}

// TestRepairToolParamsJSON_StringContainsCommaStaysIntact confirms
// a `,` inside a quoted string before what looks like `}` is NOT
// removed. Only structural commas are subject to the trailing-comma
// repair.
func TestRepairToolParamsJSON_StringContainsCommaStaysIntact(t *testing.T) {
	raw := json.RawMessage(`{"pattern":"foo,}bar"}`)
	repaired, ok := repairToolParamsJSON(raw)
	if ok {
		t.Errorf("string-internal `,}` must not trigger repair; got repaired=%s", repaired)
	}
	var probe struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("untouched payload must still parse: %v", err)
	}
	if probe.Pattern != "foo,}bar" {
		t.Errorf("string content corrupted; pattern = %q, want %q", probe.Pattern, "foo,}bar")
	}
}

// TestRepairToolParamsJSON_TruncatedInvalid bails on truly broken
// JSON that no bounded repair can fix (e.g. unterminated string).
func TestRepairToolParamsJSON_TruncatedInvalid(t *testing.T) {
	raw := json.RawMessage(`{"pattern":"unclosed`)
	repaired, ok := repairToolParamsJSON(raw)
	if ok {
		t.Errorf("unrepairable truncation should return ok=false; got %s", repaired)
	}
}

// TestRepairToolParamsJSON_DoubleObjectBails refuses to repair when
// the trailing portion contains a SECOND top-level value (would be
// silently discarding LLM intent).
func TestRepairToolParamsJSON_DoubleObjectBails(t *testing.T) {
	// A complete object followed by a string value → trailing `"` is
	// non-safe → bail.
	raw := json.RawMessage(`{"a":1}"second"`)
	repaired, ok := repairToolParamsJSON(raw)
	if ok {
		t.Errorf("trailing non-garbage value should NOT trigger repair; got %s", repaired)
	}
}

// TestRepairToolParamsJSON_EscapedQuotesInString covers the escape
// state machine: a `\"` inside a string must not toggle the
// in-string flag.
func TestRepairToolParamsJSON_EscapedQuotesInString(t *testing.T) {
	raw := json.RawMessage(`{"pattern":"foo\"bar","files_only":true,}`)
	repaired, ok := repairToolParamsJSON(raw)
	if !ok {
		t.Fatalf("trailing comma after escaped-quote string should still repair")
	}
	var probe struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("repaired must parse: %v", err)
	}
	if probe.Pattern != `foo"bar` {
		t.Errorf("escaped-quote content corrupted; pattern = %q", probe.Pattern)
	}
}

// TestRepairToolParamsJSON_NestedObjectsTrailingBrace covers a
// complex nested shape with a single trailing `}`.
func TestRepairToolParamsJSON_NestedObjectsTrailingBrace(t *testing.T) {
	raw := json.RawMessage(`{"params":{"a":{"b":[1,2]}}}}`)
	repaired, ok := repairToolParamsJSON(raw)
	if !ok {
		t.Fatalf("nested-object trailing brace should be repaired")
	}
	if !strings.HasSuffix(string(repaired), `}}}`) {
		t.Errorf("repaired must end with the correct number of closing braces; got %s", repaired)
	}
}

// TestRepairToolParamsJSON_MultipleTrailingArtifacts handles the
// combined case: multiple trailing closing braces + commas.
func TestRepairToolParamsJSON_MultipleTrailingArtifacts(t *testing.T) {
	raw := json.RawMessage(`{"a":1}}}}`)
	repaired, ok := repairToolParamsJSON(raw)
	if !ok {
		t.Fatalf("multiple trailing braces should be repaired")
	}
	if string(repaired) != `{"a":1}` {
		t.Errorf("repaired = %q, want %q", repaired, `{"a":1}`)
	}
}

// TestRepairToolParamsJSON_EmptyInput returns ok=false on empty
// input (nothing to repair).
func TestRepairToolParamsJSON_EmptyInput(t *testing.T) {
	repaired, ok := repairToolParamsJSON(nil)
	if ok {
		t.Errorf("empty input should return ok=false; got repaired=%s", repaired)
	}
}
