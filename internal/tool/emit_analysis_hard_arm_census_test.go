package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// emit_analysis_hard_arm_census_test.go — V4-4 structural tripwire
// (colleague_merge_audit §40.22 ③, §40.47 fold-in A1/A4; templates:
// hard_arm_mutable_carrier_census, diagram_identity_authority_census).
//
// Rule: an analyze-stage hard gate may judge only the INTERNAL CONSISTENCY
// of what the model declared (or the schema shape of a declared carrier); it
// may never judge the COMPLETENESS of what the model did not declare, because
// analysis does not read file bodies and such facts are often legitimately
// unknown there. Every `return types.ToolResult{Success: false}` inside
// (*EmitAnalysis).Execute is attributed by DATA FLOW:
//   - identifiers and selector chains (`x`, `x.f.g`) resolve lexically to a
//     variable object; a field write `x.f = e` flows into the field object
//     only, so `val.Warnings = append(...)` never reaches `val.RejectReason`;
//   - a variable's producers are the outermost non-neutral callees of EVERY
//     assignment to it on any path (flow-insensitive union), transitively
//     through plain identifier / selector-chain copies; call ARGUMENTS are
//     inputs, not copies (`validateX(rm)` is produced by validateX, not by
//     everything that ever flowed into rm); `q := &x` aliases x;
//   - a compound non-call RHS (`ok := len(xs) < 2`, `flag := !other`) carries
//     its own printed expression as a producer, so an inline judgement can
//     never hide behind the registered producer of a variable it reads;
//   - a frame's keys are its Init/Cond callees plus the producers of every
//     variable its Cond reads; when the Cond calls nothing and reads either
//     no produced variable or more than one variable, the printed condition
//     is itself a key (the condition is then the judge: `ctx == nil`, a
//     two-field contradiction).
//
// A site carries producers (union of the keys of every enclosing non-else
// frame — all must be registered, in any class) and judges (keys of the
// innermost frame not made of registered value carriers only — at least one
// must be a judging-class row). Transparency is registration-driven: a frame
// is skipped only when every key in it is a registered value_carrier (an
// escape flag, a normalizer, a decoder whose result some arm compares) —
// never by syntactic shape — so an unregistered validating call, a
// reassignment chain, a reused identifier name, a bare-boolean guard, or a
// field write is red wherever it is nested. A completeness_exempt row is
// allowed only with a ruling citation.
//
// Frames follow the flow resolver's totality (§40.47 fold-in round five):
// EVERY branching statement opens one — `if` (Init+Cond), each `switch` /
// type-switch case (Init+Tag+case exprs / asserted subject), each `select`
// comm clause, `for` (Init+Cond, or `<unconditional for>`), and `range` (the
// ranged expression) — so a reject nested under a non-if judge can never
// inherit only the enclosing registered frame's keys. `else` branches and
// `default` clauses are skipped frames (their condition did not hold).
//
// Sites are bound by data flow over the return value, not by the literal
// spelling: every `types.ToolResult` composite literal whose Success does not
// PROVABLY resolve to `true` (the literal `false`, a missing Success key, a
// variable that is not always-true, any computed value) is a site, and every
// `return` in Execute whose first result is not a composite literal (a
// variable, a dereference, a helper call — the signature makes it a
// ToolResult) is a site whose result producers join its producer set and
// stand in as judges when no frame encloses it. A reject built through a
// helper or carried through a variable is therefore red until its producer
// is registered.

type emitAnalysisHardArm struct {
	// class ∈ schema_shape (a declared carrier fails its own typed shape or
	// verbatim provenance), internal_consistency (two declared fields
	// contradict each other, including a declared shape whose mandatory
	// companion carrier is absent), host_precondition (the host context, not
	// the model, is unusable), completeness_exempt (judges undeclared
	// content — allowed only with a ruling citation), value_carrier (a
	// decoder / normalizer / escape flag whose result some arm compares; it
	// never rejects on its own, so a frame made only of carriers is
	// transparent and a site judged only by carriers is red).
	class  string
	ruling string
}

var emitAnalysisHardArmClasses = map[string]bool{
	"schema_shape": true, "internal_consistency": true, "host_precondition": true, "completeness_exempt": true, "value_carrier": true,
}

var emitAnalysisHardArmJudgingClasses = map[string]bool{
	"schema_shape": true, "internal_consistency": true, "host_precondition": true, "completeness_exempt": true,
}

