package ground

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ground_definition_shape_scope_test.go — §40.59 收编复核三轮 (batch-six
// fold-in review round three #3/#4) and 收编复核四轮 (#3): the regex-lane
// definition-shaped refusal reads two more precise signals, drops one false
// one, and never buys a refusal with a genuine call statement.
//
//   - A generic method whose parameter list holds a function-typed
//     parameter (`map<U>(fn: (x: T) => U): U[] {`) reaches its own return
//     annotation through a one-level nested parameter atom, so it is
//     refused like its bare twin (generic ≡ bare parity), while every
//     genuine generic call with a function argument stays grounded.
//   - The constructor member-initialiser form spans a default call
//     argument and the function-try-block `try :`, and requires the
//     terminal name to equal its immediate qualifier (`Parser::Parser`) —
//     the spelling C++ reserves for constructors — so a scoped call that
//     continues a ternary (`Foo::bar(x) : Foo::baz(y)`) and a one-line
//     ternary whose condition is a scoped call are call sites again.
//   - The C-family signature keeps its base parameter atom (`[^;{}]*`): a
//     `{` inside the parentheses ends the shape. Round three admitted one
//     brace level so `int parse(Opts o = {}) {` was refused — and with it
//     every semicolon-free call statement `<word> name({…})` (Go `go
//     worker(Job{ID: 1})`, JS `new Vue({ el: '#app' })`, Python `assert
//     check({"a": 1})`, Ruby `render json: build({a: 1})`) on the regex
//     lane, including the graph-less argument-flow and callback-handoff
//     synthesizers. A regex cannot tell that argument from that default,
//     so the atom is reverted and the C-family default-brace-argument
//     definitions are the disclosed residual: accepted, as on the base.
//     The constructor initialiser atom keeps its brace admission: its
//     `) : ident(` tail plus the name agreement are two typed signals no
//     call statement spells, so that definition stays refused without
//     collateral.
//
// This file is self-contained (own probe helper) so the identical pins run
// unchanged against scratch copies of both bases.
//
// EVOLUTION RECORD (round three, run on 480939385 and, in scratch copies,
// on 381f36cc9 and 8a1e5d695):
//   - callback-typed generic methods: red on 480939385 and on 8a1e5d695
//     (Grounded, definition accepted as a call site); green on 381f36cc9
//     only by the over-broad optional-type-parameter method form that also
//     refused genuine trailing-lambda calls.
//   - the function-try-block: red on all three trees (Grounded); the
//     default call argument (constructor) was refused on 480939385 by its
//     unbounded parameter atom and stays refused through the one-level
//     paren admission.
//   - one-line ternary with a scoped-call condition: red on 480939385
//     (Ungrounded with the false "constructor definition" note), green on
//     both bases; the ternary continuation `Foo::bar(x) : Foo::baz(y)` is
//     red on 480939385 and 381f36cc9, green on 8a1e5d695.
//   - genuine generic calls with function arguments: green on 480939385
//     and 8a1e5d695, red on 381f36cc9 (the optional-type-parameter form).
//
// EVOLUTION RECORD (round four, run on live and, in git-archive scratch
// copies, on 480939385 and b6f7eeec3):
//   - the eight brace-argument call statements (GroundItem) and the two
//     graph-less synthesizer pins (argument flows for `go worker(Job{ID:
//     1})`, callback handoff for `go run(handler, Opts{Retry: 1})`): red on
//     b6f7eeec3 (Ungrounded with the false "C-style signature" note; zero
//     flows; no handoff), green on 480939385.
//   - the three default-brace-argument definitions (residual, accepted):
//     red on b6f7eeec3 (refused), green on 480939385.
//   - the base definition shapes without a brace (`int parse(int x) {`,
//     `int Parser::parse(Opts o) const {`, `static int parse(Opts o =
//     Opts()) {`): refused on every tree.
//   - green on live for all of the above.

func groundDefinitionShapeProbe(line, anchor string) (types.EvidenceItem, Report) {
	gc := &Context{LineIndex: map[string]map[int]string{"src/file": {18: line}}}
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "src/file", LineStart: 18, LineEnd: 18,
		Subject: "caller", Predicate: "calls", Object: anchor,
		AnchorKind: types.AnchorCall, AnchorSymbol: anchor,
	}
	report := GroundItem(&item, gc)
	return item, report
}

