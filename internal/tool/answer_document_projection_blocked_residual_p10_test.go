package tool

// answer_document_projection_blocked_residual_p10_test.go — CR-3 件② P10
// display pins (§29.42 P10, 2026-07-12; 冷读案7 GPU-fence witness): a
// D-family root-cause row that consumed NO blocked_reason caller while the
// window holds markers for its thread discloses the unconsumed residual on
// its identity face, and the unresolved next-step headline appends the
// 「但窗内存在 N 条 blocked_reason 记录」 clause. Rows that consumed their
// caller (CAL-1 内核调用点) stay untouched.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBlockedReasonResidualDisclosesOnTreeRow — the unconsumed residual
// rides 行2 where the 内核调用点 chip would sit.
func TestBlockedReasonResidualDisclosesOnTreeRow(t *testing.T) {
	fence := dstateRefineFence(t, "d_state_or_io_wait",
		"blocked_reason_window_count=12", "blocked_reason_window_caller=gpu_fence_wait")
	// Wrap-insensitive membership: the fence may legitimately fold the chip
	// across continuation lines at width.
	flat := strings.NewReplacer("\n", "", " ", "").Replace(fence)
	if !strings.Contains(flat, "窗内存在12条blocked_reason记录(caller=gpu_fence_wait,未核销)") {
		t.Fatalf("the unconsumed residual must disclose on the row identity face:\n%s", fence)
	}
	if strings.Contains(fence, "内核调用点") {
		t.Fatalf("no unanimous caller → no 内核调用点 chip:\n%s", fence)
	}
}

// TestBlockedReasonResidualNeverBesideConsumedCaller — CAL-1-consumed rows
// are out of scope (件② 工单原文: 显示侧已消费 caller 的行不涉).
func TestBlockedReasonResidualNeverBesideConsumedCaller(t *testing.T) {
	fence := dstateRefineFence(t, "d_state_or_io_wait",
		"dstate_all_noniowait=true", "blocked_reason_caller=dma_fence_default_wait")
	if strings.Contains(fence, "未核销") || strings.Contains(fence, "blocked_reason 记录") {
		t.Fatalf("a consumed-caller row must not carry the residual:\n%s", fence)
	}
}

// TestBlockedReasonResidualOnUndrilledHeadline — the unresolved headline
// appends the residual clause (ledger wording: 该行标未解析,但窗内存在 N 条
// blocked_reason 记录(caller=…)).
func TestBlockedReasonResidualOnUndrilledHeadline(t *testing.T) {
	lead := types.TraceCausalProjectionNode{
		Subject:                   "CompThread_0-2955",
		Object:                    "d_state_or_io_wait",
		Rank:                      1,
		BlockedReasonWindowCount:  12,
		BlockedReasonWindowCaller: "gpu_fence_wait",
	}
	zhRow := runtimeTraceNextStepUndrilledHeadlineText(lead, "", true, true)
	if !strings.Contains(zhRow, "尚无已核实的上游因果,但窗内存在 12 条 blocked_reason 记录(caller=gpu_fence_wait,未核销)") {
		t.Fatalf("zh headline must append the residual clause: %q", zhRow)
	}
	enRow := runtimeTraceNextStepUndrilledHeadlineText(lead, "", true, false)
	if !strings.Contains(enRow, "yet the window holds 12 blocked_reason record(s) (caller=gpu_fence_wait, unconsumed)") {
		t.Fatalf("en headline must append the residual clause: %q", enRow)
	}
	// Controls: zero-count and non-unresolved rows keep the original text.
	clean := types.TraceCausalProjectionNode{Subject: "CompThread_0-2955", Object: "d_state_or_io_wait", Rank: 1}
	if row := runtimeTraceNextStepUndrilledHeadlineText(clean, "", true, true); strings.Contains(row, "blocked_reason") {
		t.Fatalf("zero-count row must not fabricate a residual: %q", row)
	}
	if row := runtimeTraceNextStepUndrilledHeadlineText(lead, "", false, true); strings.Contains(row, "未核销") {
		t.Fatalf("the missing-wakeup arm never appends the depth-residual clause: %q", row)
	}
}
