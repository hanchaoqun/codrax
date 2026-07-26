package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_nextinfo_v1_test.go — NEXTINFO-V1 tree face:
// the CPU-constraint description line stops speaking the inverted 「策略
// restricted=true」 claim and speaks the truthful boost word instead. Both
// language faces pinned; the legacy token never renders again.

func nextInfoV1ConstraintNode(policy string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Subject: "worker-300", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset",
		CPUConstraintPolicy: policy,
		CPUConstraintCPUSet: "background",
	}
}

func TestNextInfoV1TreeFaceSpeaksBoostWord(t *testing.T) {
	row := runtimeTraceProjTreeRow{Node: nextInfoV1ConstraintNode("next_info affinity=f ices_boost=true"), marks: &runtimeTraceProjMarkSet{}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if !strings.Contains(notes, "策略 ices_boost=true(前台加速)") {
		t.Fatalf("zh face must speak the boost word:\n%s", notes)
	}
	notesEN := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, false), "\n")
	if !strings.Contains(notesEN, "policy ices_boost=true (foreground boost)") {
		t.Fatalf("en face must speak the boost word:\n%s", notesEN)
	}
}

func TestNextInfoV1TreeFaceLegacyTokenNeverRenders(t *testing.T) {
	row := runtimeTraceProjTreeRow{Node: nextInfoV1ConstraintNode("next_info affinity=f restricted=true"), marks: &runtimeTraceProjMarkSet{}}
	for _, zh := range []bool{true, false} {
		notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, zh), "\n")
		if strings.Contains(notes, "restricted=true") {
			t.Fatalf("the retired restricted claim must never render (zh=%v):\n%s", zh, notes)
		}
	}
}

func nextInfoV1SchedEventView(t *testing.T, nextInfo string) tracequery.EventView {
	t.Helper()
	ev, ok := tracequery.ParseLine(1, `        app-20   (   20) [001] .... 1.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53 next_info=`+nextInfo+` cg=top-app`, nil)
	if !ok {
		t.Fatal("fixture line must parse")
	}
	return tracequery.EventView{Event: ev}
}

func TestNextInfoV1DetailFaceRetiresRestrictedToken(t *testing.T) {
	// Dual-review pins lens: the scheduler detail face (event_search rows)
	// was the second display consumer of the retired token — resurrecting
	// " restricted=%t" here must go red.
	detail := traceEventSchedulerDetail(nextInfoV1SchedEventView(t, "e,0,1,1,0,1"))
	if strings.Contains(detail, "restricted=") {
		t.Fatalf("detail face must not speak the retired restricted token: %q", detail)
	}
	if !strings.Contains(detail, "ices_boost=true") {
		t.Fatalf("detail face must speak the truthful boost word: %q", detail)
	}
	// Out-of-doc boost claims nothing on the boost lane here either.
	detail2 := traceEventSchedulerDetail(nextInfoV1SchedEventView(t, "f,10,1,3,0"))
	if strings.Contains(detail2, "restricted=") || strings.Contains(detail2, "ices_boost=") {
		t.Fatalf("out-of-doc boost must claim nothing on the detail face: %q", detail2)
	}
}

func TestNextInfoV1ToolWordFacesSpeakBoost(t *testing.T) {
	// Both LLM-facing word surfaces — Description AND Parameters — teach the
	// ices_boost reading; the retired "affinity/restricted" pairing must not
	// survive on either (the byte golden pins only Description).
	tq := &TraceQuery{}
	for name, face := range map[string]string{
		"description": tq.Description(),
		"parameters":  string(tq.Parameters()),
	} {
		if strings.Contains(face, "affinity/restricted") {
			t.Fatalf("%s still teaches the retired affinity/restricted pairing", name)
		}
		if !strings.Contains(face, "affinity/ices_boost") {
			t.Fatalf("%s must teach the affinity/ices_boost reading", name)
		}
	}
}