// emitAnalysisHardArmRegistry is the single declared table, keyed by the
// producer of the reject (see emitAnalysisFrameKeys). Unregistered producers
// and stale rows are both red.
var emitAnalysisHardArmRegistry = map[string]emitAnalysisHardArm{
	// ---- judges -----------------------------------------------------------
	"ctx == nil || ctx.Mutable == nil":                          {"host_precondition", "no writable Mutable — the tool cannot deposit a model at all"},
	"types.IsREPLControlInput":                                  {"host_precondition", "the request itself is a local REPL control input, not a code question"},
	"decodeStrictNormalizedToolParams":                          {"schema_shape", "strict decode of the params payload (unknown/misplaced/malformed fields); its failure result is returned through a variable and pinned by strict_decode tests"},
	"missingEmitAnalysisRequiredTopLevelFields":                 {"schema_shape", "presence of runtime-required top-level carriers"},
	"p.CompletenessObligation == nil":                           {"schema_shape", "presence-required typed decision (r193)"},
	`scenario == types.ScenarioChitchat && chitchatReply == ""`: {"internal_consistency", "CHATFIX-1: declared chitchat scenario without its reply"},
	"validateAnalysisInput":                                     {"schema_shape", "declared keyword/entity roster floor (advisory today; RejectReason reserved)"},
	"rejectDegenerateClassification":                            {"schema_shape", "declared analysis intent with an empty keyword+entity roster"},
	"parsePredicates":                                           {"schema_shape", "v4 predicates carrier must be present and explicit"},
	"parseHistorySelectionProfile":                              {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseDiagnosticProfile":                                    {"schema_shape", "typed carrier shape"},
	"parseExternalObservationPolicy":                            {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseCurrentSourceExplanationProfile":                      {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseConversationReferenceProfile":                         {"schema_shape", "typed carrier shape"},
	"parseSourceScopeProfile":                                   {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseChangeImpactProfile":                                  {"schema_shape", "typed carrier shape"},
	"validateConfidenceRange":                                   {"schema_shape", "confidence scalars in [0,1]"},
	"parsePredicateAxis":                                        {"schema_shape", "closed enum"},
	"parseRequestedAnswerDimensions":                            {"schema_shape", "typed carrier shape (presence fields, confidence, role enum)"},
	"requiredDiagramRequestedDimension":                         {"internal_consistency", "declared required diagram dimension without its diagram_hint companion carrier"},
	"parseDiagramHint":                                          {"schema_shape", "typed carrier shape + verbatim participant provenance (eaa1a5b53 unanchored roster)"},
	"parseAnswerExclusionPolicy":                                {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseAnswerVisibilityProfile":                              {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseSourceInventoryProfile":                               {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseAnswerRoleProfile":                                    {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseErrorGranularityProfile":                              {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseRuntimeArtifactValueProfile":                          {"schema_shape", "typed carrier shape inside a runtime-artifact turn"},
	"trimNonEmptyStrings":                                       {"schema_shape", "MERGE-AUDIT T6-2 census of the independent runtime profile carriers"},
	"validateRuntimeQuestionProfileConsistency":                 {"internal_consistency", "runtime question profile vs declared intent/scenario/predicates/dimensions"},
	"validateRuntimePerformanceCallChainScopeConsistency":       {"internal_consistency", "runtime performance scope vs declared call-chain shape"},
	"shouldDropInvalidOptionalFieldValueProfile":                {"schema_shape", "field_value_profile carrier shape (soft outside the lanes that need it)"},
	"parseEnumerationBoundary":                                  {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseCompletenessObligation":                               {"schema_shape", "typed carrier shape + verbatim provenance"},
	"parseQuestionBuckets":                                      {"schema_shape", "typed carrier shape + verbatim label provenance"},
	"validateExactTargets":                                      {"schema_shape", "exact_targets must be verbatim in the current request"},
	"reconcileSourceCallChainAxis":                              {"internal_consistency", "declared question_kind vs predicate_axis"},
	"reconcileRuntimeSelectionProfile":                          {"internal_consistency", "runtime_selection_profile vs call_chain_endpoints"},
	"validateCallChainEndpointWireShape":                        {"internal_consistency", "endpoint carrier vs declared call-chain shape"},
	"validateCallChainRuntimeSelectionDeclaration":              {"internal_consistency", "runtime selection declaration vs endpoint profile"},
	"validateSourceCallChainEndpointDeclaration":                {"internal_consistency", "declared source call_chain shape requires its endpoint companion carrier"},
	"validateSelfConsistencyDetailed":                           {"internal_consistency", "intent/scenario/kind/predicates/diagnostic/axis/subject contradictions"},
	"validateRuntimeArtifactCallChainConsistency":               {"internal_consistency", "call-chain kind vs declared runtime targets"},
	`len(exactTargets) > 1 && predicates.HasPerMemberTable && (strings.EqualFold(strings.TrimSpace(kind), "config_mapping") || scenario == types.ScenarioConfigTrace) && len(exactContextRoles) == 0`: {"internal_consistency", "declared multi-key per-member config table requires its exact_context_roles companion carrier"},
	"validateRequiredDiagramEmptyParticipantSlate":     {"internal_consistency", "required diagram + cross-component predicate vs an explicitly empty slate"},
	"validateRequiredFlowDiagramParticipantProvenance": {"internal_consistency", "relation_scope_quote / participant rows / entity roster contradictions (V4-1 §40.34, V4-3 §40.21)"},
	"validateRequiredFileDimensionContradictions":      {"internal_consistency", "V4-4 §40.22: owner ∧ navigation-only on one file; index outside the declared dimension set — completeness arms retired"},
	"validateCallChainRequestedDimensionRoles":         {"internal_consistency", "call-chain kind vs declared dimension roles"},
	"validateAuxiliaryPrincipalExclusionConflict":      {"internal_consistency", "source scope vs exclusion policy"},
	// ---- value carriers (never a judge; transparent only when alone) ------
	"emitAnalysisObservationOnlyRuntimeArtifact":                {"value_carrier", "escape flag: observation-only runtime-artifact turn relaxes exact_targets provenance"},
	"parseFieldValueProfile":                                    {"value_carrier", "field_value_profile decoder; its error is judged by shouldDropInvalidOptionalFieldValueProfile"},
	"normalizeScenario":                                         {"value_carrier", "scenario enum normalizer feeding the chitchat and config-table judges"},
	"normalizeRuntimeArtifactScalarIntent":                      {"value_carrier", "intent/scenario normalizer for scalar runtime-artifact turns"},
	`scenario == types.ScenarioChitchat && chitchatReply != ""`: {"value_carrier", "typed chitchat exemption flag (chitchatExempt)"},
	"normalizeQuestionKind":                                     {"value_carrier", "question_kind normalizer feeding the config-table judge"},
	"normalizeRoleBindingScalarShape":                           {"value_carrier", "predicates normalizer (role-binding scalar shape)"},
	"normalizeDiagnosticMirrorSignals":                          {"value_carrier", "predicates normalizer (diagnostic mirror signals)"},
	"reconcileSetValuedCountPredicates":                         {"value_carrier", "predicates normalizer (set-valued count)"},
	"reconcileSetValuedRoleLocatePredicates":                    {"value_carrier", "predicates normalizer (set-valued role locate)"},
	"reconcileSourceCallChainKindFromEndpointProfile":           {"value_carrier", "predicates normalizer (source call-chain kind from endpoint profile)"},
	"types.MentionedEntitiesFromRawRequest":                     {"value_carrier", "exact_targets fallback carrier inside an observation-only runtime turn"},
	"types.ExactResolutionTargets":                              {"value_carrier", "exact_targets inference carrier from typed resolution predicates"},
	"sanitizeExactContextRoles":                                 {"value_carrier", "exact_context_roles sanitizer whose result the config-table judge counts"},
}

type emitAnalysisHardArmSite struct {
	position  string
	judges    []string // keys of the innermost non-transparent frame
	producers []string // keys of every enclosing non-else frame (data flow)
	condition string   // printed innermost condition, for diagnostics
}

// ---- lexical resolver: identifier / selector chain → object → producers ----

type emitAnalysisFlowObj struct {
	name      string
	producers []string                        // outermost callees of assignments (+ compound-expression texts)
	copies    []*emitAnalysisFlowObj          // RHS was a plain identifier / selector chain
	fields    map[string]*emitAnalysisFlowObj // `x.f = e` writes land here
	parent    *emitAnalysisFlowObj
	boolTrue  bool // some assignment was the literal `true`
	boolFalse bool // some assignment was the literal `false`
	boolOther bool // some assignment was neither a bool literal nor a plain copy
}

// alwaysTrueBool reports whether every value this object can carry is the
// literal `true` — the only case a `Success:` binding is provably a success
// result. Any `false`, any computed assignment, any producer, or a total
// absence of information keeps the site conservative (a potential reject).
func (o *emitAnalysisFlowObj) alwaysTrueBool(seen map[*emitAnalysisFlowObj]bool) bool {
	if seen[o] {
		return true
	}
	seen[o] = true
	if o.boolFalse || o.boolOther || len(o.producers) > 0 {
		return false
	}
	if !o.boolTrue && len(o.copies) == 0 {
		return false
	}
	for _, c := range o.copies {
		if !c.alwaysTrueBool(seen) {
			return false
		}
	}
	return true
}

func (o *emitAnalysisFlowObj) field(name string) *emitAnalysisFlowObj {
	if o.fields == nil {
		o.fields = map[string]*emitAnalysisFlowObj{}
	}
	f, ok := o.fields[name]
	if !ok {
		f = &emitAnalysisFlowObj{name: o.name + "." + name, parent: o}
		o.fields[name] = f
	}
	return f
}

// producersOf collects the transitive producer set: the object's own
// producers, its copies (fully), its fields (a bare read sees every field
// write), and the whole-object producers of its ancestors (a field read sees
// what produced the enclosing value, not its sibling fields).
func (o *emitAnalysisFlowObj) producersOf(seen map[*emitAnalysisFlowObj]bool, out map[string]bool) {
	o.collect(seen, out, true)
	for a := o.parent; a != nil; a = a.parent {
		a.collect(seen, out, false)
	}
}

func (o *emitAnalysisFlowObj) collect(seen map[*emitAnalysisFlowObj]bool, out map[string]bool, withFields bool) {
	if o == nil || seen[o] {
		return
	}
	seen[o] = true
	for _, p := range o.producers {
		out[p] = true
	}
	for _, c := range o.copies {
		c.producersOf(seen, out)
	}
	if withFields {
		for _, f := range o.fields {
			f.collect(seen, out, true)
		}
	}
}

type emitAnalysisFlowScope struct {
	vars   map[string]*emitAnalysisFlowObj
	parent *emitAnalysisFlowScope
}

func (s *emitAnalysisFlowScope) lookup(name string) *emitAnalysisFlowObj {
	for sc := s; sc != nil; sc = sc.parent {
		if o, ok := sc.vars[name]; ok {
			return o
		}
	}
	return nil
}

type emitAnalysisFlowResolver struct {
	fset  *token.FileSet
	refs  map[ast.Node]*emitAnalysisFlowObj // every read of a variable / selector chain
	scope *emitAnalysisFlowScope
}

func (r *emitAnalysisFlowResolver) push() {
	r.scope = &emitAnalysisFlowScope{vars: map[string]*emitAnalysisFlowObj{}, parent: r.scope}
}
func (r *emitAnalysisFlowResolver) pop() { r.scope = r.scope.parent }

func (r *emitAnalysisFlowResolver) define(id *ast.Ident) *emitAnalysisFlowObj {
	if id.Name == "_" {
		return nil
	}
	o := &emitAnalysisFlowObj{name: id.Name}
	r.scope.vars[id.Name] = o
	return o
}

// chain resolves `x` / `x.f.g` to its object; anything else (a package
// selector such as types.ScenarioGeneric, a call, an index) is not a chain.
func (r *emitAnalysisFlowResolver) chain(e ast.Expr) *emitAnalysisFlowObj {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return r.chain(x.X)
	case *ast.Ident:
		return r.scope.lookup(x.Name)
	case *ast.SelectorExpr:
		if base := r.chain(x.X); base != nil {
			return base.field(x.Sel.Name)
		}
	}
	return nil
}

func emitAnalysisNeutralCallee(callee string) bool {
	return strings.HasPrefix(callee, "strings.") || callee == "len" || callee == "cap" || callee == "append" || callee == "make"
}

// record adds the producers of rhs to o: a plain chain is a copy; otherwise
// every outermost non-neutral callee (arguments are inputs, not copies),
// every chain read outside call arguments is a copy, and a compound
// expression (a comparison, a boolean operator, a negation) with no callee
// contributes its printed text — an inline judgement is its own producer.
// Literals, package constants, composite and function literals carry nothing.
func (r *emitAnalysisFlowResolver) record(o *emitAnalysisFlowObj, rhs ast.Expr) {
	if id, ok := rhs.(*ast.Ident); ok {
		switch id.Name {
		case "true":
			o.boolTrue = true
			return
		case "false":
			o.boolFalse = true
			return
		}
	}
	if src := r.chain(rhs); src != nil {
		if src != o {
			o.copies = append(o.copies, src)
		}
		return
	}
	callees := 0
	compound := false
	var walk func(e ast.Node)
	walk = func(e ast.Node) {
		switch x := e.(type) {
		case nil:
		case *ast.ParenExpr:
			walk(x.X)
		case *ast.FuncLit, *ast.CompositeLit, *ast.BasicLit:
		case *ast.CallExpr:
			c := emitAnalysisCallee(x)
			if c != "" && !emitAnalysisNeutralCallee(c) {
				callees++
				o.producers = append(o.producers, c)
				return
			}
			for _, a := range x.Args {
				walk(a)
			}
		case *ast.Ident, *ast.SelectorExpr:
			if src := r.chain(x.(ast.Expr)); src != nil && src != o {
				o.copies = append(o.copies, src)
			} else if sel, ok := x.(*ast.SelectorExpr); ok {
				walk(sel.X)
			}
		case *ast.BinaryExpr:
			compound = true
			walk(x.X)
			walk(x.Y)
		case *ast.UnaryExpr:
			if x.Op == token.NOT {
				compound = true
			}
			walk(x.X)
		case *ast.StarExpr:
			walk(x.X)
		case *ast.IndexExpr:
			walk(x.X)
			walk(x.Index)
		case *ast.SliceExpr:
			walk(x.X)
		case *ast.TypeAssertExpr:
			walk(x.X)
		case *ast.KeyValueExpr:
			walk(x.Value)
		default:
			ast.Inspect(e, func(n ast.Node) bool {
				if n == e {
					return true
				}
				walk(n)
				return false
			})
		}
	}
	walk(rhs)
	if compound && callees == 0 {
		o.producers = append(o.producers, emitAnalysisCondText(r.fset, rhs))
	}
	o.boolOther = true
}

func (r *emitAnalysisFlowResolver) assign(lhs, rhs []ast.Expr, define bool) {
	for i, l := range lhs {
		var o *emitAnalysisFlowObj
		if id, ok := l.(*ast.Ident); ok {
			if define {
				if existing, ok := r.scope.vars[id.Name]; ok {
					o = existing
				} else if len(rhs) == len(lhs) {
					// `q := &x` aliases x: writes through q are writes to x.
					if u, ok := rhs[i].(*ast.UnaryExpr); ok && u.Op == token.AND {
						if target := r.chain(u.X); target != nil {
							r.scope.vars[id.Name] = target
							continue
						}
					}
					o = r.define(id)
				} else {
					o = r.define(id)
				}
			} else {
				o = r.scope.lookup(id.Name)
			}
		} else {
			o = r.chain(l)
			if o == nil {
				// `xs[i] = e` / `*p = e`: a write into the container.
				switch x := l.(type) {
				case *ast.IndexExpr:
					o = r.chain(x.X)
				case *ast.StarExpr:
					o = r.chain(x.X)
				}
			}
		}
		if o == nil {
			continue
		}
		switch {
		case len(rhs) == len(lhs):
			r.record(o, rhs[i])
		case len(rhs) == 1:
			r.record(o, rhs[0])
		}
	}
}

// expr registers every variable / selector-chain read inside e in refs.
func (r *emitAnalysisFlowResolver) expr(e ast.Expr) {
	if e == nil {
		return
	}
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			r.funcLit(x)
			return false
		case *ast.Ident, *ast.SelectorExpr:
			if o := r.chain(x.(ast.Expr)); o != nil {
				r.refs[n] = o
				return false
			}
			if sel, ok := x.(*ast.SelectorExpr); ok {
				r.expr(sel.X)
			}
			return false
		case *ast.KeyValueExpr:
			r.expr(x.Value)
			return false
		}
		return true
	})
}

func (r *emitAnalysisFlowResolver) funcLit(f *ast.FuncLit) {
	r.push()
	r.fields(f.Type)
	r.block(f.Body.List)
	r.pop()
}

func (r *emitAnalysisFlowResolver) fields(ft *ast.FuncType) {
	for _, field := range ft.Params.List {
		for _, n := range field.Names {
			r.define(n)
		}
	}
	if ft.Results != nil {
		for _, field := range ft.Results.List {
			for _, n := range field.Names {
				r.define(n)
			}
		}
	}
}

func (r *emitAnalysisFlowResolver) block(list []ast.Stmt) {
	for _, s := range list {
		r.stmt(s)
	}
}

func (r *emitAnalysisFlowResolver) stmt(s ast.Stmt) {
	switch x := s.(type) {
	case nil:
	case *ast.AssignStmt:
		for _, e := range x.Rhs {
			r.expr(e)
		}
		if x.Tok != token.DEFINE {
			for _, l := range x.Lhs {
				r.expr(l)
			}
		}
		r.assign(x.Lhs, x.Rhs, x.Tok == token.DEFINE)
	case *ast.DeclStmt:
		gd, ok := x.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				r.expr(v)
			}
			for i, n := range vs.Names {
				if o := r.define(n); o != nil && i < len(vs.Values) {
					r.record(o, vs.Values[i])
				}
			}
		}
	case *ast.ExprStmt:
		r.expr(x.X)
	case *ast.IncDecStmt:
		r.expr(x.X)
	case *ast.SendStmt:
		r.expr(x.Chan)
		r.expr(x.Value)
	case *ast.ReturnStmt:
		for _, e := range x.Results {
			r.expr(e)
		}
	case *ast.DeferStmt:
		r.expr(x.Call)
	case *ast.GoStmt:
		r.expr(x.Call)
	case *ast.BlockStmt:
		r.push()
		r.block(x.List)
		r.pop()
	case *ast.IfStmt:
		r.push()
		r.stmt(x.Init)
		r.expr(x.Cond)
		r.stmt(x.Body)
		r.stmt(x.Else)
		r.pop()
	case *ast.ForStmt:
		r.push()
		r.stmt(x.Init)
		r.expr(x.Cond)
		r.stmt(x.Post)
		r.stmt(x.Body)
		r.pop()
	case *ast.RangeStmt:
		r.push()
		r.expr(x.X)
		src := r.chain(x.X)
		for _, e := range []ast.Expr{x.Key, x.Value} {
			if id, ok := e.(*ast.Ident); ok && x.Tok == token.DEFINE {
				if o := r.define(id); o != nil && src != nil {
					o.copies = append(o.copies, src)
				}
			} else {
				r.expr(e)
			}
		}
		r.stmt(x.Body)
		r.pop()
	case *ast.SwitchStmt:
		r.push()
		r.stmt(x.Init)
		r.expr(x.Tag)
		for _, c := range x.Body.List {
			cc := c.(*ast.CaseClause)
			r.push()
			for _, e := range cc.List {
				r.expr(e)
			}
			r.block(cc.Body)
			r.pop()
		}
		r.pop()
	case *ast.TypeSwitchStmt:
		r.push()
		r.stmt(x.Init)
		var guard *ast.Ident
		var guarded ast.Expr
		switch a := x.Assign.(type) {
		case *ast.AssignStmt:
			for _, e := range a.Rhs {
				r.expr(e)
			}
			if id, ok := a.Lhs[0].(*ast.Ident); ok {
				guard = id
				if ta, ok := a.Rhs[0].(*ast.TypeAssertExpr); ok {
					guarded = ta.X
				}
			}
		case *ast.ExprStmt:
			r.expr(a.X)
		}
		for _, c := range x.Body.List {
			cc := c.(*ast.CaseClause)
			r.push()
			if guard != nil {
				if o := r.define(&ast.Ident{Name: guard.Name, NamePos: cc.Pos()}); o != nil && guarded != nil {
					r.record(o, guarded)
				}
			}
			for _, e := range cc.List {
				r.expr(e)
			}
			r.block(cc.Body)
			r.pop()
		}
		r.pop()
	case *ast.SelectStmt:
		for _, c := range x.Body.List {
			cc := c.(*ast.CommClause)
			r.push()
			r.stmt(cc.Comm)
			r.block(cc.Body)
			r.pop()
		}
	case *ast.LabeledStmt:
		r.stmt(x.Stmt)
	case *ast.BranchStmt, *ast.EmptyStmt:
	default:
		panic(fmt.Sprintf("emit_analysis hard-arm census: unhandled statement %T (extend the resolver; a silent skip would defeat the tripwire)", s))
	}
}

func emitAnalysisResolveExecute(fset *token.FileSet, fn *ast.FuncDecl) *emitAnalysisFlowResolver {
	r := &emitAnalysisFlowResolver{fset: fset, refs: map[ast.Node]*emitAnalysisFlowObj{}}
	r.push()
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			for _, n := range f.Names {
				r.define(n)
			}
		}
	}
	r.fields(fn.Type)
	r.block(fn.Body.List)
	return r
}

