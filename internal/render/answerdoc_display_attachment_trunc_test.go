package render

// TRUNC 批 (P1, §29.10-1, 2026-07-09) — huadong_792 witness pins.
//
// 客户实锤:cust_trace_huadong_792.txt 行 926 以裸 "..." 截断,丢失 =
// 第一稿保留段中部起 + 末尾系统补充整块。保留段可见体恰 15997–15999 rune
// = 旧 maxAnswerDisplayBodyRunes(16000) 裸帽逐字节吻合。
//
// 修复原则(任务裁定):客户面答案永不静默截断——
//   (1) 帽值必须远大于任何真实答案形(huadong 全稿量级 ≈2-4 万 rune);
//   (2) 确需截断时尾部显式披露字符总数与已显示数,禁止裸 "...";
//   (3) 正常短答案零变化。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const truncTailSentinel = "尾部完整性哨兵TRUNCTAIL系统补充核对完毕"

func truncTestDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "主体答案。",
		}},
	}
}

// truncBodyOfRunes builds a huadong-shaped preserved-draft body of at least
// n runes whose LAST line is the unique tail sentinel — the assertion target
// for "the tail must survive rendering".
func truncBodyOfRunes(n int) string {
	line := "根因说明:线程优先级反转候选,供给折算缺口按大核满频折算,下界,簇结构不可判,按纯频率比折算,独立口径不计入有效归因。\n"
	var b strings.Builder
	for b.Len() < n*4 { // CJK ≈3 bytes/rune; overshoot then check runes
		b.WriteString(line)
		if len([]rune(b.String())) >= n {
			break
		}
	}
	b.WriteString(truncTailSentinel)
	return b.String()
}

// TestTRUNCHuadongScaleAttachmentRendersFullBody: huadong 量级(4 万 rune,
// 高于旧 16000 帽)保留段必须完整产出——尾哨兵在,且不得出现 witness 截形
// (裸 "..." 行 + 面板收尾 "---")。修前红:旧帽在 16000 rune 处裸切。
func TestTRUNCHuadongScaleAttachmentRendersFullBody(t *testing.T) {
	body := truncBodyOfRunes(40000)
	out := RenderAnswerDocumentWithAttachments(truncTestDoc(), []types.AnswerDisplayAttachment{{
		Kind:  types.AnswerDisplayAttachmentMarkdown,
		Title: "第一稿答案（校验前参考）",
		Body:  body,
	}}, "zh")
	if !strings.Contains(out, truncTailSentinel) {
		t.Fatalf("huadong-scale preserved draft lost its tail (witness 截形): rendered %d bytes, want tail sentinel present", len(out))
	}
	if strings.Contains(out, "\n...\n") {
		t.Fatalf("bare \"...\" truncation marker resurfaced in rendered answer")
	}
	if !strings.Contains(out, "主体答案。") || !strings.Contains(out, "系统保留内容") {
		t.Fatalf("structured body / preserved panel frame missing:\n%.400s", out)
	}
}

// TestTRUNCOverCapAttachmentDisclosesExplicitly: 超帽形(> maxAnswerDisplayBodyRunes)
// 若仍需截断,必须显式披露字符总数与已显示数——禁止 witness 的裸 "..."。
func TestTRUNCOverCapAttachmentDisclosesExplicitly(t *testing.T) {
	body := truncBodyOfRunes(maxAnswerDisplayBodyRunes + 500)
	total := len([]rune(strings.TrimSpace(body)))
	out := RenderAnswerDocumentWithAttachments(truncTestDoc(), []types.AnswerDisplayAttachment{{
		Kind:  types.AnswerDisplayAttachmentMarkdown,
		Title: "第一稿答案（校验前参考）",
		Body:  body,
	}}, "zh")
	if strings.Contains(out, "\n...\n") {
		t.Fatalf("over-cap preserved draft still truncates with a bare \"...\" (witness shape)")
	}
	wantDisclosure := fmt.Sprintf("原文共 %d 字符", total)
	if !strings.Contains(out, wantDisclosure) || !strings.Contains(out, fmt.Sprintf("前 %d 字符", maxAnswerDisplayBodyRunes)) {
		t.Fatalf("over-cap truncation must disclose total+shown rune counts (want %q):\n...%s", wantDisclosure, tailOf(out, 600))
	}
	// The panel must still close cleanly after the disclosure.
	if !strings.Contains(out, "---") {
		t.Fatalf("preserved panel end separator missing")
	}

	outEN := RenderAnswerDocumentWithAttachments(truncTestDoc(), []types.AnswerDisplayAttachment{{
		Kind:  types.AnswerDisplayAttachmentMarkdown,
		Title: "First Draft Answer (Pre-review Reference)",
		Body:  body,
	}}, "en")
	if !strings.Contains(outEN, fmt.Sprintf("%d characters total", total)) ||
		!strings.Contains(outEN, fmt.Sprintf("first %d", maxAnswerDisplayBodyRunes)) {
		t.Fatalf("EN over-cap truncation must disclose total+shown rune counts:\n...%s", tailOf(outEN, 600))
	}
}

// TestTRUNCDisplayBodyCapRatchet: 突变 pin——帽偷偷调小必须咬红。
// 200000 rune 由 TRUNC 批裁定为"远大于任何真实答案形"的保护性上限
// (witness huadong 全稿 ≈2-4 万 rune;16000 旧帽正是事故根源之一)。
func TestTRUNCDisplayBodyCapRatchet(t *testing.T) {
	if maxAnswerDisplayBodyRunes < 200000 {
		t.Fatalf("maxAnswerDisplayBodyRunes ratchet violated: got %d, want >= 200000 (TRUNC §29.10-1: cap must dwarf any real answer shape)", maxAnswerDisplayBodyRunes)
	}
}

// TestTRUNCShortAttachmentUnchanged: 正常短答案零变化——附件段整段逐字节
// 相等 pin(P2-1 加固:Contains 级断言对尾空格等字节漂移不敏感,升级为
// exact + 长度双断言)。任何普通路径的字节漂移(尾空格/多余空行/标记
// 残留)都必须咬红。
func TestTRUNCShortAttachmentUnchanged(t *testing.T) {
	out := RenderAnswerDocumentWithAttachments(truncTestDoc(), []types.AnswerDisplayAttachment{{
		Kind:  types.AnswerDisplayAttachmentMarkdown,
		Title: "第一稿答案（校验前参考）",
		Body:  "短保留内容,一段即止。",
	}}, "zh")
	marker := "---\n\n> **系统保留内容**"
	idx := strings.Index(out, marker)
	if idx < 0 {
		t.Fatalf("preserved panel missing:\n%s", out)
	}
	segment := out[idx:]
	want := "---\n\n> **系统保留内容**\n>\n> 下面内容来自模型已生成但未能完整进入结构化答案的输出，系统按原文保留展示；这部分不是上方已校验结构化答案的主体，请按补充参考阅读。\n\n" +
		"#### 第一稿答案（校验前参考）\n\n" +
		"短保留内容,一段即止。\n\n" +
		"---\n"
	if len(segment) != len(want) {
		t.Fatalf("attachment segment length drifted: got %d bytes, want %d bytes\ngot: %q", len(segment), len(want), segment)
	}
	if segment != want {
		t.Fatalf("attachment segment drifted byte-wise:\ngot:  %q\nwant: %q", segment, want)
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
