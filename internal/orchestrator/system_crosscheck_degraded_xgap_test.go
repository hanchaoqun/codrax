package orchestrator

// XGAP-FIX ⑤ pins (§29.104.8, witness 20260715-202022.323-89609): the four
// prose defenses (S3' cross-check appendix scans: PSG scalar
// re-derivation / lexicon board / wall-clock conservation / fact
// juxtaposition) must consume the degraded recovery-lane document instead
// of structurally skipping on AnswerDocumentV2()==nil.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCollectSystemCrossCheckFindings_ScansDegradedRecoveryDoc(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 主线程实际运行仅 20.372ms，runnable 46.364ms。")

	// The degraded lane: NO validated carrier, only the recovery snapshot.
	mut.SetDegradedRecoveredAnswerDocumentV2(doc)
	if mut.AnswerDocumentV2() != nil {
		t.Fatal("fixture broken: validated carrier must stay empty on the degraded lane")
	}

	o := &Orchestrator{busCtx: bus}
	findings := o.collectSystemCrossCheckFindings()
	var present bool
	for _, f := range findings {
		if strings.Contains(f, "状态时长之和") || strings.Contains(f, "超过该线程该维度已发布总量") {
			present = true
		}
	}
	if !present {
		t.Fatalf("degraded recovery draft must be scanned by the cross-check defenses, got %v", findings)
	}
}

func TestCollectSystemCrossCheckFindings_NoDocStillInert(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	o := &Orchestrator{busCtx: bus}
	if findings := o.collectSystemCrossCheckFindings(); len(findings) != 0 {
		t.Fatalf("no shipped document of any kind → no findings, got %v", findings)
	}
}

func TestCollectSystemCrossCheckFindings_ValidatedDocWinsOverStaleDegraded(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	// A stale degraded snapshot followed by a real validated emit: the
	// carrier semantics clear the snapshot, so the defenses scan the
	// validated document only.
	mut.SetDegradedRecoveredAnswerDocumentV2(psgProseDoc("com.baidu.tieba-59566 主线程实际运行仅 20.372ms，runnable 46.364ms。"))
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, psgProseDoc("窗口内无异常。"))

	o := &Orchestrator{busCtx: bus}
	for _, f := range o.collectSystemCrossCheckFindings() {
		if strings.Contains(f, "状态时长之和") {
			t.Fatalf("stale degraded snapshot leaked into the defenses after a validated emit: %v", f)
		}
	}
}