// ---- frame keys ---------------------------------------------------------

// emitAnalysisFrameKeysFor returns the attribution keys of one frame of any
// statement kind: the outermost non-neutral callees in its Init and condition
// expressions, the transitive producers of every variable / selector chain
// those expressions read, and — when the conditions call nothing and read
// either no produced variable or more than one variable — the fallback text
// (the printed condition; the condition is then the judge itself).
func emitAnalysisFrameKeysFor(r *emitAnalysisFlowResolver, init ast.Stmt, conds []ast.Expr, fallback string) []string {
	keys := map[string]bool{}
	calls := 0
	if initAssign, ok := init.(*ast.AssignStmt); ok {
		for _, rhs := range initAssign.Rhs {
			if c := emitAnalysisCallee(rhs); c != "" && !emitAnalysisNeutralCallee(c) {
				keys[c] = true
				calls++
			}
		}
	}
	objs := map[*emitAnalysisFlowObj]bool{}
	produced := map[string]bool{}
	for _, cond := range conds {
		if cond == nil {
			continue
		}
		ast.Inspect(cond, func(n ast.Node) bool {
			if o := r.refs[n]; o != nil {
				objs[o] = true
				o.producersOf(map[*emitAnalysisFlowObj]bool{}, produced)
				return false
			}
			switch x := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CallExpr:
				if c := emitAnalysisCallee(x); c != "" && !emitAnalysisNeutralCallee(c) {
					keys[c] = true
					calls++
					return false
				}
			}
			return true
		})
	}
	for p := range produced {
		keys[p] = true
	}
	if calls == 0 && (len(objs) != 1 || len(produced) == 0) {
		keys[fallback] = true
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func emitAnalysisFrameKeys(r *emitAnalysisFlowResolver, frame *ast.IfStmt) []string {
	return emitAnalysisFrameKeysFor(r, frame.Init, []ast.Expr{frame.Cond}, emitAnalysisCondText(r.fset, frame.Cond))
}

// emitAnalysisResultProducers collects the data-flow producers of a returned
// non-literal result: transitive producers of every variable / selector chain
// it reads plus every outermost non-neutral callee. A result whose flow
// resolves to nothing is opaque and carries its printed text, so it can never
// pass silently.
func emitAnalysisResultProducers(r *emitAnalysisFlowResolver, e ast.Expr) []string {
	produced := map[string]bool{}
	ast.Inspect(e, func(n ast.Node) bool {
		if o := r.refs[n]; o != nil {
			o.producersOf(map[*emitAnalysisFlowObj]bool{}, produced)
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if c := emitAnalysisCallee(x); c != "" && !emitAnalysisNeutralCallee(c) {
				produced[c] = true
				return false
			}
		}
		return true
	})
	if len(produced) == 0 {
		produced["<opaque return "+emitAnalysisCondText(r.fset, e)+">"] = true
	}
	out := make([]string, 0, len(produced))
	for p := range produced {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func emitAnalysisCondText(fset *token.FileSet, e ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, fset, e)
	return strings.Join(strings.Fields(b.String()), " ")
}

// emitAnalysisHardArmCensus attributes every `return types.ToolResult{...
// Success: false ...}` inside (*EmitAnalysis).Execute. carriers is the set
// of keys registered as value_carrier (the only transparency lane).
func emitAnalysisHardArmCensus(src string, carriers map[string]bool) ([]emitAnalysisHardArmSite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "emit_analysis.go", src, 0)
	if err != nil {
		return nil, err
	}
	var execute *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "Execute" && emitAnalysisReceiver(fn) {
			execute = fn
		}
	}
	if execute == nil {
		return nil, errEmitAnalysisExecuteNotFound
	}
	r := emitAnalysisResolveExecute(fset, execute)
	type frame struct {
		keys []string
		cond string
		skip bool // else branch / default clause: this frame's condition did not hold
	}
	var stack []frame
	var sites []emitAnalysisHardArmSite
	depth := 0 // function-literal nesting: their returns are not Execute's
	addSite := func(n ast.Node, resultProducers []string) {
		site := emitAnalysisHardArmSite{position: fset.Position(n.Pos()).String()}
		producers := map[string]bool{}
		for _, p := range resultProducers {
			producers[p] = true
		}
		for i := len(stack) - 1; i >= 0; i-- {
			fr := stack[i]
			if fr.skip {
				continue
			}
			for _, k := range fr.keys {
				producers[k] = true
			}
			if site.judges != nil {
				continue
			}
			transparent := len(fr.keys) > 0
			for _, k := range fr.keys {
				transparent = transparent && carriers[k]
			}
			if transparent {
				continue
			}
			site.judges = fr.keys
			site.condition = fr.cond
		}
		if site.judges == nil {
			if len(resultProducers) > 0 {
				site.judges = resultProducers // an unframed return: its producer made the decision
			} else {
				site.judges = []string{"<no enclosing frame>"}
			}
		}
		for k := range producers {
			site.producers = append(site.producers, k)
		}
		sort.Strings(site.producers)
		sites = append(sites, site)
	}
	push := func(keys []string, cond string, skip bool) {
		stack = append(stack, frame{keys: keys, cond: cond, skip: skip})
	}
	pop := func() { stack = stack[:len(stack)-1] }
	stmtText := func(s ast.Stmt) string {
		var b strings.Builder
		_ = printer.Fprint(&b, fset, s)
		return strings.Join(strings.Fields(b.String()), " ")
	}
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			depth++
			ast.Inspect(node.Body, visit)
			depth--
			return false
		case *ast.IfStmt:
			push(emitAnalysisFrameKeys(r, node), emitAnalysisCondText(fset, node.Cond), false)
			if node.Init != nil {
				ast.Inspect(node.Init, visit)
			}
			ast.Inspect(node.Cond, visit)
			ast.Inspect(node.Body, visit)
			if node.Else != nil {
				stack[len(stack)-1].skip = true
				ast.Inspect(node.Else, visit)
			}
			pop()
			return false
		case *ast.SwitchStmt:
			if node.Init != nil {
				ast.Inspect(node.Init, visit)
			}
			if node.Tag != nil {
				ast.Inspect(node.Tag, visit)
			}
			for _, c := range node.Body.List {
				cc := c.(*ast.CaseClause)
				conds := append([]ast.Expr{}, cc.List...)
				if node.Tag != nil {
					conds = append(conds, node.Tag)
				}
				text := "default"
				if len(cc.List) > 0 {
					parts := make([]string, 0, len(cc.List)+1)
					if node.Tag != nil {
						parts = append(parts, emitAnalysisCondText(fset, node.Tag)+" ==")
					}
					for _, e := range cc.List {
						parts = append(parts, emitAnalysisCondText(fset, e))
					}
					text = "case " + strings.Join(parts, " ")
				}
				push(emitAnalysisFrameKeysFor(r, node.Init, conds, text), text, len(cc.List) == 0)
				for _, e := range cc.List {
					ast.Inspect(e, visit)
				}
				for _, s := range cc.Body {
					ast.Inspect(s, visit)
				}
				pop()
			}
			return false
		case *ast.TypeSwitchStmt:
			var subj ast.Expr
			switch a := node.Assign.(type) {
			case *ast.AssignStmt:
				if ta, ok := a.Rhs[0].(*ast.TypeAssertExpr); ok {
					subj = ta.X
				}
			case *ast.ExprStmt:
				if ta, ok := a.X.(*ast.TypeAssertExpr); ok {
					subj = ta.X
				}
			}
			if node.Init != nil {
				ast.Inspect(node.Init, visit)
			}
			text := "switch " + emitAnalysisCondText(fset, subj) + ".(type)"
			for _, c := range node.Body.List {
				cc := c.(*ast.CaseClause)
				push(emitAnalysisFrameKeysFor(r, node.Init, []ast.Expr{subj}, text), text, len(cc.List) == 0)
				for _, s := range cc.Body {
					ast.Inspect(s, visit)
				}
				pop()
			}
			return false
		case *ast.SelectStmt:
			for _, c := range node.Body.List {
				cc := c.(*ast.CommClause)
				var conds []ast.Expr
				text := "select default"
				switch comm := cc.Comm.(type) {
				case *ast.AssignStmt:
					conds = comm.Rhs
					text = stmtText(comm)
				case *ast.ExprStmt:
					conds = []ast.Expr{comm.X}
					text = stmtText(comm)
				case *ast.SendStmt:
					conds = []ast.Expr{comm.Chan, comm.Value}
					text = stmtText(comm)
				}
				push(emitAnalysisFrameKeysFor(r, nil, conds, text), text, cc.Comm == nil)
				if cc.Comm != nil {
					ast.Inspect(cc.Comm, visit)
				}
				for _, s := range cc.Body {
					ast.Inspect(s, visit)
				}
				pop()
			}
			return false
		case *ast.ForStmt:
			text := "<unconditional for>"
			var conds []ast.Expr
			if node.Cond != nil {
				conds = []ast.Expr{node.Cond}
				text = emitAnalysisCondText(fset, node.Cond)
			}
			if node.Init != nil {
				ast.Inspect(node.Init, visit)
			}
			if node.Cond != nil {
				ast.Inspect(node.Cond, visit)
			}
			push(emitAnalysisFrameKeysFor(r, node.Init, conds, text), text, false)
			ast.Inspect(node.Body, visit)
			if node.Post != nil {
				ast.Inspect(node.Post, visit)
			}
			pop()
			return false
		case *ast.RangeStmt:
			text := "range " + emitAnalysisCondText(fset, node.X)
			ast.Inspect(node.X, visit)
			push(emitAnalysisFrameKeysFor(r, nil, []ast.Expr{node.X}, text), text, false)
			ast.Inspect(node.Body, visit)
			pop()
			return false
		case *ast.ReturnStmt:
			if depth > 0 || len(node.Results) == 0 {
				return true
			}
			if _, ok := node.Results[0].(*ast.CompositeLit); ok {
				return true // judged at the literal below
			}
			// Execute's signature makes the first result a types.ToolResult:
			// a non-literal return is a site bound by its result's data flow.
			addSite(node, emitAnalysisResultProducers(r, node.Results[0]))
			return true
		case *ast.CompositeLit:
			if !emitAnalysisToolResultMayFail(r, node) {
				return true
			}
			addSite(node, nil)
		}
		return true
	}
	ast.Inspect(execute.Body, visit)
	return sites, nil
}

