package tool

// answer_document_projection_spantop_test.go — SPANTOP-1 pins (§29.131, user
// ruling 2026-07-18): the semantic family seat's constituent top-3 sub-rows +
// counted remainder line.
//
//   P1  µs identity: Σ(top-3) + remainder == the seat's 行1 total, parsed
//       back from the DISPLAYED strings (GATED-CAL three-way identity
//       precedent);
//   P2  typed all-or-nothing: any missing/misaligned member carrier (wall
//       list, line ranges, roster, order, 单次最大 cross-pin, identity) keeps
//       the legacy roster sub-rows byte-identically (整块不发,席行现状);
//   P3  unrelated seats byte-identical: non-semantic families and
//       non-sum_disjoint calibers never enter the block;
//   P4  cap=3 named constant;
//   P5  remainder word form + [E#] pointer;
//   P6  caliber follows the seat: 单段 (segment vocabulary), never a wait
//       word (XERR1 lesson);
//   P7  zero on-chain claims: no ⛓, no 链上 word on the sub-rows, and the ◎
//       overview/census face is invariant to the block toggling (链上 span
//       家族席硬纪律②③);
//   P8  detail stanza per-member (行a..b) locators + C4 table pointer row
//       (两面互指,不再第三面抄 — C4 pins live in the rcm2/elim tests).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// spantopConstituentProjection is the production-shaped witness geometry (the
// donghu-2955 E34 「JIT编译 2次 合计2.388ms」 form scaled to five members so
// the remainder line engages): ONE semantic family seat whose complete member
// carriers are aligned and whose member Σ equals the published seat value to
// the µs (sum_disjoint — the engine sets TotalMs = SumMs on that caliber).
func spantopConstituentProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "spantop-e1",
			Subject: "worker-9", Predicate: "trace_semantic_span",
			Object: "class_verification", SemanticClass: "class_verification",
			SpanName:    "VerifyClass com.demo.Big",
			SupportRefs: []string{"spantop.systrace:1200-1600"},
			LineStart:   1200, LineEnd: 1600,
			ImpactMS: 8.000, CumulativeImpactMS: 8.000, EffectiveImpactMS: 8.000,
			QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.2,
			FamilyMemberCount: 5, FamilyMemberMaxMS: 3.000, FamilyMemberMinMS: 0.700,
			FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{
				"VerifyClass com.demo.Big 3.000ms",
				"VerifyClass com.demo.Mid 2.000ms",
				"VerifyClass com.demo.Small 1.500ms",
				"VerifyClass com.demo.Tiny 0.800ms",
				"VerifyClass com.demo.Nano 0.700ms",
			},
			FamilyMemberWallMS:     []float64{3.000, 2.000, 1.500, 0.800, 0.700},
			FamilyMemberLineRanges: [][2]int{{1200, 1240}, {1250, 1290}, {1300, 1340}, {1350, 1390}, {1400, 1440}},
			Confidence:             0.7,
		}},
	}
}

func spantopNode() types.TraceCausalProjectionNode {
	return spantopConstituentProjection().SemanticSpans[0]
}

func spantopSubRows(t *testing.T, node types.TraceCausalProjectionNode, zh bool) ([]string, bool) {
	t.Helper()
	return runtimeTraceProjFamilySpanTopSubRows(runtimeTraceProjTreeRow{Node: node, EvidenceTag: "E1"}, zh)
}

// --- P1 + P5 + P6: the block form and its displayed identity -------------------

