package repl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// command_operation_request_display_census_selfred_test.go — self-reds for
// the request/display census (colleague_merge_audit §40.52, batch six
// fold-in review round four #7, review round five #4/#5).
//
// EVOLUTION RECORD (review round four #7): on b6f7eeec3 the census only
// looked at calls whose Fun was the literal `r.<dispatcher>` selector and
// its floors were lower bounds. A package copy carrying the four evasion
// shapes below (method value, receiver copy, other receiver, method
// expression — each handing "/approve" in the request slot, the round-three
// #5 shape) beside the live files scanned with counts identical to live
// (executeCommandOperationPlan:3) and zero problems. The rewritten census
// classifies only direct calls on the enclosing *REPL method's receiver,
// reports every other dispatcher reference, and holds site counts to an
// exact roster; each shape is red here.
//
// EVOLUTION RECORD (review round five #5): maybeDispatchCommandOperationFollowup
// (line, display, …) forwards display into two Attempt sites and line into
// the follow-up request text but was not a registered dispatcher, so a
// (display, line) swap at its three call sites in dispatch stayed green
// (a scratch copy with the swap scanned with live counts and zero
// problems). A rebinding of the receiver name inside a *REPL method
// (`r := other`) was a disclosed residual: a dispatcher call on the rebound
// name classified as a direct call. The follow-up dispatcher is registered,
// every receiver rebinding shape is red, and the multi-value producer arm
// resolves against the enclosing receiver ident.
func TestCommandOperationRequestDisplayCensusSelfRed(t *testing.T) {
	const header = "package repl\n\n"
	method := func(body string) string {
		return header + "func (r *REPL) probe(plan operation.CommandOperationPlan, request, display string) {\n\t" + body + "\n}\n"
	}
	lineMethod := func(body string) string {
		return header + "func (r *REPL) probe(line, display string, policy, rawPolicy TurnPolicy) {\n\t" + body + "\n}\n"
	}
	cases := []struct {
		name  string
		src   string
		want  string // substring the census must report; "" = green
		count int    // classified dispatcher sites across all dispatchers
	}{
		{"direct_call_control", method(`r.executeCommandOperationPlan(plan, request, "/approve")`), "", 1},
		{"direct_attempt_control", method(`r.executeCommandOperationPlanAttempt(plan, request, "/approve", commandOperationAttemptState{})`), "", 1},
		{"method_value", method("exec := r.executeCommandOperationPlan\n\texec(plan, \"/approve\", \"/approve\")"), "reference to dispatcher r.executeCommandOperationPlan outside a direct call", 0},
		{"attempt_method_value", method("exec := r.executeCommandOperationPlanAttempt\n\texec(plan, \"/approve\", \"/approve\", commandOperationAttemptState{})"), "reference to dispatcher r.executeCommandOperationPlanAttempt outside a direct call", 0},
		{"receiver_copy", method("self := r\n\tself.executeCommandOperationPlan(plan, \"/approve\", \"/approve\")"), "dispatcher called through self instead of the enclosing *REPL method's receiver", 0},
		{"method_expression", method(`(*REPL).executeCommandOperationPlan(r, plan, "/approve", "/approve")`), "dispatcher called through (*REPL) instead of the enclosing *REPL method's receiver", 0},
		{"parenthesised_callee", method(`(r.executeCommandOperationPlan)(plan, "/approve", "/approve")`), "outside a direct call", 0},
		{"non_method_function", header + "func probe(r *REPL, plan operation.CommandOperationPlan, request string) {\n\tr.executeCommandOperationPlan(plan, \"/approve\", \"/approve\")\n}\n", "dispatcher called through r instead of the enclosing *REPL method's receiver", 0},
		{"other_type_receiver", header + "func (p *other) probe(request string) {\n\tp.dispatch(\"/approve\", \"/approve\")\n}\n", "dispatcher called through p instead of the enclosing *REPL method's receiver", 0},
		{"literal_request_slot", method(`r.executeCommandOperationPlan(plan, "/approve", "/approve")`), `request slot carries "/approve" (display lane`, 1},
		{"swapped_pair", method(`r.executeCommandOperationPlan(plan, display, request)`), "request slot carries display (display lane", 1},
		// review round five #5: the follow-up dispatcher is on the route
		{"followup_direct_control", lineMethod(`r.maybeDispatchCommandOperationFollowup(line, display, policy, rawPolicy)`), "", 1},
		{"followup_swapped_pair", lineMethod(`r.maybeDispatchCommandOperationFollowup(display, line, policy, rawPolicy)`), "request slot carries display (display lane", 1},
		// review round five #5: receiver rebinding shapes, each red on the
		// binding itself (the call on the rebound name is still counted, so
		// the roster arm is unaffected)
		{"receiver_define_rebound", method("r := other\n\tr.executeCommandOperationPlan(plan, request, display)"), "receiver r rebound by a := definition", 1},
		{"receiver_type_switch_rebound", method("switch r := any(plan).(type) {\n\tcase int:\n\t\t_ = r\n\t}\n\tr.executeCommandOperationPlan(plan, request, display)"), "receiver r rebound by a := definition", 1},
		{"receiver_var_rebound", method("var r *REPL\n\tr.executeCommandOperationPlan(plan, request, display)"), "receiver r rebound by a var declaration", 1},
		{"receiver_range_rebound", method("for _, r := range others {\n\t\tr.executeCommandOperationPlan(plan, request, display)\n\t}"), "receiver r rebound by a range definition", 1},
		{"receiver_funclit_param_rebound", method("run := func(r *REPL) {\n\t\tr.executeCommandOperationPlan(plan, request, display)\n\t}\n\trun(r)"), "receiver r rebound by a func-literal parameter", 1},
		{"receiver_funclit_result_rebound", method("pick := func() (r *REPL) {\n\t\treturn nil\n\t}\n\tpick().executeCommandOperationPlan(plan, request, display)"), "receiver r rebound by a func-literal named result", 0},
		{"receiver_plain_assign_not_rebinding", method("r = other\n\tr.executeCommandOperationPlan(plan, request, display)"), "", 1},
		// review round five #5: the multi-value producer resolves on the
		// enclosing receiver ident, whatever it is named
		{"other_receiver_ident_producer", header + "func (q *REPL) probe(prompt string) {\n\tline, display, err := q.readInputPair(prompt)\n\t_ = err\n\tq.dispatch(line, display)\n}\n", "", 1},
		{"other_receiver_ident_foreign_producer", header + "func (q *REPL) probe(prompt string) {\n\tline, display, err := r.readInputPair(prompt)\n\t_ = err\n\tq.dispatch(line, display)\n}\n", "multi-value call r.readInputPair is outside the request/display producer set (not on the enclosing *REPL method's receiver)", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, tc.name+".go", tc.src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			report := requestDisplayCensusFiles(fset, []*ast.File{f})
			problems := report.Problems
			total := 0
			for _, n := range report.Counts {
				total += n
			}
			if total != tc.count {
				t.Fatalf("classified %d site(s) (%v), want %d\n%s", total, report.Counts, tc.count, strings.Join(problems, "\n"))
			}
			if len(report.Sites) != total {
				t.Fatalf("report lists %d site(s) but counted %d — every classified site must be listed", len(report.Sites), total)
			}
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("control shape must be green, got:\n%s", strings.Join(problems, "\n"))
				}
				return
			}
			if len(problems) == 0 {
				t.Fatalf("shape passed the census silently (want a problem containing %q)", tc.want)
			}
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("problems lack %q:\n%s", tc.want, strings.Join(problems, "\n"))
			}
		})
	}
}