func emitAnalysisCallee(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			return x.Name + "." + f.Sel.Name
		}
	}
	return ""
}

// emitAnalysisToolResultMayFail reports whether a types.ToolResult composite
// literal can carry Success=false: the literal `false`, a missing Success key
// (the zero value), a positional literal, or any Success binding that does
// not provably resolve to the literal `true` through the flow resolver.
func emitAnalysisToolResultMayFail(r *emitAnalysisFlowResolver, lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ToolResult" {
		return false
	}
	if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "types" {
		return false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Success" {
			continue
		}
		return !emitAnalysisResolvesAlwaysTrue(r, kv.Value)
	}
	return true
}

func emitAnalysisResolvesAlwaysTrue(r *emitAnalysisFlowResolver, v ast.Expr) bool {
	switch x := v.(type) {
	case *ast.ParenExpr:
		return emitAnalysisResolvesAlwaysTrue(r, x.X)
	case *ast.Ident:
		switch x.Name {
		case "true":
			return true
		case "false":
			return false
		}
		if o := r.refs[ast.Node(x)]; o != nil {
			return o.alwaysTrueBool(map[*emitAnalysisFlowObj]bool{})
		}
	}
	return false
}

func emitAnalysisHardArmCarriers() map[string]bool {
	carriers := map[string]bool{}
	for key, arm := range emitAnalysisHardArmRegistry {
		if arm.class == "value_carrier" {
			carriers[key] = true
		}
	}
	return carriers
}

