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

// emitAnalysisFrameKeys returns the attribution keys of one `if` frame: the
// outermost non-neutral callees in its Init and Cond, the transitive
// producers of every variable / selector chain its Cond reads, and — when
// the Cond calls nothing and reads either no produced variable or more than
// one variable — the printed condition text itself.
func emitAnalysisFrameKeys(r *emitAnalysisFlowResolver, frame *ast.IfStmt) []string {
	keys := map[string]bool{}
	calls := 0
	if init, ok := frame.Init.(*ast.AssignStmt); ok {
		for _, rhs := range init.Rhs {
			if c := emitAnalysisCallee(rhs); c != "" && !emitAnalysisNeutralCallee(c) {
				keys[c] = true
				calls++
			}
		}
	}
	objs := map[*emitAnalysisFlowObj]bool{}
	produced := map[string]bool{}
	ast.Inspect(frame.Cond, func(n ast.Node) bool {
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
	for p := range produced {
		keys[p] = true
	}
	if calls == 0 && (len(objs) != 1 || len(produced) == 0) {
		keys[emitAnalysisCondText(r.fset, frame.Cond)] = true
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
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
		stmt   *ast.IfStmt
		inElse bool
		keys   []string
	}
	var stack []frame
	var sites []emitAnalysisHardArmSite
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			stack = append(stack, frame{stmt: node, keys: emitAnalysisFrameKeys(r, node)})
			if node.Init != nil {
				ast.Inspect(node.Init, visit)
			}
			ast.Inspect(node.Cond, visit)
			ast.Inspect(node.Body, visit)
			if node.Else != nil {
				stack[len(stack)-1].inElse = true
				ast.Inspect(node.Else, visit)
			}
			stack = stack[:len(stack)-1]
			return false
		case *ast.CompositeLit:
			if !emitAnalysisIsFailedToolResult(node) {
				return true
			}
			site := emitAnalysisHardArmSite{position: fset.Position(node.Pos()).String()}
			producers := map[string]bool{}
			for i := len(stack) - 1; i >= 0; i-- {
				fr := stack[i]
				if fr.inElse {
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
				site.condition = emitAnalysisCondText(fset, fr.stmt.Cond)
			}
			if site.judges == nil {
				site.judges = []string{"<no enclosing if>"}
			}
			for k := range producers {
				site.producers = append(site.producers, k)
			}
			sort.Strings(site.producers)
			sites = append(sites, site)
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

func emitAnalysisIsFailedToolResult(lit *ast.CompositeLit) bool {
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
		value, ok := kv.Value.(*ast.Ident)
		return ok && value.Name == "false"
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
	if len(sites) < 39 {
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
	// frame (the guarded shapes); `attributed` shapes must additionally name
	// validateFooUnregisteredArm as a judge of the inserted reject.
	deposit := "	ctx.Mutable.SetRequestModel(rm)\n"
	reject := "return types.ToolResult{ToolName: t.Name(), Success: false, Summary: \"foo\"}, nil\n"
	nested := func(body string) string {
		return "	if conflict := validateAuxiliaryPrincipalExclusionConflict(rm); conflict == \"\" {\n" + body + "	}\n"
	}
	type shape struct {
		insert     string
		attributed bool
	}
	shapes := map[string]shape{
		"direct init arm": {"	if issue := validateFooUnregisteredArm(rm); issue != \"\" {\n		" + reject + "	}\n", true},
		"bare-boolean guard nested in a registered frame":      {nested("		okBar := validateFooUnregisteredArm(rm)\n		if !okBar {\n			" + reject + "		}\n"), true},
		"init bare-boolean guard nested in a registered frame": {nested("		if okBar := validateFooUnregisteredArm(rm); !okBar {\n			" + reject + "		}\n"), true},
		"reassignment chain":                           {"	chain := validateAuxiliaryPrincipalExclusionConflict(rm)\n	if chain == \"\" {\n		chain = validateFooUnregisteredArm(rm)\n	}\n	if chain != \"\" {\n		" + reject + "	}\n", true},
		"multi-assigned identifier (conflict)":         {"	conflict := validateFooUnregisteredArm(rm)\n	if conflict != \"\" {\n		" + reject + "	}\n", true},
		"multi-assigned identifier (issue)":            {"	issue := validateFooUnregisteredArm(rm)\n	if issue != \"\" {\n		" + reject + "	}\n", true},
		"identifier copy":                              {"	verdict := validateFooUnregisteredArm(rm)\n	alias := verdict\n	if alias != \"\" {\n		" + reject + "	}\n", true},
		"guard under a registered escape carrier":      {"	if !artifactOnlyRuntime {\n		if issue := validateFooUnregisteredArm(rm); issue != \"\" {\n			" + reject + "		}\n	}\n", true},
		"field write on a registered carrier":          {"	val.RejectReason = validateFooUnregisteredArm(rm)\n	if val.RejectReason != \"\" {\n		" + reject + "	}\n", true},
		"field write through a pointer alias":          {"	alias := &val\n	alias.RejectReason = validateFooUnregisteredArm(rm)\n	if val.RejectReason != \"\" {\n		" + reject + "	}\n", true},
		"negated copy of an unregistered verdict":      {"	okBar := validateFooUnregisteredArm(rm) == \"\"\n	notOK := !okBar\n	if notOK {\n		" + reject + "	}\n", true},
		"inline judgement with no call":                {"	hidden := len(rm.AnalyzerHints.RequiredFileHints) < 2\n	if hidden {\n		" + reject + "	}\n", false},
		"inline judgement beside a registered verdict": {nested("		if len(rm.AnalyzerHints.RequiredFileHints) < 2 {\n			" + reject + "		}\n"), false},
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
		if !sh.attributed {
			continue
		}
		found := false
		for _, site := range sites {
			for _, key := range site.judges {
				found = found || key == "validateFooUnregisteredArm"
			}
		}
		if !found {
			t.Fatalf("self-red %s: census must attribute the inserted arm to validateFooUnregisteredArm as a judge; problems=%v", name, problems)
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
