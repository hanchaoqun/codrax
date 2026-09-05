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
// fold-in review round four #7).
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
func TestCommandOperationRequestDisplayCensusSelfRed(t *testing.T) {
	const header = "package repl\n\n"
	method := func(body string) string {
		return header + "func (r *REPL) probe(plan operation.CommandOperationPlan, request, display string) {\n\t" + body + "\n}\n"
	}
	cases := []struct {
		name  string
		src   string
		want  string // substring the census must report; "" = green
		count int    // classified dispatcher sites across all dispatchers
	}{
		{"direct_call_control", method(`r.executeCommandOperationPlan(plan, request, "/approve")`), "", 1},
		{"direct_attempt_control", method(`r.executeCommandOperationPlanAttempt(plan, request, "/approve", 0, nil)`), "", 1},
		{"method_value", method("exec := r.executeCommandOperationPlan\n\texec(plan, \"/approve\", \"/approve\")"), "reference to dispatcher r.executeCommandOperationPlan outside a direct call", 0},
		{"attempt_method_value", method("exec := r.executeCommandOperationPlanAttempt\n\texec(plan, \"/approve\", \"/approve\", 0, nil)"), "reference to dispatcher r.executeCommandOperationPlanAttempt outside a direct call", 0},
		{"receiver_copy", method("self := r\n\tself.executeCommandOperationPlan(plan, \"/approve\", \"/approve\")"), "dispatcher called through self instead of the enclosing *REPL method's receiver", 0},
		{"method_expression", method(`(*REPL).executeCommandOperationPlan(r, plan, "/approve", "/approve")`), "dispatcher called through (*REPL) instead of the enclosing *REPL method's receiver", 0},
		{"parenthesised_callee", method(`(r.executeCommandOperationPlan)(plan, "/approve", "/approve")`), "outside a direct call", 0},
		{"non_method_function", header + "func probe(r *REPL, plan operation.CommandOperationPlan, request string) {\n\tr.executeCommandOperationPlan(plan, \"/approve\", \"/approve\")\n}\n", "dispatcher called through r instead of the enclosing *REPL method's receiver", 0},
		{"other_type_receiver", header + "func (p *other) probe(request string) {\n\tp.dispatch(\"/approve\", \"/approve\")\n}\n", "dispatcher called through p instead of the enclosing *REPL method's receiver", 0},
		{"literal_request_slot", method(`r.executeCommandOperationPlan(plan, "/approve", "/approve")`), `request slot carries "/approve" (display lane`, 1},
		{"swapped_pair", method(`r.executeCommandOperationPlan(plan, display, request)`), "request slot carries display (display lane", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, tc.name+".go", tc.src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			problems, counts := requestDisplayCensusFiles(fset, []*ast.File{f})
			total := 0
			for _, n := range counts {
				total += n
			}
			if total != tc.count {
				t.Fatalf("classified %d site(s) (%v), want %d\n%s", total, counts, tc.count, strings.Join(problems, "\n"))
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
