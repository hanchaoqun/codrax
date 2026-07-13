package tool

// answer_document_principal_enum_backfill_template_test.go — 件2 backfill
// 借壳修 pins (复核 E1-F3, 2026-07-13; witness: the waker report's
// 「系统按已验证证据补充缺失成员」 table restated the model's claim-of-absence
// variant 「…均无独立的 sched_wakeup 终点」/「嵌入 s_sleep 16.419ms 区间」
// verbatim under the system's voice). The system supplement face emits only
// its own typed template: member value tokens + typed facts; the model's
// narrative sentence never rides it.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPrincipalEnumBackfill_RuntimeSystemLabelExtraction(t *testing.T) {
	for _, tc := range []struct {
		in       string
		want     string
		banned   []string
		required []string
	}{
		{
			// The E1-F3 witness row: values + typed tokens survive, the
			// narrative clauses (嵌入…区间 / 窗口：) never do.
			in:       "d_sleep 16.064ms @ cpu=3，lines=3158-21525，peer=unknown-thread，嵌入 s_sleep 16.419ms 区间（窗口：13762.881585-13762.898021）",
			banned:   []string{"嵌入", "区间", "窗口："},
			required: []string{"d_sleep 16.064ms @ cpu=3", "lines=3158-21525", "peer=unknown-thread", "s_sleep 16.419ms", "13762.881585-13762.898021"},
		},
		{
			// A chain row: the pure-ASCII chain passes whole; the trailing
			// narrative clause contributes only its value tokens.
			in:       "tppmgr-idle-0-270 → RSUniRenderThre-2188 → CompThread_0-2955 @ 13762.898004s，s_sleep 持续 16.419ms",
			banned:   []string{"持续"},
			required: []string{"tppmgr-idle-0-270 → RSUniRenderThre-2188 → CompThread_0-2955 @ 13762.898004s", "s_sleep 16.419ms"},
		},
		{
			// Pure ASCII members pass byte-identically.
			in:   "fscache_page_wait_o+0x110/0x250[sysmgr.elf] delay=100",
			want: "fscache_page_wait_o+0x110/0x250[sysmgr.elf] delay=100",
		},
		{
			// A pure narrative member yields the neutral locator pointer
			// (P2a rider 件5 zh-en 合审: the pointer names the actual
			// 定义位置/Location column and speaks the reader's language).
			in:   "均无独立的唤醒终点",
			want: "(见定义位置)",
		},
		{
			// 件A 权属模型终态 (复核 P2-2 根修): the ENGLISH claim-of-absence
			// probe sentence — the first cut's Han gate passed it WHOLE
			// (language smuggling). Only the typed tokens survive; the
			// narrative words never do, in any language.
			in:       "all d_sleep intervals are embedded in the upstream s_sleep interval, all without an independent sched_wakeup endpoint",
			banned:   []string{"without an independent", "embedded", "all ", "upstream", "endpoint"},
			required: []string{"d_sleep", "s_sleep", "sched_wakeup"},
		},
		{
			// An English narrative with zero typed tokens also falls to the
			// neutral pointer (no language privilege in either direction).
			in:   "these waits are entirely explained by the render pipeline",
			want: "(见定义位置)",
		},
	} {
		got := principalEnumerationRuntimeSystemLabel(tc.in, true)
		if tc.want != "" && got != tc.want {
			t.Fatalf("label(%q) = %q, want %q", tc.in, got, tc.want)
		}
		for _, banned := range tc.banned {
			if strings.Contains(got, banned) {
				t.Fatalf("narrative token %q must not survive the system template: %q → %q", banned, tc.in, got)
			}
		}
		for _, want := range tc.required {
			if !strings.Contains(got, want) {
				t.Fatalf("value token %q must survive the system template: %q → %q", want, tc.in, got)
			}
		}
	}
	// P2a rider 件5 (§29.57 残留 zh-en 合审, 2026-07-13): the EN face speaks
	// the EN pointer — the zh-only 「(见定位)」 literal on an EN report was the
	// audited inconsistency.
	if got := principalEnumerationRuntimeSystemLabel("均无独立的唤醒终点", false); got != "(see Location)" {
		t.Fatalf("EN locator pointer = %q, want %q", got, "(see Location)")
	}
}

// TestPrincipalEnumBackfill_SystemFaceNeverCopiesModelSentence — the
// negative pin on the block builder itself: a runtime-artifact supplement
// row's model sentence template never reaches the system-voiced block
// (values do); a source-code row keeps its member verbatim (untouched lane).
func TestPrincipalEnumBackfill_SystemFaceNeverCopiesModelSentence(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	set := types.EnumerationDisplaySet{
		ID:    "wakers",
		Label: "CompThread_0-2955 d_sleep 区间",
	}
	rows := []types.EnumerationDisplayRow{{
		RowID:           "r1",
		SetID:           "wakers",
		Member:          "d_sleep 16.064ms @ cpu=3，嵌入 s_sleep 16.419ms 区间，均无独立的 sched_wakeup 终点",
		DisplayLabel:    "d_sleep 16.064ms @ cpu=3，嵌入 s_sleep 16.419ms 区间，均无独立的 sched_wakeup 终点",
		EvidenceOrigins: []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginRuntimeArtifact},
	}, {
		RowID:           "r2",
		SetID:           "wakers",
		Member:          "普通代码成员（保持原样）",
		DisplayLabel:    "普通代码成员（保持原样）",
		EvidenceOrigins: []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginCurrentSource},
	}}
	block := buildPrincipalEnumerationRowsBlock(doc, set, rows, true, principalEnumerationSupplementMissing)
	var joined strings.Builder
	joined.WriteString(block.Title + "\n" + block.Text + "\n")
	for _, item := range block.Items {
		joined.WriteString(item.Label + "\n" + item.Text + "\n" + strings.Join(item.Cells, "|") + "\n")
	}
	face := joined.String()
	for _, banned := range []string{"嵌入", "均无独立"} {
		if strings.Contains(face, banned) {
			t.Fatalf("the system supplement face must never copy the model sentence template (%q):\n%s", banned, face)
		}
	}
	for _, want := range []string{"d_sleep 16.064ms @ cpu=3", "s_sleep 16.419ms", "sched_wakeup"} {
		if !strings.Contains(face, want) {
			t.Fatalf("member value tokens must survive on the system face (%q):\n%s", want, face)
		}
	}
	// The non-runtime row's member stays verbatim (this fix rewrites the
	// runtime lane only).
	if !strings.Contains(face, "普通代码成员（保持原样）") {
		t.Fatalf("source-code rows must keep their member verbatim:\n%s", face)
	}
}
