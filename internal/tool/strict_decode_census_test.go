package tool

// EMITBURN-2 (NG-1, §13.4, 2026-07-25) — the strict decoder burned ONE
// unknown field per round (no path, no count, no roster): the fourth replay
// spent 2m51s on three rejects for the SAME fabricated field living in two
// containers and multiple array items. One reject must now enumerate every
// unknown key with its JSON path. Report-layer only: the decode verdict and
// the single-field message stay byte-identical.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type censusTestTarget struct {
	Kind string `json:"kind,omitempty"`
	PID  int    `json:"pid,omitempty"`
}

type censusTestLine struct {
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

type censusTestParams struct {
	Intent  string             `json:"intent,omitempty"`
	Targets []censusTestTarget `json:"runtime_targets,omitempty"`
	Lines   []censusTestLine   `json:"referenced_artifact_lines,omitempty"`
}

func TestStrictDecodeRejectEnumeratesEveryUnknownField(t *testing.T) {
	// no_touying 客户形:同名臆造字段落在两容器+多 array item,修前每轮
	// reject 只报首个。
	payload := json.RawMessage(`{
		"intent": "root_cause",
		"runtime_targets": [{"kind": "thread", "pid": 1, "description": "a"}],
		"referenced_artifact_lines": [
			{"path": "x", "line": 1, "description": "b"},
			{"path": "y", "line": 2},
			{"path": "z", "line": 3, "description": "c", "reason": "d"}
		]
	}`)
	var dst censusTestParams
	_, res, err := decodeStrictNormalizedToolParams("emit_analysis", payload, &dst, nil)
	if res == nil || err == nil {
		t.Fatal("unknown fields must still reject")
	}
	for _, want := range []string{
		"runtime_targets[0].description",
		"referenced_artifact_lines[0].description",
		"referenced_artifact_lines[2].description",
		"referenced_artifact_lines[2].reason",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("reject roster missing %q:\n%s", want, res.Summary)
		}
	}
	if !strings.Contains(err.Error(), "referenced_artifact_lines[2].reason") {
		t.Fatalf("returned error must carry the roster too: %v", err)
	}
}

func TestStrictDecodeSingleUnknownFieldMessageUnchanged(t *testing.T) {
	// 单 unknown 形保持既有消息字节(爆炸半径最小):census 注只在 >1 时追加。
	payload := json.RawMessage(`{"intent":"x","runtime_targets":[{"kind":"thread","description":"a"}]}`)
	var dst censusTestParams
	_, res, err := decodeStrictNormalizedToolParams("emit_analysis", payload, &dst, nil)
	if res == nil || err == nil {
		t.Fatal("unknown field must still reject")
	}
	if strings.Contains(res.Summary, "all unknown fields") {
		t.Fatalf("single-field reject must stay byte-identical: %s", res.Summary)
	}
}