func TestSpanTopConstituentBlockEndToEnd(t *testing.T) {
	projection := spantopConstituentProjection()
	model, fence := rcm2RenderFence(t, projection, true)
	t.Logf("spantop zh fence:\n%s", fence)
	for _, want := range []string{
		"成员(span原文) VerifyClass com.demo.Big 单段3.000ms 行1200..1240",
		"成员(span原文) VerifyClass com.demo.Mid 单段2.000ms 行1250..1290",
		"成员(span原文) VerifyClass com.demo.Small 单段1.500ms 行1300..1340",
		"另有 2 段 合计1.500ms · 全清单见明细[E1]",
	} {
		if !strings.Contains(squashSpaces(fence), want) {
			t.Fatalf("the constituent block must render %q:\n%s", want, fence)
		}
	}
	// The 4th/5th members never render as sub-rows (cap=3).
	if strings.Contains(fence, "com.demo.Tiny 单段") || strings.Contains(fence, "com.demo.Nano 单段") {
		t.Fatalf("members beyond the cap must fold into the remainder:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkFamilySpanTop) {
		t.Fatalf("the block must record its legend mark")
	}
	// P1: parse the DISPLAYED µs values back and check the identity against
	// the DISPLAYED seat total (合计8.000ms).
	if !strings.Contains(fence, "合计8.000ms") {
		t.Fatalf("the seat 行1 must publish 合计8.000ms:\n%s", fence)
	}
	sumUS := int64(3000 + 2000 + 1500) // displayed top-3
	sumUS += 1500                      // displayed remainder 合计1.500ms
	if sumUS != 8000 {
		t.Fatalf("Σ(top3)+remainder must equal the seat total to the µs: %d != 8000", sumUS)
	}
	// P6: segment vocabulary only — never a wait word on the sub-rows.
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "单段") && (strings.Contains(line, "等待") || strings.Contains(line, "阻塞")) {
			t.Fatalf("member sub-rows must not impersonate a wait caliber: %q", line)
		}
	}
}

func TestSpanTopConstituentBlockEnglishFace(t *testing.T) {
	projection := spantopConstituentProjection()
	_, fence := rcm2RenderFence(t, projection, false)
	for _, want := range []string{
		"member (verbatim span) VerifyClass com.demo.Big · segment 3.000ms · lines 1200..1240",
		"2 more segments · total 1.500ms · full inventory in the detail blocks [E1]",
	} {
		if !strings.Contains(squashSpaces(fence), want) {
			t.Fatalf("the EN constituent block must render %q:\n%s", want, fence)
		}
	}
}

func squashSpaces(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimLeft(line, "│ \t"))
		b.WriteString(" ")
	}
	out := b.String()
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return out
}

// --- P2: all-or-nothing negative arms ------------------------------------------

