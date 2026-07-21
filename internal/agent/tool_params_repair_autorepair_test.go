package agent

// AUTOREPAIR-1 (§29.175 用户裁定① — system auto-repair of schema/JSON
// failures) pins for the transport layer:
//
//   件1 T1-NEW-BRACE — repair pattern 5, the dropped `}` between adjacent
//     native array elements, replayed from the customer witness
//     (/Users/han/opt/customlogs/emit_investigation_complete_log.txt:272,
//     copied verbatim into testdata/autorepair1_missing_brace_witness.json).
//     Three-arm shape per the matrix PIN-DISCIPLINE clause: positive /
//     one-bit mutation (must NOT fire, legacy reject byte-compatible) /
//     fixed point (repair twice == repair once).
//
//   件5 — bounded, redacted malformed-params DEBUG evidence (RUN2FIX-B 件5
//     delegation point): bounded-prefix arm + sensitive-value arm.
//
//   件4 — structural call-order pins: repair runs before the malformed
//     reject is minted, and pattern 5 sits after the four legacy patterns.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
)

func loadMissingBraceWitness(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("testdata/autorepair1_missing_brace_witness.json")
	if err != nil {
		t.Fatalf("witness fixture missing: %v", err)
	}
	if len(raw) != 3994 {
		t.Fatalf("witness fixture must be the verbatim 3994-byte log payload, got %d bytes", len(raw))
	}
	if json.Valid(raw) {
		t.Fatalf("witness fixture must be the malformed original")
	}
	return raw
}