func TestDefinitionShapeRefusalSpansNestedParametersAndKeysConstructorsOnTheirName(t *testing.T) {
	refused := []struct {
		name, line, anchor, form string
	}{
		{"typescript generic method with a callback parameter", "map<U>(fn: (x: T) => U): U[] {", "map", "method signature (name<...>(...) followed by a return type)"},
		{"typescript generic reduce with a callback parameter", "reduce<U>(fn: (acc: U, x: T) => U, init: U): U {", "reduce", "method signature (name<...>(...) followed by a return type)"},
		{"typescript public generic method with a callback parameter", "public map<U>(fn: (x: T) => U): U[] {", "map", "method signature (name<...>(...) followed by a return type)"},
		{"typescript generic interface member with a callback parameter", "map<U>(fn: (x: T) => U): U[];", "map", "method signature (name<...>(...) followed by a return type)"},
		{"typescript generic method with a parenthesised return type", "pipe<U>(fn: (x: T) => U): (y: T) => U {", "pipe", "method signature (name<...>(...) followed by a return type)"},
		{"cpp constructor with a default call argument", "Parser::Parser(Opts o = Opts()) : o_(o) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp constructor with a default brace argument keeps its two typed signals", "Parser::Parser(Opts o = {}) : o_(o) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp constructor function-try-block", "Parser::Parser(int x) try : x_(x) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp constructor noexcept function-try-block", "Parser::Parser(int x) noexcept try : x_(x) {", "Parser", "constructor definition with a member-initialiser list"},
		{"cpp namespaced constructor initialiser", "ns::Parser::Parser(int x) : x_(x) {", "Parser", "constructor definition with a member-initialiser list"},
		// Base definition shapes (no brace inside the parentheses) stay
		// refused on every tree.
		{"cpp function definition", "int parse(int x) {", "parse", "C-style signature with the return type before the name"},
		{"cpp out-of-line const member definition", "int Parser::parse(Opts o) const {", "parse", "C-style signature with the return type before the name"},
		{"cpp static function with a default call argument", "static int parse(Opts o = Opts()) {", "parse", "C-style signature with the return type before the name"},
		{"cpp constructor initialiser without arguments", "Parser::Parser() : x_(0) {", "Parser", "constructor definition with a member-initialiser list"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundDefinitionShapeProbe(tc.line, tc.anchor)
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
		{"typescript generic call with an arrow argument and a dependency list", "useCallback<Fn>((x) => f(x), [])", "useCallback"},
		{"typescript generic call with a nested call in an arrow argument", "useMemo<number>(() => compute(a), [a])", "useMemo"},
		{"typescript generic call with a nested call argument and arrow", "useEffect<void>(() => { sync(a) }, [a])", "useEffect"},
		{"kotlin generic trailing lambda with a nested call argument", "run<Int>(f(x)) { it }", "run"},
		{"kotlin generic launch with a nested call argument", "launch<Unit>(ctx(a)) {", "launch"},
		{"swift generic trailing closure with a nested call argument", "Task<Void, Never>(priority: pick(a)) {", "Task"},
		{"ruby one-line ternary with a scoped condition, then arm", "Foo::ok(x) ? Foo::bar(x) : Foo::baz(y)", "bar"},
		{"ruby one-line ternary with a scoped condition, else arm", "Foo::ok(x) ? Foo::bar(x) : Foo::baz(y)", "baz"},
		{"php one-line ternary with a scoped condition", "Foo::ok($x) ? Foo::bar($x) : Foo::baz($y);", "bar"},
		{"cpp one-line ternary statement with a scoped condition", "Foo::ok(x) ? Foo::bar(x) : Foo::baz(y);", "baz"},
		{"cpp ternary continuation after a previous-line question mark", "Foo::bar(x) : Foo::baz(y);", "bar"},
		{"cpp ternary continuation, else arm", "Foo::bar(x) : Foo::baz(y);", "baz"},
		{"cpp assigned ternary with a scoped condition", "auto r = Foo::ok(x) ? Foo::bar(x) : Foo::baz(y);", "baz"},
		{"ruby scoped block call", "Foo::bar(x) { |y| y }", "bar"},
		{"ruby scoped do block", "Foo::bar(x) do |y|", "bar"},
		{"cpp constructor initialiser is still refused only for its own class name control", "Foo::bar(x) : baz(y) {", "bar"},
		// Genuine call statements carrying a brace-literal argument in
		// semicolon-free languages: a `{` inside the parentheses ends the
		// C-family shape (§40.59 收编复核四轮 #3).
		{"go statement spawning a worker with a struct literal", "go worker(Job{ID: 1})", "worker"},
		{"go statement with a tab indent and a struct literal", "\tgo worker(Job{ID: 1})", "worker"},
		{"go statement with a callback and a struct literal", "go run(handler, Opts{Retry: 1})", "run"},
		{"javascript export default new with an object literal", "export default new Router({ routes })", "Router"},
		{"javascript new with an object literal", "new Vue({ el: '#app' })", "Vue"},
		{"javascript new with an object literal and a callback", "new Server({ port }, onReady)", "Server"},
		{"python assert over a call with a dict literal", "assert check({\"a\": 1})", "check"},
		{"ruby render with a call carrying a hash literal", "render json: build({a: 1})", "build"},
		// Disclosed residuals: a generic method without a return annotation
		// (`async {` body opener included) is byte-identical to a
		// trailing-lambda call and keeps the base behaviour (grounded).
		{"residual: dart generic method without a return type", "fetch<T>(int id) async {", "fetch"},
		{"residual: typescript generic method with a callback parameter and no return type", "forEach<U>(fn: (x: T) => void) {", "forEach"},
		// Disclosed residual: a default BRACE argument ends the C-family
		// parameter atom, so these definitions keep the base behaviour
		// (grounded) — the price of never refusing the genuine
		// brace-argument calls above. The constructor shape keeps its
		// brace admission (pinned refused below): its two typed signals
		// discriminate it from every call statement.
		{"residual: cpp out-of-line member with a default brace argument", "int Parser::parse(Opts o = {}) const {", "parse"},
		{"residual: cpp function with a default brace argument", "int parse(Opts o = {}) {", "parse"},
	}
	for _, tc := range grounded {
		t.Run(tc.name, func(t *testing.T) {
			item, report := groundDefinitionShapeProbe(tc.line, tc.anchor)
			if report.Status != types.GroundingGrounded {
				t.Fatalf("%q anchor %q is a call site: report=%+v item=%+v", tc.line, tc.anchor, report, item)
			}
		})
	}
	// Disclosed residual, pinned for parity: a ternary continuation whose
	// arms are bare or generic method-shaped calls (`parse<T>(s) :
	// parse<U>(u);`) is refused by the `):` return-annotation signal on
	// every tree — the generic form joins the standing bare-form policy.
	for _, line := range []string{"parse<T>(s) : parse<U>(u);", "parse(s) : parse(u);"} {
		t.Run("residual: method-shaped ternary continuation "+line, func(t *testing.T) {
			_, report := groundDefinitionShapeProbe(line, "parse")
			if report.Status != types.GroundingUngrounded {
				t.Fatalf("%q: the `):` signal refuses the method-shaped ternary continuation on every tree (disclosed): report=%+v", line, report)
			}
		})
	}
}

// The graph-less synthesizers gate on the same definition-shape reading
// and have no tree-sitter lift: a brace-literal call argument must not turn
// a Go statement into a "definition" that yields no argument flow and no
// callback handoff (§40.59 收编复核四轮 #3).
func TestBraceArgumentCallStatementsFeedTheGraphlessSynthesizers(t *testing.T) {
	gc := &Context{LineIndex: map[string]map[int]string{"src/file": {
		18: "go worker(Job{ID: 1})",
		19: "go run(handler, Opts{Retry: 1})",
	}}}
	flows := DetectArgumentFlowsAtLine(gc, "src/file", 18, "worker")
	if len(flows) != 1 || flows[0].Argument != "Job{ID: 1}" || flows[0].Receiver != "worker" {
		t.Fatalf("go worker(Job{ID: 1}): argument flows=%+v, want the one struct-literal argument into worker", flows)
	}
	receiver, argument, ok := DetectCallbackHandoffAtLine(gc, "src/file", 19, "handler")
	if !ok || receiver != "run" || argument != "handler" {
		t.Fatalf("go run(handler, Opts{Retry: 1}): callback handoff=(%q, %q, %t), want handler handed to run", receiver, argument, ok)
	}
}
