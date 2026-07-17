package tool

// answer_document_projection_inversion_wordlane_a5_test.go — A5 反转词位单源
// (RANKDIS-SWEEP M8, §29.104.16.1, docs/design/rankdis_sweep_20260716.md 编队
// A5; INV-SUPPLY §29.61.11 转录同词推广收尾, 2026-07-17).
//
// Witness (customlogs/cust_span_vs_prio_info.txt): ONE wire token
// priority_inversion_runnable_wait wore THREE display words —
//
//	行2  调度压力候选            (form-table FormRunnable category)
//	表名 优先级反转·可运行等待    (typelabels via the name lane)
//	cell runnable调度候选        (shape-cell causeKind third word)
//
// and the ◎ overview's strong seats showed the weak 行1 composition word
// (「8.608ms … · runnable [E8]」 beside a 行2 saying 优先级反转候选).
//
// Fix contract pinned here: one token → one family word from ONE composer
// (runtimeTraceProjInversionFamilyWord, typelabels bytes; EN = raw wire
// token), spoken identically by 行2 (CauseCategoryWord), the shape cell
// (ImpactShapeCellTyped), the C7 family-word cell (ImpactFormFamilyWord) and
// the ◎ transcription (ElimClassWord); cross-token word bleed is the negative
// arm; a hardcoded word injection outside the single source reddens the scan.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	a5RunnableWaitWordZH = "优先级反转·可运行等待"
	a5CandidateWordZH    = "优先级反转候选"
)

// a5RunnableWaitSeat is the witness E6/E31 shape: a rank/board row whose typed
// token lanes carry the runnable-overlap type while its dominant state is
// runnable (StateKind=runnable → the row keeps the ⧖ state form; only the
// WORD faces speak the family token word).
func a5RunnableWaitSeat(rank int, eff float64) types.TraceCausalProjectionNode {
	return elimChainNode("E-piw", "shadowhook-task-64305", "priority_inversion_runnable_wait", "runnable", rank, eff, 100)
}

// --- 正臂: witness 形三面同词 --------------------------------------------------

func TestA5RunnableWaitTokenThreeFacesSameWord(t *testing.T) {
	node := a5RunnableWaitSeat(2, 8.608)
	// 行2 category word — zh typelabels bytes, EN raw wire token (D2).
	if word, _ := runtimeTraceProjCauseCategoryWord(node, runtimeTraceProjTreeRowChain, true); word != a5RunnableWaitWordZH {
		t.Fatalf("行2 zh: got %q, want %q", word, a5RunnableWaitWordZH)
	}
	if word, _ := runtimeTraceProjCauseCategoryWord(node, runtimeTraceProjTreeRowChain, false); word != "priority_inversion_runnable_wait" {
		t.Fatalf("行2 en: got %q, want the raw wire token", word)
	}
	// Shape cell — same bytes, never the deleted third word and never a bare
	// single-state claim.
	if cell, generic := runtimeTraceCausalProjectionImpactShapeCellTyped(node, true); cell != a5RunnableWaitWordZH || generic {
		t.Fatalf("shape cell zh: got (%q, %v)", cell, generic)
	}
	if cell, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(node, false); cell != "priority_inversion_runnable_wait" {
		t.Fatalf("shape cell en: got %q", cell)
	}
	// C7 family-word cell — same bytes (the form table's candidate CategoryZH
	// must not dress this token).
	if word := runtimeTraceProjImpactFormFamilyWord(node, true); word != a5RunnableWaitWordZH {
		t.Fatalf("C7 family word zh: got %q", word)
	}
	// ◎ transcription — the 行2 word, never the weak state word.
	projection := gatedCalProjection(node)
	_, fence := elimRenderOverview(t, projection, true)
	seatLine := ""
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "8.608ms") {
			seatLine = line
		}
	}
	if seatLine == "" {
		t.Fatalf("the runnable-overlap seat must render on the ◎ board:\n%s", fence)
	}
	if !strings.Contains(seatLine, a5RunnableWaitWordZH) || strings.Contains(seatLine, "· runnable") {
		t.Fatalf("◎ 词位 must transcribe the 行2 family word (强席不显弱词):\n%s", seatLine)
	}
	// End-to-end tree face: the 行2 identity line speaks the family word.
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(tree, a5RunnableWaitWordZH) {
		t.Fatalf("tree 行2 must carry the family word:\n%s", tree)
	}
	if strings.Contains(tree, "调度压力候选·根因排序#2") {
		t.Fatalf("the runnable-overlap seat's 行2 must not wear the form-table 调度压力候选 (M8 fork):\n%s", tree)
	}
}

