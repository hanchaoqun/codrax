package ground

import (
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ground_generic_callee_test.go — colleague B1554 grounding matrix
// (colleague_merge_audit §40.59). Witness: on an already-read
// `return std::make_unique<ConsoleSink>();` every legal callee anchor failed
// because the regex tier's call shapes stopped at `callee(` and the AST row
// carried the instantiated spelling; the fallback explanation then called
// the failure "token not found" although the token was visible. The matrix
// pins, per language, that the bare callee grounds on a generic call
// (regex tier, no graph), that a wrong target on the same line — a type
// argument, a comparison operand — stays ungrounded, and that the two
// ungrounded explanations (absent token vs target/shape mismatch) are
// distinct.

func groundCallAnchor(line string, anchor string, graph *repomap.Graph) (types.EvidenceItem, Report) {
	gc := &Context{LineIndex: map[string]map[int]string{"src/file": {18: line}}, Graph: graph}
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "src/file", LineStart: 18, LineEnd: 18,
		Subject: "caller", Predicate: "calls", Object: anchor,
		AnchorKind: types.AnchorCall, AnchorSymbol: anchor,
	}
	report := GroundItem(&item, gc)
	return item, report
}

func TestGenericCallAnchorsGroundAcrossLanguagesWithoutGraph(t *testing.T) {
	cases := []struct {
		lang, line, anchor string
	}{
		{"cpp qualified template", "return std::make_unique<ConsoleSink>();", "make_unique"},
		{"cpp qualified template full target", "return std::make_unique<ConsoleSink>();", "std::make_unique"},
		{"cpp template method", "auto sink = registry.get<ConsoleSink>(kind);", "get"},
		{"cpp nested template", "auto map = build<std::map<std::string, int>>(kind);", "build"},
		{"rust turbofish", "let value = parse::<u32>(text)?;", "parse"},
		{"rust path turbofish", "let size = std::mem::size_of::<Header>();", "size_of"},
		{"rust method turbofish", "let item = repo.load::<Item>(1);", "load"},
		{"swift generic constructor", "let box = Box<Int>(value)", "Box"},
		{"kotlin generic call", "val items = listOf<Int>(1, 2)", "listOf"},
		{"kotlin generic method", "val item = repo.load<Item>(key)", "load"},
		{"typescript generic call", "const parsed = parse<number>(text);", "parse"},
		{"typescript generic method", "const item = repo.load<Item>(key);", "load"},
		{"cangjie generic call", "let parsed = parse<Int64>(text)", "parse"},
		{"cangjie function-type argument", "let handler = wrap<(Int64)->Unit>(callback)", "wrap"},
		{"java explicit type witness", "Object parsed = Util.<String>parse(text);", "parse"},
		{"rust reference argument", "let text = parse::<&str>(input);", "parse"},
		{"space before paren", "auto sink = make_unique<ConsoleSink> ();", "make_unique"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingGrounded || item.LineStart != 18 {
				t.Fatalf("%q anchor %q must ground at the cited line: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
		})
	}
}

func TestGenericCallLinesRejectWrongTargetsAndComparisons(t *testing.T) {
	cases := []struct {
		name, line, anchor string
	}{
		{"type argument is not the callee", "return std::make_unique<ConsoleSink>();", "ConsoleSink"},
		{"rust type argument", "let value = parse::<Header>(text)?;", "Header"},
		{"kotlin type argument", "val items = listOf<Item>(one)", "Item"},
		{"comparison chain operand", "if (lhs<limit && cnt>(max)) {", "lhs"},
		{"comparison with spaces", "if (lhs < limit && cnt > (max)) {", "lhs"},
		{"expression operand", "if (lhs<limit+1>(max)) {", "lhs"},
		{"or chain operand", "if (lhs<limit || cnt>(max)) {", "lhs"},
		{"unbalanced angle", "if (lhs<limit(max)) {", "lhs"},
		{"identifier suffix", "auto s = fmake_unique<ConsoleSink>();", "make_unique"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingUngrounded {
				t.Fatalf("%q anchor %q must stay ungrounded: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
		})
	}
}

// AST tier: the extractor now publishes the bare callee on the row, so the
// symbol-table path grounds the same anchor from the relation alone.
func TestGenericCallAnchorGroundsFromBareCalleeRelation(t *testing.T) {
	graph := &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"src/file": {
			RelPath: "src/file", Language: repomap.LangCpp,
			Relations: []repomap.Relation{{Kind: "call", Line: 18, ToEP: repomap.RelationEndpoint{Name: "make_unique", Receiver: "std"}}},
		},
	}}
	item, report := groundCallAnchor("return std::make_unique<ConsoleSink>();", "make_unique", graph)
	if report.Status != types.GroundingGrounded {
		t.Fatalf("bare callee relation must ground the generic call: report=%+v item=%+v", report, item)
	}
}

// The ungrounded explanation names the actual failure: a visible token
// with the wrong shape is a target/shape mismatch, a token that is not on
// the read lines is an absence. Both wordings are pinned so a repair hint
// can never conflate them again.
func TestUngroundedCallExplanationSplitsAbsenceFromShapeMismatch(t *testing.T) {
	item, report := groundCallAnchor("return std::make_unique<ConsoleSink>();", "ConsoleSink", nil)
	if report.Status != types.GroundingUngrounded {
		t.Fatalf("type argument must not ground as the callee: %+v", report)
	}
	if !strings.Contains(item.GroundingNote, `anchor_symbol "ConsoleSink" is present as a whole-word token near line 18`) ||
		!strings.Contains(item.GroundingNote, `anchor_kind "call" requires`) ||
		!strings.Contains(item.GroundingNote, "target/shape mismatch, not a missing token") {
		t.Fatalf("visible token with the wrong shape must be explained as a target/shape mismatch: %q", item.GroundingNote)
	}
	if strings.Contains(item.GroundingNote, "not found as a whole-word token") {
		t.Fatalf("shape mismatch must not be worded as token absence: %q", item.GroundingNote)
	}
	item, report = groundCallAnchor("return std::make_unique<ConsoleSink>();", "flushSink", nil)
	if report.Status != types.GroundingUngrounded {
		t.Fatalf("absent callee must stay ungrounded: %+v", report)
	}
	if !strings.Contains(item.GroundingNote, `anchor_symbol "flushSink" not found as a whole-word token near line 18`) ||
		strings.Contains(item.GroundingNote, "target/shape mismatch") {
		t.Fatalf("absent token must keep the absence wording: %q", item.GroundingNote)
	}
}