func TestSpanTopAllOrNothingNegativeArms(t *testing.T) {
	legacy := runtimeTraceProjFamilyRosterSubRows(spantopNode(), true)
	if len(legacy) == 0 {
		t.Fatalf("fixture must have a legacy roster form")
	}
	arms := map[string]func(*types.TraceCausalProjectionNode){
		// Belt-and-braces (修根轮 件2 遮蔽备案, 2026-07-18): the absent form is
		// unavoidably Σ-shadowed — a nil wall list sums to 0 against a positive
		// seat total, so the Σ identity gate must also reject it (the length
		// gate's own de-masked pin is the Σ-preserving short arm below).
		"wall list absent": func(n *types.TraceCausalProjectionNode) { n.FamilyMemberWallMS = nil },
		// Belt-and-braces: Σ-shadowed (7.3 != 8.0); the de-masked length pin is
		// "wall length gate alone" below.
		"wall list short":   func(n *types.TraceCausalProjectionNode) { n.FamilyMemberWallMS = n.FamilyMemberWallMS[:4] },
		"line ranges short": func(n *types.TraceCausalProjectionNode) { n.FamilyMemberLineRanges = n.FamilyMemberLineRanges[:4] },
		"roster short":      func(n *types.TraceCausalProjectionNode) { n.FamilyMemberRoster = n.FamilyMemberRoster[:4] },
		// --- De-masked single-gate arms (修根轮 件2, 2026-07-18; M5c 去遮蔽
		// 先例): each form below PASSES the Σ µs identity, so exactly one gate
		// can reject it — deleting that gate makes the block render and the arm
		// go red (the Σ gate can no longer absorb the mutation). -------------
		"positive gate alone (zero member, Σ preserved)": func(n *types.TraceCausalProjectionNode) {
			// Σ = 3.000+2.000+1.500+1.500+0 = 8.000 == seat total; order stays
			// non-increasing; only v<=0 on the last member can reject.
			n.FamilyMemberWallMS = []float64{3.000, 2.000, 1.500, 1.500, 0}
			n.FamilyMemberRoster = []string{
				"VerifyClass com.demo.Big 3.000ms",
				"VerifyClass com.demo.Mid 2.000ms",
				"VerifyClass com.demo.Small 1.500ms",
				"VerifyClass com.demo.Tiny 1.500ms",
				"VerifyClass com.demo.Nano 0.000ms",
			}
		},
		"wall length gate alone (short list, Σ preserved)": func(n *types.TraceCausalProjectionNode) {
			// 4 entries summing to the seat total (3.000+2.000+1.500+1.500 =
			// 8.000): roster/ranges stay complete, the top-3 suffixes stay
			// aligned — only len(wall) != n can reject.
			n.FamilyMemberWallMS = []float64{3.000, 2.000, 1.500, 1.500}
		},
		"totalUS gate alone (zero seat, sub-µs members)": func(n *types.TraceCausalProjectionNode) {
			// All value channels zero → totalUS = 0; two sub-µs members (0.0004
			// each: positive, so the v<=0 gate passes; each rounds to 0µs, so
			// Σ == totalUS == 0 and the identity gate passes) — only the
			// totalUS<=0 gate can reject this all-zero form.
			n.ImpactMS, n.CumulativeImpactMS, n.EffectiveImpactMS = 0, 0, 0
			n.FamilyMemberCount = 2
			n.FamilyMemberMaxMS, n.FamilyMemberMinMS = 0.0004, 0.0004
			n.FamilyMemberWallMS = []float64{0.0004, 0.0004}
			n.FamilyMemberLineRanges = [][2]int{{1200, 1240}, {1250, 1290}}
			n.FamilyMemberRoster = []string{
				"VerifyClass com.demo.Big 0.000ms",
				"VerifyClass com.demo.Mid 0.000ms",
			}
		},
		// The order arm keeps Σ==total AND aligned roster suffixes so ONLY
		// the desc gate can reject (突变复核: an identity- or suffix-shadowed
		// arm let the neutered order gate survive — the swap keeps every
		// other gate satisfied).
		"order violated": func(n *types.TraceCausalProjectionNode) {
			n.FamilyMemberWallMS = []float64{3.000, 1.500, 2.000, 0.800, 0.700}
			n.FamilyMemberRoster = []string{
				"VerifyClass com.demo.Big 3.000ms",
				"VerifyClass com.demo.Mid 1.500ms",
				"VerifyClass com.demo.Small 2.000ms",
				"VerifyClass com.demo.Tiny 0.800ms",
				"VerifyClass com.demo.Nano 0.700ms",
			}
		},
		"identity broken (seat value)": func(n *types.TraceCausalProjectionNode) {
			n.ImpactMS, n.CumulativeImpactMS, n.EffectiveImpactMS = 8.100, 8.100, 8.100
		},
		"单次最大 cross-pin broken": func(n *types.TraceCausalProjectionNode) { n.FamilyMemberMaxMS = 2.900 },
		"roster suffix mismatch": func(n *types.TraceCausalProjectionNode) {
			n.FamilyMemberRoster[0] = "VerifyClass com.demo.Big 3.001ms"
		},
		// Belt-and-braces: Σ-shadowed (7.3 != 8.0); the de-masked positive pin
		// is "positive gate alone" above.
		"non-positive member": func(n *types.TraceCausalProjectionNode) {
			n.FamilyMemberWallMS = []float64{3.000, 2.000, 1.500, 0.800, 0}
		},
		"interval_union caliber": func(n *types.TraceCausalProjectionNode) { n.FamilyFoldCaliber = "interval_union" },
		"no semantic class":      func(n *types.TraceCausalProjectionNode) { n.SemanticClass = "" },
	}
	for name, mutate := range arms {
		node := spantopNode()
		mutate(&node)
		if rows, ok := runtimeTraceProjFamilySpanTopSubRows(runtimeTraceProjTreeRow{Node: node, EvidenceTag: "E1"}, true); ok {
			t.Fatalf("%s: the block must not render (got %v)", name, rows)
		}
	}
	// Remainder without an [E#] tag: the pointer would dangle — block absent.
	if rows, ok := runtimeTraceProjFamilySpanTopSubRows(runtimeTraceProjTreeRow{Node: spantopNode()}, true); ok {
		t.Fatalf("a remainder without the seat's [E#] must not render (got %v)", rows)
	}
	// Identity arithmetic note: "identity broken" above keeps Σ(members)=8.000
	// against a displayed 8.100 seat — one µs of drift already blocks (integer
	// µs equality, never a tolerance band).
	node := spantopNode()
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 8.001, 8.001, 8.001
	if _, ok := spantopSubRows(t, node, true); ok {
		t.Fatalf("a 1µs identity drift must block the whole block")
	}
}