// The flag-lane seat (E8 witness shape — candidate Object + candidate flag,
// pure runnable-full account) keeps the candidate word on every face; its ◎
// upgrade off the weak 行1 word is pinned in the GATED-CAL battery (件1⑤ 负臂
// A5 重钉).
func TestA5CandidateFlagSeatKeepsCandidateWord(t *testing.T) {
	node := gatedCalCompositeNode(2)
	node.GatedRunnableMS = 8.608
	node.GatedRunningDeficitMS = 0
	node.EffectiveImpactMS = 8.608
	if word, _ := runtimeTraceProjCauseCategoryWord(node, runtimeTraceProjTreeRowChain, true); word != a5CandidateWordZH {
		t.Fatalf("flag seat 行2 zh: got %q", word)
	}
	if cell, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(node, true); cell != a5CandidateWordZH {
		t.Fatalf("flag seat cell zh: got %q", cell)
	}
	if cell, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(node, false); cell != "priority_inversion_candidate" {
		t.Fatalf("flag seat cell en: got %q", cell)
	}
}

// 表 cell flag 行与值行同词: a FLAGGED row whose typed token lane carries the
// runnable-overlap type speaks that token's word — the same word its
// occurrence-segment (值行) sibling speaks; the flag alone never overrides the
// token back to the candidate word.
func TestA5FlagRowWithRunnableWaitTokenSpeaksTokenWord(t *testing.T) {
	node := a5RunnableWaitSeat(2, 8.608)
	node.PriorityInversionCandidate = true
	if cell, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(node, true); cell != a5RunnableWaitWordZH {
		t.Fatalf("flag+token row cell: got %q, want the token word (flag 行与值行同词)", cell)
	}
	if word, _ := runtimeTraceProjCauseCategoryWord(node, runtimeTraceProjTreeRowChain, true); word != a5RunnableWaitWordZH {
		t.Fatalf("flag+token row 行2: got %q, want the token word", word)
	}
}

// --- 负臂: 异 token 词不串 ------------------------------------------------------

func TestA5CrossTokenWordsNeverBleed(t *testing.T) {
	// The candidate-token seat never wears the runnable-overlap word.
	candidate := gatedCalCompositeNode(1)
	for _, face := range []string{
		firstWordA5(runtimeTraceProjCauseCategoryWord(candidate, runtimeTraceProjTreeRowChain, true)),
		firstWordA5(runtimeTraceCausalProjectionImpactShapeCellTyped(candidate, true)),
		runtimeTraceProjImpactFormFamilyWord(candidate, true),
	} {
		if strings.Contains(face, a5RunnableWaitWordZH) {
			t.Fatalf("candidate seat face %q bleeds the runnable-overlap word", face)
		}
	}
	// The runnable-overlap token seat never wears the candidate word.
	token := a5RunnableWaitSeat(3, 7.727)
	for _, face := range []string{
		firstWordA5(runtimeTraceProjCauseCategoryWord(token, runtimeTraceProjTreeRowChain, true)),
		firstWordA5(runtimeTraceCausalProjectionImpactShapeCellTyped(token, true)),
		runtimeTraceProjImpactFormFamilyWord(token, true),
	} {
		if strings.Contains(face, a5CandidateWordZH) {
			t.Fatalf("runnable-overlap seat face %q bleeds the candidate word", face)
		}
	}
	// Off-family rows keep their lanes: a plain runnable_wait row never enters
	// the composer.
	plain := elimChainNode("E-plain", "worker-1", "runnable_wait", "runnable", 4, 1.0, 400)
	if word, ok := runtimeTraceProjInversionFamilyWord(plain, true); ok {
		t.Fatalf("plain runnable_wait row must stay off the family composer, got %q", word)
	}
	if word, _ := runtimeTraceProjCauseCategoryWord(plain, runtimeTraceProjTreeRowChain, true); word != "调度压力候选" {
		t.Fatalf("plain runnable_wait 行2 must keep the form-table word, got %q", word)
	}
}

func firstWordA5(word string, _ bool) string { return word }