// emitAnalysisHardArmProblems classifies every site against the registry:
// an unregistered producer anywhere in the site's data flow, or a judge
// frame with no judging-class row, is a problem.
func emitAnalysisHardArmProblems(sites []emitAnalysisHardArmSite) (problems []string, seen map[string]bool) {
	seen = map[string]bool{}
	for _, site := range sites {
		for _, key := range site.producers {
			seen[key] = true
			if _, ok := emitAnalysisHardArmRegistry[key]; !ok {
				problems = append(problems, site.position+" unregistered producer="+key+" (cond: "+site.condition+")")
			}
		}
		judged := false
		for _, key := range site.judges {
			judged = judged || emitAnalysisHardArmJudgingClasses[emitAnalysisHardArmRegistry[key].class]
		}
		if !judged {
			problems = append(problems, site.position+" has no judging-class producer in its judge frame "+fmt.Sprint(site.judges)+" (cond: "+site.condition+")")
		}
	}
	sort.Strings(problems)
	return problems, seen
}

func TestEmitAnalysisHardArmCensus(t *testing.T) {
	for producer, arm := range emitAnalysisHardArmRegistry {
		if !emitAnalysisHardArmClasses[arm.class] {
			t.Fatalf("registry row %q has unknown class %q", producer, arm.class)
		}
		if strings.TrimSpace(arm.ruling) == "" {
			t.Fatalf("registry row %q must state what it judges", producer)
		}
		if arm.class == "completeness_exempt" && !strings.Contains(arm.ruling, "§") {
			t.Fatalf("registry row %q judges undeclared content and must cite a ruling section (§40.22 ③)", producer)
		}
	}
	src, err := os.ReadFile("emit_analysis.go")
	if err != nil {
		t.Fatal(err)
	}
	carriers := emitAnalysisHardArmCarriers()
	sites, err := emitAnalysisHardArmCensus(string(src), carriers)
	if err != nil {
		t.Fatalf("census parse failed (a silent green would defeat the tripwire): %v", err)
	}
	if len(sites) < 45 {
		t.Fatalf("census attributed only %d hard-reject sites — it lost its subject", len(sites))
	}
	problems, seen := emitAnalysisHardArmProblems(sites)
	if len(problems) > 0 {
		t.Fatalf("hard reject(s) in (*EmitAnalysis).Execute are not fully classified in emitAnalysisHardArmRegistry — classify each producer as schema_shape / internal_consistency / host_precondition / value_carrier, or completeness_exempt WITH a ruling citation (§40.22 ③):\n  %s",
			strings.Join(problems, "\n  "))
	}
	for producer := range emitAnalysisHardArmRegistry {
		if !seen[producer] {
			t.Fatalf("stale registry row %q (no hard reject flows through it) — prune it", producer)
		}
	}

	// Self-red: an unregistered validating arm must be reported in every
	// shape the data-flow rule claims to cover. Each mutation is inserted
	// before the deposit site (the plain shape) or nested inside a registered
	// frame (the guarded shapes); a non-empty `judge` must additionally be
	// named as a judge of the inserted reject, a non-empty `producer` must be
	// named inside some reported problem line.
	deposit := "	ctx.Mutable.SetRequestModel(rm)\n"
	reject := "return types.ToolResult{ToolName: t.Name(), Success: false, Summary: \"foo\"}, nil\n"
	nested := func(body string) string {
		return "	if conflict := validateAuxiliaryPrincipalExclusionConflict(rm); conflict == \"\" {\n" + body + "	}\n"
	}
	const fooJudge = "validateFooUnregisteredArm"
	type shape struct {
		insert   string
		judge    string // expected among some site's judges ("" = skip)
		producer string // expected inside some problem line ("" = skip)
	}
	shapes := map[string]shape{
		"direct init arm": {"	if issue := validateFooUnregisteredArm(rm); issue != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"bare-boolean guard nested in a registered frame":      {nested("		okBar := validateFooUnregisteredArm(rm)\n		if !okBar {\n			" + reject + "		}\n"), fooJudge, ""},
		"init bare-boolean guard nested in a registered frame": {nested("		if okBar := validateFooUnregisteredArm(rm); !okBar {\n			" + reject + "		}\n"), fooJudge, ""},
		"reassignment chain":                           {"	chain := validateAuxiliaryPrincipalExclusionConflict(rm)\n	if chain == \"\" {\n		chain = validateFooUnregisteredArm(rm)\n	}\n	if chain != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"multi-assigned identifier (conflict)":         {"	conflict := validateFooUnregisteredArm(rm)\n	if conflict != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"multi-assigned identifier (issue)":            {"	issue := validateFooUnregisteredArm(rm)\n	if issue != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"identifier copy":                              {"	verdict := validateFooUnregisteredArm(rm)\n	alias := verdict\n	if alias != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"guard under a registered escape carrier":      {"	if !artifactOnlyRuntime {\n		if issue := validateFooUnregisteredArm(rm); issue != \"\" {\n			" + reject + "		}\n	}\n", fooJudge, ""},
		"field write on a registered carrier":          {"	val.RejectReason = validateFooUnregisteredArm(rm)\n	if val.RejectReason != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"field write through a pointer alias":          {"	alias := &val\n	alias.RejectReason = validateFooUnregisteredArm(rm)\n	if val.RejectReason != \"\" {\n		" + reject + "	}\n", fooJudge, ""},
		"negated copy of an unregistered verdict":      {"	okBar := validateFooUnregisteredArm(rm) == \"\"\n	notOK := !okBar\n	if notOK {\n		" + reject + "	}\n", fooJudge, ""},
		"inline judgement with no call":                {"	hidden := len(rm.AnalyzerHints.RequiredFileHints) < 2\n	if hidden {\n		" + reject + "	}\n", "", ""},
		"inline judgement beside a registered verdict": {nested("		if len(rm.AnalyzerHints.RequiredFileHints) < 2 {\n			" + reject + "		}\n"), "", ""},
		// §40.47 fold-in round five: non-if frames nested inside a registered
		// frame, and rejects not spelled as a Success:false literal.
		"switch arm nested in a registered frame":                 {nested("		switch {\n		case validateFooUnregisteredArm(rm) != \"\":\n			" + reject + "		}\n"), fooJudge, ""},
		"switch-init arm nested in a registered frame":            {nested("		switch issue := validateFooUnregisteredArm(rm); {\n		case issue != \"\":\n			" + reject + "		}\n"), fooJudge, ""},
		"type-switch arm nested in a registered frame":            {nested("		switch validateFooUnregisteredArm(rm).(type) {\n		case string:\n			" + reject + "		}\n"), fooJudge, ""},
		"for-cond arm nested in a registered frame":               {nested("		for validateFooUnregisteredArm(rm) != \"\" {\n			" + reject + "		}\n"), fooJudge, ""},
		"range arm nested in a registered frame":                  {nested("		for _, issue := range validateFooUnregisteredArm(rm) {\n			_ = issue\n			" + reject + "		}\n"), fooJudge, ""},
		"select arm nested in a registered frame":                 {nested("		select {\n		case issue := <-validateFooUnregisteredArmCh(rm):\n			_ = issue\n			" + reject + "		}\n"), "validateFooUnregisteredArmCh", ""},
		"helper-built reject nested in a registered frame":        {nested("		return emitAnalysisRejectFoo(rm, raw), nil\n"), "", "emitAnalysisRejectFoo"},
		"reject carried through a variable return":                {"	failure := buildFooFailureUnregistered(rm)\n	if issue := validateFooUnregisteredArm(rm); issue != \"\" {\n		return failure, nil\n	}\n", fooJudge, "buildFooFailureUnregistered"},
		"Success bound to a variable under an unregistered judge": {"	if issue := validateFooUnregisteredArm(rm); issue != \"\" {\n		rejected := issue != \"\"\n		return types.ToolResult{ToolName: t.Name(), Success: rejected, Summary: issue}, nil\n	}\n", fooJudge, ""},
	}
	for name, sh := range shapes {
		mutated := strings.Replace(string(src), deposit, sh.insert+deposit, 1)
		if mutated == string(src) {
			t.Fatalf("self-red %s: deposit site not found", name)
		}
		sites, err := emitAnalysisHardArmCensus(mutated, carriers)
		if err != nil {
			t.Fatalf("self-red %s: %v", name, err)
		}
		problems, _ := emitAnalysisHardArmProblems(sites)
		if len(problems) == 0 {
			t.Fatalf("self-red %s: census stayed green with an unclassified arm", name)
		}
		if sh.producer != "" {
			named := false
			for _, p := range problems {
				named = named || strings.Contains(p, sh.producer)
			}
			if !named {
				t.Fatalf("self-red %s: census must name %s as an unregistered producer of the inserted reject; problems=%v", name, sh.producer, problems)
			}
		}
		if sh.judge == "" {
			continue
		}
		found := false
		for _, site := range sites {
			for _, key := range site.judges {
				found = found || key == sh.judge
			}
		}
		if !found {
			t.Fatalf("self-red %s: census must attribute the inserted arm to %s as a judge; problems=%v", name, sh.judge, problems)
		}
	}
	// Self-red: a registered value carrier cannot hide a nested reject with
	// no judge at all (a bare `if !flag { reject }` under a carrier only).
	mutated := strings.Replace(string(src), deposit, "	if !artifactOnlyRuntime {\n		"+reject+"	}\n"+deposit, 1)
	sites, err = emitAnalysisHardArmCensus(mutated, carriers)
	if err != nil {
		t.Fatal(err)
	}
	judgeless := false
	for _, site := range sites {
		judged := false
		for _, key := range site.judges {
			judged = judged || emitAnalysisHardArmJudgingClasses[emitAnalysisHardArmRegistry[key].class]
		}
		judgeless = judgeless || !judged
	}
	if !judgeless {
		t.Fatal("self-red: a reject guarded only by a registered value carrier must be reported as judgeless")
	}
	// Self-green control: the same insertion with a REGISTERED judge must
	// not be reported, so the self-reds above fail for the right reason.
	control := strings.Replace(string(src), deposit, nested("		if issue := validateAuxiliaryPrincipalExclusionConflict(rm); issue != \"\" {\n			"+reject+"		}\n")+deposit, 1)
	sites, err = emitAnalysisHardArmCensus(control, carriers)
	if err != nil {
		t.Fatal(err)
	}
	if problems, _ := emitAnalysisHardArmProblems(sites); len(problems) > 0 {
		t.Fatalf("control: a registered nested arm must stay green, got %v", problems)
	}
}