// --- P2b: an [E#]-less seat with n<=cap needs no pointer and may render --------

func TestSpanTopNoRemainderNeedsNoEvidenceTag(t *testing.T) {
	node := spantopNode()
	node.FamilyMemberCount = 3
	node.FamilyMemberRoster = node.FamilyMemberRoster[:3]
	node.FamilyMemberWallMS = []float64{3.000, 2.000, 1.500}
	node.FamilyMemberLineRanges = node.FamilyMemberLineRanges[:3]
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 6.500, 6.500, 6.500
	rows, ok := runtimeTraceProjFamilySpanTopSubRows(runtimeTraceProjTreeRow{Node: node}, true)
	if !ok || len(rows) != 3 {
		t.Fatalf("a complete n<=cap family renders all members and no remainder: %v %v", rows, ok)
	}
	for _, row := range rows {
		if strings.Contains(row, "另有") {
			t.Fatalf("no remainder line without a remainder: %q", row)
		}
	}
}

// --- P3: unrelated seats stay on their legacy faces ----------------------------

func TestSpanTopUnrelatedSeatsUntouched(t *testing.T) {
	// A generic (non-semantic) family seat: the block never engages even with
	// aligned carriers — the scope is the semantic family seat (纪律6).
	projection := rcm2OpendirInodeFamilyProjection()
	node := projection.OnChainCauses[0]
	node.FamilyMemberWallMS = []float64{1.136, 0.462}
	node.FamilyMemberLineRanges = [][2]int{{900, 920}, {925, 940}}
	if rows, ok := spantopSubRows(t, node, true); ok {
		t.Fatalf("a non-semantic family must keep its legacy face (got %v)", rows)
	}
	// Fence-level: the generic family fixture renders byte-identically with
	// and without the new carriers present (the block is the ONLY consumer).
	_, before := rcm2RenderFence(t, projection, true)
	projection.OnChainCauses[0].FamilyMemberWallMS = []float64{1.136, 0.462}
	projection.OnChainCauses[0].FamilyMemberLineRanges = [][2]int{{900, 920}, {925, 940}}
	_, after := rcm2RenderFence(t, projection, true)
	if before != after {
		t.Fatalf("unrelated seat bytes must not move:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// --- P4: the customer-named cap ------------------------------------------------

func TestSpanTopCapConstant(t *testing.T) {
	if runtimeTraceProjFamilySpanTopCap != 3 {
		t.Fatalf("the customer-named constituent cap is 3, got %d", runtimeTraceProjFamilySpanTopCap)
	}
	rows, ok := spantopSubRows(t, spantopNode(), true)
	if !ok || len(rows) != runtimeTraceProjFamilySpanTopCap+1 {
		t.Fatalf("top-%d + one remainder line, got %d rows (%v)", runtimeTraceProjFamilySpanTopCap, len(rows), ok)
	}
}

// --- P7: zero on-chain claims + census invariance ------------------------------

func TestSpanTopSubRowsMakeNoOnChainClaims(t *testing.T) {
	// The sub-rows are strings on the seat's subordinate lane — they carry no
	// ⛓, no 链上 word, and no credential vocabulary (硬纪律②③).
	for _, zh := range []bool{true, false} {
		rows, ok := spantopSubRows(t, spantopNode(), zh)
		if !ok {
			t.Fatalf("fixture must render")
		}
		for _, row := range rows {
			for _, banned := range []string{"⛓", "链上", "on-chain", "凭证", "credential"} {
				if strings.Contains(row, banned) {
					t.Fatalf("constituent sub-row must not claim %q: %q", banned, row)
				}
			}
		}
	}
	// The ◎ overview/census face is INVARIANT to the block toggling: the
	// sub-rows are not model rows, so populations/denominators cannot move.
	withBlock := spantopConstituentProjection()
	withoutBlock := spantopConstituentProjection()
	withoutBlock.SemanticSpans[0].FamilyMemberWallMS = nil
	modelWith := buildRuntimeTraceProjTreeModel(withBlock, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	modelWithout := buildRuntimeTraceProjTreeModel(withoutBlock, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	elimWith := runtimeTraceProjElimOverviewFence(withBlock, modelWith, true)
	elimWithout := runtimeTraceProjElimOverviewFence(withoutBlock, modelWithout, true)
	if elimWith != elimWithout {
		t.Fatalf("the ◎ face must be invariant to the constituent block:\n--- with ---\n%s\n--- without ---\n%s", elimWith, elimWithout)
	}
	// Every non-member line of the two fences is byte-identical (值通道/
	// 序数域零动: only the sub-row lines differ between the two forms).
	fenceWith := runtimeTraceProjTreeFence(modelWith, true)
	fenceWithout := runtimeTraceProjTreeFence(modelWithout, true)
	filter := func(s string) string {
		var kept []string
		for _, line := range strings.Split(s, "\n") {
			if strings.Contains(line, "成员") || strings.Contains(line, "其余") ||
				strings.Contains(line, "单段") || strings.Contains(line, "另有") ||
				strings.Contains(line, "行12") || strings.Contains(line, "全清单") {
				continue
			}
			kept = append(kept, line)
		}
		return strings.Join(kept, "\n")
	}
	if filter(fenceWith) != filter(fenceWithout) {
		t.Fatalf("only the sub-row lines may differ:\n--- with ---\n%s\n--- without ---\n%s", fenceWith, fenceWithout)
	}
}

// --- P8: detail stanza per-member locators -------------------------------------

func TestSpanTopDetailStanzaMemberLocators(t *testing.T) {
	projection := spantopConstituentProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	for _, want := range []string{
		"VerifyClass com.demo.Big 3.000ms(行1200..1240)",
		"VerifyClass com.demo.Nano 0.700ms(行1400..1440)",
		"(共5,列5)",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("the detail stanza must carry %q:\n%s", want, detail)
		}
	}
	// Misaligned carriers annotate nothing (bare roster stays).
	bare := spantopConstituentProjection()
	bare.SemanticSpans[0].FamilyMemberLineRanges = bare.SemanticSpans[0].FamilyMemberLineRanges[:4]
	bareModel := buildRuntimeTraceProjTreeModel(bare, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	bareDetail := runtimeTraceProjDetailFullText(bareModel, true)
	if strings.Contains(bareDetail, "(行1200..1240)") {
		t.Fatalf("a misaligned line-range set must not annotate the roster:\n%s", bareDetail)
	}
	if !strings.Contains(bareDetail, "VerifyClass com.demo.Big 3.000ms") {
		t.Fatalf("the bare roster must stay:\n%s", bareDetail)
	}
}

// --- semDual arm hookup (dual-caliber 行3 + constituent block) ------------------

func TestSpanTopSemanticChainDualCaliberSeatEndToEnd(t *testing.T) {
	// 修根轮 件3 (2026-07-18): the semDual 行3 arm carries its OWN spantop
	// hookup (the dual-caliber branch replaces the legacy roster sub-rows too;
	// the generic family arm's hookup is pinned by the E2E test above) — this
	// is that hookup's only pin: 行3 speaks the published INTERSECTION
	// (链上计入 + 窗口投影合计 disclosure) while the constituent sub-rows
	// decompose the 行1 UNION value (the µs identity runs against the display
	// impact, so Σ(members) == 8.000 union, not the 6.400 intersection).
	projection := spantopConstituentProjection()
	node := &projection.SemanticSpans[0]
	node.SemanticChainProjectedMS = 6.400
	node.EffectiveImpactMS = 6.400
	// Structured-parts level (wrap-free): dual-caliber 行3 + spantop sub-rows.
	row := runtimeTraceProjTreeRow{Node: *node, Kind: runtimeTraceProjTreeRowSemantic, HasData: true, EvidenceTag: "E1"}
	structured, ok := runtimeTraceProjCauseStructuredParts(row, true)
	if !ok {
		t.Fatalf("the semDual family row must build the cause grammar")
	}
	if !strings.Contains(structured.Breakdown, "有效归因 6.400ms = 链上计入(共5段,同线程)") ||
		!strings.Contains(structured.Breakdown, "(窗口投影合计 8.000ms 见明细)") {
		t.Fatalf("行3 must speak the dual-caliber intersection form: %q", structured.Breakdown)
	}
	if len(structured.SubRows) != runtimeTraceProjFamilySpanTopCap+1 {
		t.Fatalf("the semDual seat must carry the constituent top-%d + remainder sub-rows, got %v",
			runtimeTraceProjFamilySpanTopCap, structured.SubRows)
	}
	for _, want := range []string{
		"成员(span原文) VerifyClass com.demo.Big 单段3.000ms 行1200..1240",
		"另有 2 段 合计1.500ms · 全清单见明细[E1]",
	} {
		found := false
		for _, sub := range structured.SubRows {
			if strings.Contains(sub, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("the semDual sub-rows must carry %q: %v", want, structured.SubRows)
		}
	}
	// E2E fence level: both faces render together and the legend marks record
	// the pairing (spantop block + dual-caliber word on one seat).
	model, fence := rcm2RenderFence(t, projection, true)
	for _, want := range []string{
		"成员(span原文) VerifyClass com.demo.Big 单段3.000ms 行1200..1240",
		"另有 2 段 合计1.500ms · 全清单见明细[E1]",
	} {
		if !strings.Contains(squashSpaces(fence), want) {
			t.Fatalf("the semDual fence must render %q:\n%s", want, fence)
		}
	}
	if !strings.Contains(fence, "链上计入") {
		t.Fatalf("the semDual fence must keep the dual-caliber word:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkFamilySpanTop) ||
		!model.Marks.has(runtimeTraceProjMarkFamilyChainIntersection) {
		t.Fatalf("the semDual seat must record both legend marks")
	}
}

// --- name truncation (B5b tail-keeping) ----------------------------------------

func TestSpanTopNameTailTruncation(t *testing.T) {
	longName := "JIT compiling void android.widget.TextView$TextAppearanceAttributes.<init>() (kind=Baseline) from /system/framework/framework.jar!classes4.dex, dex_location=/system/framework/framework.jar!classes4.dex)"
	face, truncated := runtimeTraceProjFamilySpanTopNameFace(longName, runtimeTraceProjFamilySpanTopNameBudget)
	if !truncated {
		t.Fatalf("a long dex_location name must truncate")
	}
	if !strings.HasPrefix(face, "JIT compilin") || !strings.Contains(face, "…") {
		t.Fatalf("the head keeps the action word: %q", face)
	}
	if !strings.HasSuffix(face, "framework.jar!classes4.dex)") {
		t.Fatalf("the tail (distinguishing segment) must survive whole-budget: %q", face)
	}
	short := "VerifyClass com.demo.Big"
	if got, cut := runtimeTraceProjFamilySpanTopNameFace(short, runtimeTraceProjFamilySpanTopNameBudget); cut || got != short {
		t.Fatalf("a fitting name stays verbatim: %q %v", got, cut)
	}
	// A truncated block drops the C12 (span原文) verbatim chip — the cut name
	// is no longer verbatim (the detail stanza keeps the bytes).
	node := spantopNode()
	node.FamilyMemberRoster[0] = longName + " 3.000ms"
	rows, ok := spantopSubRows(t, node, true)
	if !ok {
		t.Fatalf("the truncated block must still render")
	}
	for _, row := range rows[:3] {
		if strings.Contains(row, "(span原文)") {
			t.Fatalf("a truncated block must not wear the verbatim chip: %q", row)
		}
	}
}