// TestStrictDecodeHintMatchedCensusDropsRemoveImperative — R6-1
// (round-6 sweep, eval/sweep_round6_findings_20260730.md): when a
// MisplacedFieldHint matched the rejected field, the message already
// carries THE imperative for it (rename / relocate). The census note's
// blanket "remove every one of them" contradicted it on the exact lane
// EVALFIX-2A was built to fix — one imperative per failure. With the
// hinted field as the only unknown, the census adds nothing and stays
// silent. Red-first: pre-fix the Summary carried both "rename the key"
// and "remove every one of them".
func TestStrictDecodeHintMatchedCensusDropsRemoveImperative(t *testing.T) {
	payload := json.RawMessage(`{"requested_files":["a.go"]}`)
	var dst emitAnalysisParams
	_, res, err := decodeStrictNormalizedToolParams("emit_analysis", payload, &dst, emitAnalysisMisplacedHints)
	if res == nil || err == nil {
		t.Fatal("unknown field must still reject")
	}
	if !strings.Contains(res.Summary, "rename the key and keep the value unchanged") {
		t.Fatalf("top-level near-miss must keep the rename teaching:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "remove every one of them") {
		t.Fatalf("hint-matched reject must NOT also order removal (contradictory imperatives):\n%s", res.Summary)
	}
	if strings.Contains(err.Error(), "remove every one of them") {
		t.Fatalf("returned error must not carry the removal imperative either: %v", err)
	}
}

// TestStrictDecodeHintMatchedCensusKeepsOtherUnknowns — R6-1 second arm:
// a hint match must not hide OTHER unknown fields (EMITBURN-2 roster
// value survives), but the hinted field leaves the roster (its
// instruction is above) and the remainder wording defers instead of
// contradicting.
func TestStrictDecodeHintMatchedCensusKeepsOtherUnknowns(t *testing.T) {
	payload := json.RawMessage(`{"requested_files":["a.go"],"bogus_field_name":1}`)
	var dst emitAnalysisParams
	_, res, err := decodeStrictNormalizedToolParams("emit_analysis", payload, &dst, emitAnalysisMisplacedHints)
	if res == nil || err == nil {
		t.Fatal("unknown fields must still reject")
	}
	if !strings.Contains(res.Summary, "rename the key and keep the value unchanged") {
		t.Fatalf("rename teaching must survive:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "remove every one of them") {
		t.Fatalf("hint-matched reject must NOT carry the removal imperative:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "bogus_field_name") {
		t.Fatalf("the other unknown field must stay on the roster:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "requested_files (did you mean") {
		t.Fatalf("the hinted field must leave the roster (its instruction is the rename above):\n%s", res.Summary)
	}
}

// TestStrictDecodeNestedWrongNameGetsCensusDidYouMean — R6-2 (round-6
// sweep): a NESTED occurrence of a CanonicalName hint's wrong spelling
// (source_inventory_profile.requested_files, the requested_fields
// near-miss) must NOT be taught the top-level rename — Go's
// unknown-field error carries no path, so only TOP-LEVEL membership in
// the raw payload licenses the rename row. Nested occurrences fall
// through to the census did-you-mean against their own container.
// Red-first: pre-fix the Summary taught 'rename the key to
// "required_files"' — wrong in both container and intent.
func TestStrictDecodeNestedWrongNameGetsCensusDidYouMean(t *testing.T) {
	payload := json.RawMessage(`{"source_inventory_profile":{"is_source_inventory":true,"requested_files":["name","location"]}}`)
	var dst emitAnalysisParams
	_, res, err := decodeStrictNormalizedToolParams("emit_analysis", payload, &dst, emitAnalysisMisplacedHints)
	if res == nil || err == nil {
		t.Fatal("nested unknown field must still reject")
	}
	if strings.Contains(res.Summary, "rename the key and keep the value unchanged") ||
		strings.Contains(res.Summary, `the field is named "required_files"`) {
		t.Fatalf("nested occurrence must NOT get the top-level rename teaching:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, `source_inventory_profile.requested_files (did you mean "requested_fields"?)`) {
		t.Fatalf("nested near-miss must fall through to the census did-you-mean:\n%s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == "tool_param_misnamed_field" {
		t.Fatalf("nested occurrence must not mint the rename repair: %+v", res.Repair)
	}
}

func TestStrictDecodeCensusWalksTypedTree(t *testing.T) {
	payload := json.RawMessage(`{"bogus_top": 1, "runtime_targets": [{"pid": 2}, {"nested_bogus": true}]}`)
	var dst censusTestParams
	census := strictDecodeUnknownFieldCensus(payload, &dst)
	if len(census) != 2 || census[0] != "bogus_top" || census[1] != "runtime_targets[1].nested_bogus" {
		t.Fatalf("census = %v", census)
	}
	// json.RawMessage / 非法 JSON fail-open 为空。
	if got := strictDecodeUnknownFieldCensus(json.RawMessage(`{"intent":`), &dst); got != nil {
		t.Fatalf("malformed payload must fail open: %v", got)
	}
	_ = time.Now
}
