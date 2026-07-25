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