// Positive arm: the exact customer payload repairs with ONE structural `}`
// insertion at the s1/obs1 element boundary, every content byte preserved,
// and the repaired bytes strict-decode into the three authored blocks.
func TestRepairToolParamsJSON_MissingBraceWitnessPositive(t *testing.T) {
	raw := loadMissingBraceWitness(t)
	boundary := []byte(`",{"claim_uses"`)
	if n := bytes.Count(raw, boundary); n != 1 {
		t.Fatalf("witness must contain the defect boundary exactly once, got %d", n)
	}
	idx := bytes.Index(raw, boundary)

	repaired, ok := repairToolCallParamsJSON("emit_answer_document", raw)
	if !ok {
		t.Fatalf("pattern 5 must repair the witness payload")
	}
	// Exactly one `}` inserted before the element-boundary comma; all other
	// bytes verbatim (Tier1: content bytes untouched).
	want := append([]byte(nil), raw[:idx+1]...)
	want = append(want, '}')
	want = append(want, raw[idx+1:]...)
	if !bytes.Equal(repaired, want) {
		t.Fatalf("repair must insert exactly one `}` at the element boundary and change nothing else")
	}
	// Full strict re-parse: the repaired payload must decode into the three
	// authored blocks with zero unknown-field tolerance at this shape level.
	var doc struct {
		Blocks []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"blocks"`
		Citations []json.RawMessage `json:"citations"`
	}
	dec := json.NewDecoder(bytes.NewReader(repaired))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("repaired witness must decode: %v", err)
	}
	if len(doc.Blocks) != 3 || doc.Blocks[0].ID != "s1" || doc.Blocks[1].ID != "obs1" || doc.Blocks[2].ID != "cv1" {
		t.Fatalf("repaired witness must carry the three authored blocks s1/obs1/cv1, got %+v", doc.Blocks)
	}
}

// Fixed-point arm: repairing the repaired bytes is a no-op (valid input
// takes the fast path).
func TestRepairToolParamsJSON_MissingBraceWitnessFixedPoint(t *testing.T) {
	raw := loadMissingBraceWitness(t)
	repaired, ok := repairToolCallParamsJSON("emit_answer_document", raw)
	if !ok {
		t.Fatalf("pattern 5 must repair the witness payload")
	}
	again, ok := repairToolParamsJSON(repaired)
	if ok {
		t.Fatalf("repaired payload must be a fixed point (no second repair)")
	}
	if !bytes.Equal(again, repaired) {
		t.Fatalf("fixed point must return the payload unchanged")
	}
}

// Mutation arm (matrix T1-NEW-BRACE pin plan, arm 1): flip the one-bit
// condition — `:` instead of `,` before the `{` (a nested-value position).
// The repair must NOT fire and the legacy malformed reject must stay
// byte-identical to the pre-AUTOREPAIR copy.
func TestRepairToolParamsJSON_MissingBraceWitnessCommaMutationDoesNotFire(t *testing.T) {
	raw := loadMissingBraceWitness(t)
	mutated := bytes.Replace(raw, []byte(`",{"claim_uses"`), []byte(`":{"claim_uses"`), 1)
	if bytes.Equal(mutated, raw) {
		t.Fatalf("mutation must change the boundary byte")
	}
	repaired, ok := repairToolCallParamsJSON("emit_answer_document", mutated)
	if ok {
		t.Fatalf("repair must NOT fire on the nested-value mutation")
	}
	if !bytes.Equal(repaired, mutated) {
		t.Fatalf("non-firing repair must return the original bytes unchanged")
	}
	res := malformedToolParamsResult(llm.ToolCall{Name: "emit_answer_document", ID: "mut1", Params: mutated})
	const legacySummary = "invalid params: malformed JSON tool arguments for emit_answer_document " +
		"(invalid character ':' after object key:value pair). " +
		"Re-emit this tool call with a single native JSON object in arguments; do not wrap it as a string and do not emit partial JSON. " +
		"Original argument bytes were not executed."
	if res.Summary != legacySummary {
		t.Fatalf("legacy reject must stay byte-identical.\n got: %s\nwant: %s", res.Summary, legacySummary)
	}
}

// Trigger precision: `,{` at an object element whose parent is an OBJECT
// (not an array) must not fire — the alternative reading would require
// fabricating a key name (Tier3-forbidden).
func TestRepairToolParamsJSON_MissingBraceObjectParentDoesNotFire(t *testing.T) {
	raw := json.RawMessage(`{"a":{"x":1},{"y":2}}`)
	repaired, ok := repairToolParamsJSON(raw)
	if ok {
		t.Fatalf("object-parent `,{` must not repair, got: %s", repaired)
	}
	if !bytes.Equal(repaired, raw) {
		t.Fatalf("non-firing repair must return the original bytes unchanged")
	}
}

// Iteration behavior: multiple dropped element closers repair in one call
// (each pass re-triggers the identical error class at a later offset), and
// the bound of 8 abandons back to the original bytes.
func TestRepairToolParamsJSON_MissingBraceIterationAndBound(t *testing.T) {
	two := json.RawMessage(`[{"a":1,{"b":2,{"c":3}]`)
	repaired, ok := repairToolParamsJSON(two)
	if !ok {
		t.Fatalf("two dropped closers must repair")
	}
	if string(repaired) != `[{"a":1},{"b":2},{"c":3}]` {
		t.Fatalf("unexpected repair result: %s", repaired)
	}

	overflow := json.RawMessage("[" + strings.Repeat(`{"x":1,`, maxMissingObjectCloseRepairs+1) + `{"x":1}` + "]")
	kept, ok := repairToolParamsJSON(overflow)
	if ok {
		t.Fatalf("more than %d dropped closers must abandon", maxMissingObjectCloseRepairs)
	}
	if !bytes.Equal(kept, overflow) {
		t.Fatalf("abandoned repair must return the original bytes unchanged")
	}

	atBound := json.RawMessage("[" + strings.Repeat(`{"x":1,`, maxMissingObjectCloseRepairs) + `{"x":1}` + "]")
	if _, ok := repairToolParamsJSON(atBound); !ok {
		t.Fatalf("exactly %d dropped closers must still repair", maxMissingObjectCloseRepairs)
	}
}

// 件5 bounded-prefix arm: evidence is bounded regardless of payload size,
// and carries the parse-error offset with its context window.
func TestMalformedToolParamsEvidence_Bounded(t *testing.T) {
	big := []byte(`{"pattern":"` + strings.Repeat("a", 6000) + `",{`)
	prefix, errOffset, offsetContext := malformedToolParamsEvidence(big)
	if len(prefix) > malformedParamsEvidencePrefixLimit {
		t.Fatalf("prefix must be bounded to %d bytes, got %d", malformedParamsEvidencePrefixLimit, len(prefix))
	}
	if errOffset < 0 {
		t.Fatalf("syntax failure must carry its error offset")
	}
	if len(offsetContext) > 2*malformedParamsEvidenceContextRadius {
		t.Fatalf("offset context must be bounded to %d bytes, got %d", 2*malformedParamsEvidenceContextRadius, len(offsetContext))
	}
	if offsetContext == "" {
		t.Fatalf("offset context must cover the error neighbourhood")
	}
}

// 件5 redaction arm: token-shaped field values never reach the evidence,
// while ordinary field values are preserved for fixture reconstruction.
func TestMalformedToolParamsEvidence_RedactsTokenShapedValues(t *testing.T) {
	raw := []byte(`{"api_token":"SECRETVALUE123","access_token": "SECRETVALUE456","pattern":"keepme",{`)
	prefix, _, offsetContext := malformedToolParamsEvidence(raw)
	for _, secret := range []string{"SECRETVALUE123", "SECRETVALUE456"} {
		if strings.Contains(prefix, secret) || strings.Contains(offsetContext, secret) {
			t.Fatalf("sensitive value %q must not appear in evidence:\nprefix=%s\ncontext=%s", secret, prefix, offsetContext)
		}
	}
	if !strings.Contains(prefix, "[redacted]") {
		t.Fatalf("redaction marker must replace sensitive values, got: %s", prefix)
	}
	if !strings.Contains(prefix, "keepme") {
		t.Fatalf("non-sensitive values must be preserved for fixture reconstruction, got: %s", prefix)
	}
}

// 件4 structural call-order pin (agent side): the transport repair runs
// BEFORE the malformed-params reject is minted, and pattern 5 sits after the
// four legacy patterns so it can never pre-empt them.
func TestAutoRepairAgentCallOrderStructuralPin(t *testing.T) {
	agentSrc, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatalf("read agent.go: %v", err)
	}
	repairIdx := bytes.Index(agentSrc, []byte("repairToolCallParamsJSON(tc.Name, tc.Params)"))
	rejectIdx := bytes.Index(agentSrc, []byte("return malformedToolParamsResult(tc), nil"))
	if repairIdx < 0 || rejectIdx < 0 {
		t.Fatalf("expected call sites not found (repair=%d reject=%d)", repairIdx, rejectIdx)
	}
	if repairIdx >= rejectIdx {
		t.Fatalf("transport repair must run before the malformed reject is minted (repair=%d reject=%d)", repairIdx, rejectIdx)
	}

	repairSrc, err := os.ReadFile("tool_params_repair.go")
	if err != nil {
		t.Fatalf("read tool_params_repair.go: %v", err)
	}
	// Tier1 disclosure is log-only; the WARN format is matrix-specified
	// (T1-NEW-BRACE) — pin the exact wording.
	if !bytes.Contains(repairSrc, []byte(`"[agent] repaired dropped object-close between array elements tool=%s offset=%d count=%d"`)) {
		t.Fatalf("pattern-5 WARN disclosure format drifted from the matrix wording")
	}
	body := extractGoFuncBody(t, string(repairSrc), "func repairNamedToolParamsJSON(")
	order := []string{
		"tryTrimLeadingGarbage(",
		"tryTrimTrailingGarbage(",
		"tryRemoveTrailingComma(",
		"tryCompleteTruncatedJSON(",
		"tryInsertMissingObjectClose(",
	}
	last := -1
	for _, name := range order {
		idx := strings.Index(body, name)
		if idx < 0 {
			t.Fatalf("repair pattern %q not found in repairNamedToolParamsJSON", name)
		}
		if idx <= last {
			t.Fatalf("repair pattern %q out of order", name)
		}
		last = idx
	}
}

// extractGoFuncBody slices from the function declaration to the next
// top-level declaration — a deliberately dumb structural probe.
func extractGoFuncBody(t *testing.T, src, decl string) string {
	t.Helper()
	start := strings.Index(src, decl)
	if start < 0 {
		t.Fatalf("declaration %q not found", decl)
	}
	rest := src[start+len(decl):]
	end := strings.Index(rest, "\nfunc ")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
