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

// G6-debt #0 (colleague_merge_audit §40.59 合流复核收编): the definition-
// shaped refusal applies before any callee tier. A line whose callee token
// is preceded by a declaration form — keyword callables (fn/func/function/
// fun/def), type declarations (class/struct/data class/…), template<…> and
// return-type-then-name C/C++ signatures — is never a call site, and the
// type-parameter list a generic declaration spells between the name and
// its `(` does not change that. Each generic shape is pinned next to its
// non-generic twin, and the repair note names the declaration form.
//
// EVOLUTION RECORD: red on 8a1e5d695 — every generic shape grounded
// (callTargetWithTypeArguments accepted `name<…>(` while the definition
// regexes only knew `name(`), Kotlin `fun <T> parse(` and the tuple-struct
// twin were pre-existing holes; green once the declaration forms accept an
// optional type-parameter list and the call-site refusal reads the type
// declaration forms too.
func TestGenericDefinitionLinesNeverGroundAsCallSites(t *testing.T) {
	cases := []struct {
		name, line, anchor, form string
	}{
		{"rust fn", "fn parse<T>(s: &str) -> T {", "parse", `"fn" declaration`},
		{"rust fn twin", "fn parse(s: &str) -> T {", "parse", `"fn" declaration`},
		{"rust pub fn bounded", "pub fn parse<T: FromStr>(s: &str) -> T {", "parse", `"fn" declaration`},
		{"cpp template specialisation", "template<> int parse<int>(const std::string& s) {", "parse", `"template<...>" declaration`},
		{"cpp template twin", "template<class T> T parse(const std::string& s) {", "parse", `"template<...>" declaration`},
		{"cpp return-type signature", "int parse<int>(const std::string& s) {", "parse", "C-style signature with the return type before the name"},
		{"cpp return-type twin", "int parse(const std::string& s) {", "parse", "C-style signature with the return type before the name"},
		{"swift func", "func decode<T>(x: T) -> T {", "decode", `"func" declaration`},
		{"swift func twin", "func decode(x: T) -> T {", "decode", `"func" declaration`},
		{"cangjie func", "func parse<T>(s: String): T {", "parse", `"func" declaration`},
		{"cangjie func twin", "func parse(s: String): T {", "parse", `"func" declaration`},
		{"go func type parameters", "func parse[T any](s string) T {", "parse", `"func" declaration`},
		{"typescript function", "function parse<T>(s: string): T {", "parse", `"function" declaration`},
		{"typescript function twin", "function parse(s: string): T {", "parse", `"function" declaration`},
		{"typescript export function", "export function parse<T>(s: string): T {", "parse", `"function" declaration`},
		{"typescript class method", "parse<T>(s: string): T {", "parse", "method signature"},
		{"typescript class method twin", "parse(s: string): T {", "parse", "method signature"},
		{"kotlin fun with leading type parameters", "fun <T> parse(s: String): T {", "parse", `"fun" declaration`},
		{"kotlin fun twin", "fun parse(s: String): T {", "parse", `"fun" declaration`},
		{"kotlin class", "class Parser<T>(val x: T) {", "Parser", `"class" declaration`},
		{"kotlin class twin", "class Parser(val x: T) {", "Parser", `"class" declaration`},
		{"kotlin data class", "data class Box<T>(val v: T)", "Box", `"data class" declaration`},
		{"kotlin data class twin", "data class Box(val v: T)", "Box", `"data class" declaration`},
		{"rust tuple struct", "struct Wrapper<T>(T);", "Wrapper", `"struct" declaration`},
		{"rust tuple struct twin", "struct Wrapper(T);", "Wrapper", `"struct" declaration`},
		{"rust pub tuple struct", "pub struct Wrapper<T>(T);", "Wrapper", `"struct" declaration`},
		{"python class bases", "class Parser(Base):", "Base", `"class" declaration`},
		{"java record", "record Point<T>(T x, T y) {}", "Point", `"record" declaration`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingUngrounded {
				t.Fatalf("%q anchor %q is a declaration, never a call site: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
			if !strings.Contains(item.GroundingNote, "cites a definition-shaped source line") ||
				!strings.Contains(item.GroundingNote, tc.form) {
				t.Fatalf("%q: the repair note must name the declaration form %q: %q", tc.line, tc.form, item.GroundingNote)
			}
		})
	}
}

// The widened declaration forms must not swallow generic CALL lines: the
// B1554 positives stay grounded when they are spelled at statement start
// without an assignment or return keyword in front — including the
// Kotlin / Swift / TypeScript generic call with a trailing lambda,
// closure or arrow argument (`name<…>(…) {`, `name<…>(() => {`), which
// spells the same bytes as a body-only generic method and is a genuine
// call statement (§40.59 收编复核再收编 #0).
//
// EVOLUTION RECORD: red on 381f36cc9 — methodDefinitionLineRe had gained
// an optional type-parameter list, so all six trailing-lambda shapes were
// refused with the false note "cites a definition-shaped source line
// (method signature …)"; green (and green on 8a1e5d695) once the generic
// method form keys on the `): ReturnType` annotation instead.
func TestGenericCallStatementsStayGroundedAfterDeclarationWidening(t *testing.T) {
	cases := []struct {
		name, line, anchor string
	}{
		{"cpp statement call", "std::make_unique<ConsoleSink>();", "make_unique"},
		{"kotlin statement call", "parse<Int>(text)", "parse"},
		{"typescript statement call", "parse<number>(text);", "parse"},
		{"rust statement call", "parse::<u32>(text)?;", "parse"},
		{"cangjie statement call", "repo.load<Item>(key)", "load"},
		{"kotlin trailing lambda", "launch<Unit>(Dispatchers.IO) {", "launch"},
		{"kotlin trailing lambda with body", "remember<Int>(key) { compute() }", "remember"},
		{"kotlin nullable nested type argument", "produceState<Result<T>?>(null, id) {", "produceState"},
		{"swift trailing closure", "Task<Void, Never>(priority: .high) {", "Task"},
		{"swift trailing closure with capture", "withCheckedContinuation<Int, Never>(isolation: nil) { cont in", "withCheckedContinuation"},
		{"typescript arrow argument", "useEffect<void>(() => {", "useEffect"},
		{"kotlin receiver-qualified trailing lambda", "scope.launch<Unit>(Dispatchers.IO) {", "launch"},
		{"kotlin assigned trailing lambda", "val x = remember<Int>(key) { compute() }", "remember"},
		{"swift returned trailing closure", "return Task<Void, Never>(priority: .high) {", "Task"},
		{"typescript receiver-qualified arrow argument", "React.useEffect<void>(() => {", "useEffect"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingGrounded {
				t.Fatalf("%q anchor %q is a call site: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
		})
	}
}

// A generic METHOD definition is refused on the one token a trailing-lambda
// call never carries: the return-type annotation right after the parameter
// list (`): T {`, `): T =>`, `): Promise<T> {`). A generic method spelled
// without a return annotation (`parse<T>(s: string) {`) is byte-identical
// to a Kotlin / Swift trailing-lambda call and keeps the base behaviour
// (grounded) — disclosed residual, pinned so the trade-off is visible.
func TestGenericMethodDefinitionRefusalKeysOnReturnTypeAnnotation(t *testing.T) {
	refused := []struct {
		name, line, anchor string
	}{
		{"typescript generic method", "parse<T>(s: string): T {", "parse"},
		{"typescript generic method arrow body", "parse<T>(s: string): T =>", "parse"},
		{"typescript async generic method", "async fetch<T>(id: number): Promise<T> {", "fetch"},
		{"typescript exported generic method", "public static build<T>(s: string): Box<T> {", "build"},
		{"typescript generic interface member", "parse<T>(s: string): T;", "parse"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingUngrounded {
				t.Fatalf("%q anchor %q is a generic method definition, never a call site: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
			if !strings.Contains(item.GroundingNote, "method signature (name<...>(...) followed by a return type)") {
				t.Fatalf("%q: the repair note must name the generic method form: %q", tc.line, item.GroundingNote)
			}
		})
	}
	t.Run("generic method without a return annotation keeps the base behaviour", func(t *testing.T) {
		item, report := groundCallAnchor("parse<T>(s: string) {", "parse", nil)
		if report.Status != types.GroundingGrounded {
			t.Fatalf("`name<T>(…) {` is indistinguishable from a trailing-lambda call by regex; base behaviour (grounded) must stand: report=%+v item=%+v", report, item)
		}
	})
}

// §40.59 收编复核再收编 #3: declaration shapes the regex tier accepted as
// call sites beyond the generic-parity class, each extended precisely per
// shape — a leading type-parameter list before the return type (Java), a
// receiver-qualified `fun` name path (Kotlin extensions, nullable receiver
// included), the Dart `async` / `async*` / `sync*` body opener, a scoped
// out-of-line definition with a return type before `Class<T>::name`, the
// constructor member-initialiser list `) : ident(` after a scoped name, a
// `template<…>` prefix with nested default arguments, and two levels of
// nesting in the shared type-parameter spelling. Each shape is pinned
// beside a genuine-call twin that must stay grounded. Forms the regex
// cannot discriminate from calls stay on the base behaviour and are
// pinned as disclosed residuals.
//
// EVOLUTION RECORD: red on 381f36cc9 and on 8a1e5d695 — every refused
// shape below was Status=grounded tier=line_text on both trees (the
// hard gate accepting a definition line as a call site); green once the
// declaration forms carry the per-shape extensions.
func TestDeclarationFormsBeyondGenericParityNeverGroundAsCallSites(t *testing.T) {
	refused := []struct {
		name, line, anchor, form string
	}{
		{"java leading type parameters", "public static <T> List<T> parse(String s) {", "parse", "C-style signature with the return type before the name"},
		{"java bare leading type parameters", "<T> T parse(String s) {", "parse", "C-style signature with the return type before the name"},
		{"java bounded leading type parameters", "static <T extends Comparable<T>> T parse(String s) {", "parse", "C-style signature with the return type before the name"},
		{"java leading type parameters with throws", "public <T> T parse(String s) throws ParseException {", "parse", "C-style signature with the return type before the name"},
		{"kotlin generic receiver extension", "fun <T> List<T>.second(): T {", "second", `"fun" declaration`},
		{"kotlin generic receiver", "fun List<Int>.second(): Int {", "second", `"fun" declaration`},
		{"kotlin generic receiver expression body", "fun <T> Flow<T>.throttle(ms: Long): Flow<T> = channelFlow {", "throttle", `"fun" declaration`},
		{"kotlin nullable receiver", "fun String?.orEmpty(): String {", "orEmpty", `"fun" declaration`},
		{"dart async main", "Future<void> main() async {", "main", "C-style signature with the return type before the name"},
		{"dart async generic", "Future<T> fetch<T>(int id) async {", "fetch", "C-style signature with the return type before the name"},
		{"dart async void", "void main() async {", "main", "C-style signature with the return type before the name"},
		{"dart async generator", "Stream<int> ticks() async* {", "ticks", "C-style signature with the return type before the name"},
		{"dart sync generator", "Iterable<int> ticks() sync* {", "ticks", "C-style signature with the return type before the name"},
		{"cpp out-of-line template member", "template<typename T> T Parser<T>::parse(const std::string& s) {", "parse", `"template<...>" declaration`},
		{"cpp out-of-line generic class member", "int Parser<T>::parse(const std::string& s) const {", "parse", "C-style signature with the return type before the name"},
		{"cpp constructor initialiser generic", "Parser<T>::Parser(int x) : x_(x) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp constructor initialiser", "Parser::Parser(int x) : x_(x) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp constructor initialiser template prefix", "template<typename T> Parser<T>::Parser(int x) : x_(x) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp constructor initialiser base class", "Derived::Derived(int x) : Base<int>(x) {", "Derived", "constructor definition with a member-initialiser list"},
		{"cpp constructor brace initialiser", "Parser::Parser(int x) : x_{x} {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp default template argument", "template <typename T, typename U = std::vector<T>> T parse(const U& u) {", "parse", `"template<...>" declaration`},
		{"rust two-level bound", "fn parse<T: Into<Vec<u8>>>(s: T) {", "parse", `"fn" declaration`},
		{"cpp two-level specialisation", "template<> int parse<std::map<int, std::vector<int>>>(const std::string& s) {", "parse", `"template<...>" declaration`},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingUngrounded {
				t.Fatalf("%q anchor %q is a declaration, never a call site: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
			if !strings.Contains(item.GroundingNote, "cites a definition-shaped source line") ||
				!strings.Contains(item.GroundingNote, tc.form) {
				t.Fatalf("%q: the repair note must name the declaration form %q: %q", tc.line, tc.form, item.GroundingNote)
			}
		})
	}
	grounded := []struct {
		name, line, anchor string
	}{
		{"kotlin receiver call", "list.second()", "second"},
		{"kotlin generic receiver call", "items.throttle<Int>(ms)", "throttle"},
		{"kotlin nullable-safe call", "name?.orEmpty()", "orEmpty"},
		{"cpp two-level template call", "auto map = build<std::map<std::string, std::vector<int>>>(kind);", "build"},
		{"cpp two-level template statement call", "build<std::map<std::string, std::vector<int>>>(kind);", "build"},
		{"rust two-level turbofish", "let v = parse::<Vec<Vec<u8>>>(s);", "parse"},
		{"cpp scoped call statement", "Parser::parse(s);", "parse"},
		{"cpp scoped template call statement", "Parser<int>::parse(s);", "parse"},
		{"dart call", "fetch(id);", "fetch"},
		{"dart awaited call", "await fetch<int>(id);", "fetch"},
		{"ruby scoped block call", "Foo::bar(x) { |y| y }", "bar"},
		{"java generic call statement", "Util.<String>parse(text);", "parse"},
		// Disclosed residuals: shapes no regex can tell from a call keep
		// the base behaviour (grounded).
		{"residual: out-of-line constructor without initialiser list", "Parser::Parser(int x) {", "Parser"},
		{"residual: three-level nested type-parameter list", "fn parse<T: Into<Vec<Vec<u8>>>>(s: T) {", "parse"},
	}
	for _, tc := range grounded {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundCallAnchor(tc.line, tc.anchor, nil)
			if report.Status != types.GroundingGrounded {
				t.Fatalf("%q anchor %q must keep grounding: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
		})
	}
}