// TestCommandOperationRequestDisplayRosterSelfRed pins the exact-roster arm
// in both directions: an added site, a removed site, a dispatcher counted
// but not registered, and a registered dispatcher with no site are each
// red; the exact counts are green.
func TestCommandOperationRequestDisplayRosterSelfRed(t *testing.T) {
	roster := map[string]int{"dispatch": 6, "operationDispatch": 3}
	cases := []struct {
		name   string
		counts map[string]int
		want   string
	}{
		{"exact", map[string]int{"dispatch": 6, "operationDispatch": 3}, ""},
		{"added_site", map[string]int{"dispatch": 7, "operationDispatch": 3}, "7 direct dispatch call site(s) classified, roster registers 6"},
		{"removed_site", map[string]int{"dispatch": 5, "operationDispatch": 3}, "5 direct dispatch call site(s) classified, roster registers 6"},
		{"unregistered_dispatcher", map[string]int{"dispatch": 6, "operationDispatch": 3, "newDispatch": 1}, "newDispatch is classified but not registered"},
		{"registered_without_sites", map[string]int{"dispatch": 6}, "0 direct operationDispatch call site(s) classified, roster registers 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := requestDisplayRosterProblems(tc.counts, roster)
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("exact counts must be green, got %v", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// TestCommandOperationRequestDisplayCensusFailureListsSites pins the red
// output (review round five #4): a legitimately added dispatcher site —
// clean lanes, so not a problem of its own — makes the roster arm red, and
// the failure text names every classified site with its file:line,
// enclosing method, dispatcher and lanes, plus the per-dispatcher counts.
// EVOLUTION RECORD: on 533a939fb the failure printed only the problems
// list while the counts were logged on the green path, so the roster
// message's "(it is classified above)" referred to nothing that was
// printed and the added site had to be found by hand.
func TestCommandOperationRequestDisplayCensusFailureListsSites(t *testing.T) {
	src := "package repl\n\nfunc (r *REPL) probe(line, display string) {\n\tr.dispatch(line, display)\n}\n\nfunc (r *REPL) probeTwo(plan operation.CommandOperationPlan, request string) {\n\tr.executeCommandOperationPlan(plan, request, \"/approve\")\n}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "added_site.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	report := requestDisplayCensusFiles(fset, []*ast.File{f})
	if len(report.Problems) != 0 {
		t.Fatalf("both sites carry clean lanes and must not be problems of their own: %v", report.Problems)
	}
	roster := map[string]int{"dispatch": 0, "executeCommandOperationPlan": 1}
	problems := requestDisplayRosterProblems(report.Counts, roster)
	if len(problems) != 1 || !strings.Contains(problems[0], "1 direct dispatch call site(s) classified, roster registers 0") {
		t.Fatalf("the added site must be red on the roster arm alone, got %v", problems)
	}
	out := requestDisplayCensusFailure(problems, report)
	for _, want := range []string{
		problems[0],
		"every classified site is listed below",
		"classified sites (2), counts per dispatcher map[dispatch:1 executeCommandOperationPlan:1]",
		"added_site.go:4 probe.dispatch: request=line (raw lane: parameter line), display=display (display lane: parameter display)",
		`added_site.go:8 probeTwo.executeCommandOperationPlan: request=request (request lane: parameter request), display="/approve" (display lane: string literal is a display form)`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("failure output lacks %q:\n%s", want, out)
		}
	}
}