// 行尾形态词撤 geometry: a STATELESS family-token row relocates its word to
// 行2 (the tag lane would re-render the same shape-cell word — suppressed,
// exactly like the pre-A5 relocation arm it replaces), while a state-bearing
// row keeps its 裁定4 state tag (relocated=false — the · runnable tag is a
// state disclosure, never a second type word).
func TestA5RelocationForksOnStateTagLane(t *testing.T) {
	stateless := a5RunnableWaitSeat(2, 8.608)
	stateless.StateKind = ""
	if word, relocated := runtimeTraceProjCauseCategoryWord(stateless, runtimeTraceProjTreeRowChain, true); word != a5RunnableWaitWordZH || !relocated {
		t.Fatalf("stateless family row must relocate its word to 行2: got (%q, %v)", word, relocated)
	}
	stateful := a5RunnableWaitSeat(2, 8.608)
	if word, relocated := runtimeTraceProjCauseCategoryWord(stateful, runtimeTraceProjTreeRowChain, true); word != a5RunnableWaitWordZH || relocated {
		t.Fatalf("state-bearing family row keeps its state tag lane: got (%q, %v)", word, relocated)
	}
	// End-to-end stateless render: the family word appears once per row face
	// group — never doubled by the tag lane.
	projection := gatedCalProjection(stateless)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	rowBlock := ""
	for _, block := range strings.Split(tree, "─ ") {
		if strings.Contains(block, "[E-piw]") || strings.Contains(block, "shadowhook-task-64305") {
			rowBlock = block
			break
		}
	}
	if rowBlock == "" {
		t.Fatalf("stateless family row must render:\n%s", tree)
	}
	if got := strings.Count(rowBlock, a5RunnableWaitWordZH); got != 1 {
		t.Fatalf("the family word must appear exactly once on the row block, got %d:\n%s", got, rowBlock)
	}
}

// --- crown no-drift companion ---------------------------------------------------

func TestA5SelfCrownCategoryFollowsFamilyWord(t *testing.T) {
	target := "ease.cloudmusic-63993"
	node := elimChainNode("E-e6", target, "priority_inversion_runnable_wait", "runnable", 6, 1.782, 500)
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", target},
		WindowStartTs: 17729.471, WindowEndTs: 17729.623,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	state, category := runtimeTraceProjSelfCauseCrownState(node, projection, model, true)
	if state != "runnable" {
		t.Fatalf("crown state: got %q", state)
	}
	if category != a5RunnableWaitWordZH {
		t.Fatalf("crown category must follow the 行2 family word (no drift), got %q", category)
	}
}

// --- 单源突变: hardcoded word injection reddens ---------------------------------

// TestA5InversionWordSingleSourceScan — the runnable-overlap display word's
// BYTES live only in the typelabels table (the composer reads them through
// runtimeTraceRootCauseTypeZHLabel), and the deleted third word never returns.
// A hardcoded re-spelling in any display package is the M8 fork reborn.
func TestA5InversionWordSingleSourceScan(t *testing.T) {
	dirs := []string{".", "../tracequery", "../types", "../preview", "../render", "../tracefence"}
	wordFiles := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(raw), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				if at := strings.Index(line, " //"); at >= 0 {
					line = line[:at]
				}
				if strings.Contains(line, "\"runnable调度候选\"") || strings.Contains(line, "\"runnable scheduling candidate\"") {
					t.Errorf("%s: the deleted M8 third word is back: %s", path, strings.TrimSpace(line))
				}
				if strings.Contains(line, a5RunnableWaitWordZH) {
					wordFiles[filepath.Base(path)] = true
				}
			}
		}
	}
	if len(wordFiles) != 1 || !wordFiles["answer_document_mutation_runtime_typelabels.go"] {
		t.Fatalf("the %s bytes must live ONLY in the typelabels table, found in: %v", a5RunnableWaitWordZH, wordFiles)
	}
}

// The registry-side family membership: BOTH inversion tokens resolve to the ⇅
// family through the token table (a stateless family row never falls to ◦),
// judged by the UXG-1 family single point — never a local token spelling.
func TestA5TokenFamilyCarriesBothInversionTokens(t *testing.T) {
	for _, token := range []string{"priority_inversion_candidate", "priority_inversion_runnable_wait"} {
		if form := runtimeTraceProjImpactFormTokenFamily(token); form != runtimeTraceProjImpactFormInversion {
			t.Fatalf("token %s must ride the inversion form family, got %v", token, form)
		}
	}
}
