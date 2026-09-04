package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// behavior_contract_id_resolution_census_test.go — V5-4 tripwire
// (colleague_merge_audit §40.24 item 3, V5-3 §40.23 item 3, §40.46 C4 + the
// §40.46 合流复核收编 return/alias/copy/named-type/per-expression tightening):
// the behavior-contract id space is resolved through ONE tombstone-aware,
// ledger-aware projection. The census is bound to DATA FLOW, not to the
// spelling of one call site: a value derived from the pre-rebase analyzer
// snapshot (`Request.BehaviorContracts`) is tainted through aliases
// (`x := ir.Request.BehaviorContracts`), `req := ir.Request` carrier
// aliases, range clauses, copy() destinations, call arguments into the
// parameters of censused functions AND callee returns back into the call
// expression (functions and methods, fixpoint over every scanned package),
// and no tainted value may reach a sink. A refs gate's result counts
// whether it is spelled `string` or a named string type, and the gate is
// resolved per id-source EXPRESSION: every id it judges must come from an
// accepted authority — calling an authority for something else resolves
// nothing.
//
//	(a) no non-test file references the identifier WriteBehaviorContractIDs
//	    (the raw id set is unexported in types; this guards a re-export);
//	(b) no Required/HardRequired/PlacementRequiredWriteBehaviorContractIDs
//	    call receives a tainted argument (snapshot, alias, or tainted
//	    parameter);
//	(c) every refs gate in tool (a function whose result is `string` or a
//	    named string type and whose body reads .ContractRefs /
//	    .PlacementRefs) that touches the id-source side — directly, through
//	    a tainted parameter, or through a helper that does — resolves EVERY
//	    id it judges through an accepted authority (an authority call, a
//	    required-id call over authority-derived values, or a clean authority
//	    helper) and never consults WriteAnalysisIR() / the snapshot / a
//	    tainted value / a raw .BehaviorContracts union — per expression, so
//	    an authority called for something else resolves nothing;
//	(d) SupersededBehaviorContracts / SupersededBehaviorContractIDs /
//	    BehaviorContractGeneration are written only by
//	    attachWriteBehaviorContracts — as assignment targets AND as composite
//	    literal keys;
//	(e) the pure projection ProjectWriteBehaviorContractGeneration is called
//	    outside package types only by the generation-0 workflow seed in
//	    writeflow; tool/agent/orchestrator reach the projection through
//	    Mutable.ProjectBehaviorContractGeneration, which supplies the run's
//	    tombstone ledger from state so no consumer can forget it;
//	(f) in the rendering packages (agent, writeflow — which never produce an
//	    IR) a tainted value is never iterated or indexed: the snapshot is only
//	    ever the input of the projection.
//
// A parse error is red; the file-count floor keeps a silently empty scan
// red; the drift floor counts resolution-checked gates, not skipped ones.

var behaviorContractIDAuthorities = map[string]bool{
	"resolveBehaviorContractIDs":              true,
	"ResolveChangePlanBehaviorContractIDs":    true,
	"ProjectBehaviorContractGeneration":       true,
	"ProjectWriteBehaviorContractGeneration":  true,
	"ChangePlanVerificationBehaviorContracts": true,
}

var behaviorContractRequiredIDFuncs = map[string]bool{
	"RequiredWriteBehaviorContractIDs":          true,
	"HardRequiredWriteBehaviorContractIDs":      true,
	"PlacementRequiredWriteBehaviorContractIDs": true,
}

var behaviorContractTombstoneWriterFields = map[string]bool{
	"SupersededBehaviorContracts":   true,
	"SupersededBehaviorContractIDs": true,
	"BehaviorContractGeneration":    true,
}

type behaviorContractCensusFile struct {
	name string
	src  string
	pkg  string // "tool" | "agent" | "writeflow" | "orchestrator"
}

func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

type behaviorContractCensusCall struct {
	callee string
	// pkg is the census package key the callee lives in: the caller's own
	// package for an unqualified call, or the imported package for a
	// `pkg.Callee(...)` call (resolved through the file's import block), so
	// call-argument taint crosses package boundaries.
	pkg  string
	args []ast.Expr
	node *ast.CallExpr
}

// behaviorContractCensusModulePath is the module root of every import the
// census resolves to a scanned package.
const behaviorContractCensusModulePath = "github.com/hanchaoqun/codrax/"

// behaviorContractCensusPackageKey maps a module-relative directory
// ("internal/tool", "internal/writeflow/convention", "cmd") to the census
// package key: the bare name for the four packages the fine-grained rules
// address, the relative path for every other scanned package (unique, never
// colliding with those names).
func behaviorContractCensusPackageKey(relDir string) string {
	switch relDir = filepath.ToSlash(relDir); relDir {
	case "internal/tool", "internal/agent", "internal/writeflow", "internal/orchestrator":
		return filepath.Base(relDir)
	}
	return relDir
}

type behaviorContractCensusFunc struct {
	name, file, pkg string
	decl            *ast.FuncDecl
	params          []string
	// resultTypes holds the declared result type expressions; stringResult
	// is judged after the whole scan so a named string type declared in any
	// scanned file counts (`type rejection string` gates are refs gates).
	resultTypes  []ast.Expr
	namedResults []string
	stringResult bool
	readsRefs    bool
	idSource     bool
	authority    bool
	readsIR      bool
	writesField  bool
	// returnsTainted / returnsReqTainted: some return statement of this
	// function (or method) returns a snapshot-tainted / Request-carrier
	// expression, so every call expression naming it carries that colour.
	returnsTainted    bool
	returnsReqTainted bool
	calls             []behaviorContractCensusCall
	returns           []*ast.ReturnStmt
	tainted           map[string]bool
	// taintedReq marks Request-carrier locals (`req := ir.Request`): a
	// `.BehaviorContracts` selector on a carrier is the snapshot.
	taintedReq map[string]bool
	// §40.43 round-six #8 — the DIRECT-colour lane. A field store used to
	// drop the snapshot colour entirely (the LHS was not an Ident, so the
	// assignment was skipped and `s.c` read back untainted). The store lane
	// is driven by the PRECISE spellings only (directSnapshotExpr): the
	// broad exprTainted rules (callee returns, composite literals,
	// field-of-tainted fall-through) colour many benign values in the
	// rendering packages, and keying a store lane off them turned the
	// census into a hard gate on a noisy signal (repo red line).
	//
	//   - directTainted: identifiers and dotted field paths ("s.c") that
	//     hold a direct copy of the snapshot within this function — by a
	//     store (`s.c = …`) or, §40.43 round-seven #6, by a KEYED composite
	//     literal (`s := stash{c: ir.Request.BehaviorContracts}`), so the
	//     precise lane also follows `t.c = s.c; range t.c`.
	//   - fieldTaint: PACKAGE-shared, keyed "RecvType.field" — receiver
	//     fields some method of that type stashed the snapshot into
	//     (`f.c = f.ir.Request.BehaviorContracts` in one method, iterated
	//     in another).
	//
	// Residual (disclosed, wording corrected §40.43 round-seven #6): a store
	// of a value coloured ONLY through a callee return, a POSITIONAL
	// composite literal, a carrier (`.Request`) stash, or a struct value
	// handed across functions is not tracked by this lane. The read-site
	// sinks bracket only reads of the source expression / the broad-tainted
	// local itself (`range s.c` after `s := stash{…}`, `range x` after
	// `x := contracts(ir)`); a read THROUGH such an untracked store
	// (`t.c = x; range t.c`) is uncoloured and is NOT caught — that is the
	// residual, accepted because driving the store lane off the broad rules
	// would make a hard gate of a noisy signal (repo red line).
	directTainted map[string]bool
	fieldTaint    map[string]bool
	// recvIdent / recvType name the method receiver ("" for free
	// functions) so receiver-field reads resolve through fieldTaint.
	recvIdent string
	recvType  string
	// untrackedStores records field/element stores of tainted values whose
	// base could not be reduced to an identifier — the census fails loud on
	// them instead of guessing.
	untrackedStores []string
	// authDerived marks locals whose value came from an accepted authority
	// (per-expression gate resolution).
	authDerived map[string]bool
	importKey   map[string]string
	lookup      map[string]map[string]*behaviorContractCensusFunc
}

// calleeFunc resolves a call to the censused function it names: an
// unqualified call in the caller's package, a `pkg.Callee(...)` call through
// the file's import block, or — for a method / value receiver — a name-keyed
// fallback in the caller's own package (methods are collected by name, so a
// `f.contracts()` return-taint is not laundered by the receiver spelling).
func (fn *behaviorContractCensusFunc) calleeFunc(call *ast.CallExpr) *behaviorContractCensusFunc {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return fn.lookup[fn.pkg][f.Name]
	case *ast.SelectorExpr:
		if qual, ok := f.X.(*ast.Ident); ok {
			if key, ok := fn.importKey[qual.Name]; ok {
				return fn.lookup[key][f.Sel.Name]
			}
		}
		return fn.lookup[fn.pkg][f.Sel.Name]
	}
	return nil
}

// behaviorContractTaintSanitizers are calls whose RESULT never carries the
// snapshot's id space even when the snapshot is an argument: the projection
// (its result is the active generation) and len().
var behaviorContractTaintSanitizers = map[string]bool{
	"ProjectWriteBehaviorContractGeneration": true,
	"ProjectBehaviorContractGeneration":      true,
	"len":                                    true,
}

// exprTainted reports whether an expression CARRIES a value derived from the
// analyzer snapshot. The judgment is structural value flow — a
// `.BehaviorContracts` selector off a Request carrier, a tainted identifier,
// a field/element/slice of a tainted value, append()/conversions/composite
// literals over tainted values, and calls whose censused callee (function or
// method) returns a tainted value — never string contamination: a
// fmt.Sprintf/Errorf over a tainted value produces prose, not the id space,
// so it does not taint (precise signals for hard gates; the sinks judge the
// id space, not messages that mention it).
func (fn *behaviorContractCensusFunc) exprTainted(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return fn.exprTainted(v.X)
	case *ast.StarExpr:
		return fn.exprTainted(v.X)
	case *ast.UnaryExpr:
		return fn.exprTainted(v.X)
	case *ast.TypeAssertExpr:
		return fn.exprTainted(v.X)
	case *ast.Ident:
		return fn.tainted[v.Name] || fn.directTainted[v.Name]
	case *ast.SelectorExpr:
		if v.Sel.Name == "BehaviorContracts" && fn.exprReqTainted(v.X) {
			return true
		}
		// §40.43 round-six #8: a stashed-into field reads back tainted —
		// the dotted path for a local base, the receiver-typed shared map
		// for a receiver base (cross-method stash).
		if base, ok := v.X.(*ast.Ident); ok {
			if fn.directTainted[base.Name+"."+v.Sel.Name] {
				return true
			}
			if base.Name == fn.recvIdent && fn.fieldTaint[fn.recvType+"."+v.Sel.Name] {
				return true
			}
		}
		// A field of a tainted value carries the taint.
		return fn.exprTainted(v.X)
	case *ast.IndexExpr:
		return fn.exprTainted(v.X)
	case *ast.SliceExpr:
		return fn.exprTainted(v.X)
	case *ast.KeyValueExpr:
		return fn.exprTainted(v.Value)
	case *ast.CompositeLit:
		for _, elt := range v.Elts {
			if fn.exprTainted(elt) {
				return true
			}
		}
	case *ast.CallExpr:
		if behaviorContractTaintSanitizers[calleeName(v)] {
			return false
		}
		if calleeName(v) == "append" {
			for _, arg := range v.Args {
				if fn.exprTainted(arg) {
					return true
				}
			}
			return false
		}
		// A callee's tainted RETURN taints the call expression.
		if callee := fn.calleeFunc(v); callee != nil && callee != fn && callee.returnsTainted {
			return true
		}
		// A conversion (`[]types.WriteBehaviorContract(x)`, `rejection(x)`)
		// carries its operand: a Fun that is not a resolvable callee and
		// not a name at all is a type expression.
		switch v.Fun.(type) {
		case *ast.Ident, *ast.SelectorExpr:
		default:
			if len(v.Args) == 1 {
				return fn.exprTainted(v.Args[0])
			}
		}
	}
	return false
}

// exprReqTainted reports whether an expression carries the analyzer
// snapshot's Request (the carrier whose `.BehaviorContracts` field IS the
// snapshot): any `.Request` selector, a Request-carrier alias, or a call
// whose censused callee returns a carrier.
func (fn *behaviorContractCensusFunc) exprReqTainted(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return fn.exprReqTainted(v.X)
	case *ast.StarExpr:
		return fn.exprReqTainted(v.X)
	case *ast.UnaryExpr:
		return fn.exprReqTainted(v.X)
	case *ast.IndexExpr:
		return fn.exprReqTainted(v.X)
	case *ast.SelectorExpr:
		return v.Sel.Name == "Request"
	case *ast.Ident:
		return fn.taintedReq[v.Name]
	case *ast.CallExpr:
		if behaviorContractTaintSanitizers[calleeName(v)] {
			return false
		}
		if callee := fn.calleeFunc(v); callee != nil && callee != fn && callee.returnsReqTainted {
			return true
		}
	}
	return false
}

// behaviorContractStoreTarget reduces a non-Ident assignment LHS to its
// base identifier and (for a field store) the outermost stored-into field
// name: `s.c` → (s, "c"), `s.c[i]` → (s, "c"), `(*p).f` → (p, "f"),
// `m[k]` → (m, ""). A nil base means the store target is untracked
// (§40.43 round-six #8 fail-loud lane).
func behaviorContractStoreTarget(lhs ast.Expr) (*ast.Ident, string) {
	switch v := lhs.(type) {
	case *ast.ParenExpr:
		return behaviorContractStoreTarget(v.X)
	case *ast.StarExpr:
		return behaviorContractStoreTarget(v.X)
	case *ast.IndexExpr:
		return behaviorContractStoreTarget(v.X)
	case *ast.SelectorExpr:
		base, field := behaviorContractStoreTarget(v.X)
		if field == "" {
			field = v.Sel.Name
		}
		return base, field
	case *ast.Ident:
		return v, ""
	}
	return nil, ""
}

// directSnapshotExpr (§40.43 round-six #8) reports whether an expression IS
// the snapshot in its precise spellings: a `.BehaviorContracts` selector off
// a Request carrier (paren/index/slice unwrapped), or an identifier / dotted
// field path the precise seed already coloured. Deliberately NARROWER than
// exprTainted for the field-STORE rule: the broad rules (callee returns,
// composite literals, field-of-tainted fall-through) colour many benign
// values in the rendering packages, and driving the store lane off them
// turned the census into a noisy hard gate (repo red line: precise signals
// for hard gates). Keyed composite literals whose value IS the snapshot
// colour their field key (colourCompositeLiteralKeys, §40.43 round-seven
// #6). Residual (disclosed): a store of a value coloured ONLY through a
// callee return or a positional composite literal is not tracked by this
// lane; the read-site sinks bracket reads of the coloured value itself, not
// reads through the untracked stored copy (`t.c = x; range t.c` escapes).
func (fn *behaviorContractCensusFunc) directSnapshotExpr(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return fn.directSnapshotExpr(v.X)
	case *ast.IndexExpr:
		return fn.directSnapshotExpr(v.X)
	case *ast.SliceExpr:
		return fn.directSnapshotExpr(v.X)
	case *ast.Ident:
		return fn.directTainted[v.Name]
	case *ast.SelectorExpr:
		if v.Sel.Name == "BehaviorContracts" && fn.exprReqTainted(v.X) {
			return true
		}
		if base, ok := v.X.(*ast.Ident); ok {
			if fn.directTainted[base.Name+"."+v.Sel.Name] {
				return true
			}
			if base.Name == fn.recvIdent && fn.fieldTaint[fn.recvType+"."+v.Sel.Name] {
				return true
			}
		}
		return false
	}
	return false
}

// colourCompositeLiteralKeys (§40.43 round-seven #6) propagates the DIRECT
// colour through a composite literal's field keys: `s := stash{c:
// ir.Request.BehaviorContracts}` (or `&stash{…}` / a `var` spec) colours the
// dotted path "s.c" exactly like the store `s.c = ir.Request.BehaviorContracts`
// does, so a later field copy `t.c = s.c` stays on the precise store lane and
// `range t.c` reads back coloured. Only KEYED elements whose value IS the
// snapshot in its precise spellings (directSnapshotExpr) colour a key —
// positional literals carry no field name without type information and stay
// on the broad lane (disclosed residual). Returns true when a key was newly
// coloured.
func (fn *behaviorContractCensusFunc) colourCompositeLiteralKeys(base string, rhs ast.Expr) bool {
	for {
		switch v := rhs.(type) {
		case *ast.ParenExpr:
			rhs = v.X
			continue
		case *ast.UnaryExpr:
			rhs = v.X
			continue
		}
		break
	}
	lit, ok := rhs.(*ast.CompositeLit)
	if !ok {
		return false
	}
	changed := false
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || !fn.directSnapshotExpr(kv.Value) {
			continue
		}
		path := base + "." + key.Name
		if !fn.directTainted[path] {
			fn.directTainted[path] = true
			changed = true
		}
	}
	return changed
}

// propagateTaintLocally taints locals assigned from tainted expressions
// (both colours: snapshot values and Request carriers), range variables over
// tainted collections, and copy() destinations fed a tainted source, to a
// fixpoint. Returns true when anything changed.
func (fn *behaviorContractCensusFunc) propagateTaintLocally() bool {
	changed := false
	for {
		round := false
		ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range v.Lhs {
					var rhs ast.Expr
					if len(v.Rhs) == len(v.Lhs) {
						rhs = v.Rhs[i]
					} else if len(v.Rhs) == 1 {
						rhs = v.Rhs[0]
					}
					if rhs == nil {
						continue
					}
					id, ok := lhs.(*ast.Ident)
					if !ok {
						// §40.43 round-six #8: a store INTO a field or
						// element used to drop the colour. Track it under
						// the dotted path key ("s.c"; element stores taint
						// the base), and for a RECEIVER field additionally
						// under the receiver-typed package-shared map so a
						// stash read from another method carries too. A
						// store whose base is not an identifier is
						// untracked → fail loud.
						base, field := behaviorContractStoreTarget(lhs)
						if !fn.directSnapshotExpr(rhs) {
							continue
						}
						if base == nil {
							fn.untrackedStores = append(fn.untrackedStores,
								fn.file+": "+fn.name+" stores the snapshot into an untracked target — the census cannot follow it")
							continue
						}
						localKey := base.Name
						if field != "" {
							localKey = base.Name + "." + field
						}
						if !fn.directTainted[localKey] {
							fn.directTainted[localKey] = true
							round = true
						}
						if field != "" && fn.recvIdent != "" && base.Name == fn.recvIdent {
							recvKey := fn.recvType + "." + field
							if !fn.fieldTaint[recvKey] {
								fn.fieldTaint[recvKey] = true
								round = true
							}
						}
						continue
					}
					if !fn.tainted[id.Name] && fn.exprTainted(rhs) {
						fn.tainted[id.Name] = true
						round = true
					}
					if !fn.directTainted[id.Name] && fn.directSnapshotExpr(rhs) {
						fn.directTainted[id.Name] = true
						round = true
					}
					if fn.colourCompositeLiteralKeys(id.Name, rhs) {
						round = true
					}
					if !fn.taintedReq[id.Name] && fn.exprReqTainted(rhs) {
						fn.taintedReq[id.Name] = true
						round = true
					}
				}
			case *ast.ValueSpec:
				for i, name := range v.Names {
					if i >= len(v.Values) {
						continue
					}
					if !fn.tainted[name.Name] && fn.exprTainted(v.Values[i]) {
						fn.tainted[name.Name] = true
						round = true
					}
					if fn.colourCompositeLiteralKeys(name.Name, v.Values[i]) {
						round = true
					}
					if !fn.taintedReq[name.Name] && fn.exprReqTainted(v.Values[i]) {
						fn.taintedReq[name.Name] = true
						round = true
					}
				}
			case *ast.CallExpr:
				// copy(dst, taintedSrc) taints dst.
				if calleeName(v) == "copy" && len(v.Args) == 2 {
					if id, ok := v.Args[0].(*ast.Ident); ok && !fn.tainted[id.Name] && fn.exprTainted(v.Args[1]) {
						fn.tainted[id.Name] = true
						round = true
					}
				}
			case *ast.RangeStmt:
				if !fn.exprTainted(v.X) {
					return true
				}
				for _, e := range []ast.Expr{v.Key, v.Value} {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !fn.tainted[id.Name] {
						fn.tainted[id.Name] = true
						round = true
					}
				}
			}
			return true
		})
		if !round {
			return changed
		}
		changed = true
	}
}

// recomputeReturnTaint refreshes the function's return colours from its
// return statements (func-literal returns are excluded at collection time).
// A bare `return` returns the named results. Returns true when a colour was
// newly set.
func (fn *behaviorContractCensusFunc) recomputeReturnTaint() bool {
	changed := false
	for _, ret := range fn.returns {
		if len(ret.Results) == 0 {
			for _, name := range fn.namedResults {
				if !fn.returnsTainted && fn.tainted[name] {
					fn.returnsTainted = true
					changed = true
				}
				if !fn.returnsReqTainted && fn.taintedReq[name] {
					fn.returnsReqTainted = true
					changed = true
				}
			}
			continue
		}
		for _, res := range ret.Results {
			if !fn.returnsTainted && fn.exprTainted(res) {
				fn.returnsTainted = true
				changed = true
			}
			if !fn.returnsReqTainted && fn.exprReqTainted(res) {
				fn.returnsReqTainted = true
				changed = true
			}
		}
	}
	return changed
}

// exprAuthDerived reports whether an expression's value came from an
// accepted authority: an authority call, a clean authority helper's return,
// a method called on an authority-derived value (`res.ActiveIDs()`), or a
// selector/index/alias of one.
func (fn *behaviorContractCensusFunc) exprAuthDerived(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return fn.exprAuthDerived(v.X)
	case *ast.StarExpr:
		return fn.exprAuthDerived(v.X)
	case *ast.UnaryExpr:
		return fn.exprAuthDerived(v.X)
	case *ast.Ident:
		return fn.authDerived[v.Name]
	case *ast.SelectorExpr:
		return fn.exprAuthDerived(v.X)
	case *ast.IndexExpr:
		return fn.exprAuthDerived(v.X)
	case *ast.CallExpr:
		if behaviorContractIDAuthorities[calleeName(v)] {
			return true
		}
		if callee := fn.calleeFunc(v); callee != nil && callee != fn &&
			callee.authority && !callee.readsRefs && !callee.readsIR && len(callee.tainted) == 0 {
			return true
		}
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok && fn.exprAuthDerived(sel.X) {
			return true
		}
	}
	return false
}

// propagateAuthDerived fills fn.authDerived with the locals whose values
// derive from an accepted authority, to a fixpoint.
func (fn *behaviorContractCensusFunc) propagateAuthDerived() {
	fn.authDerived = map[string]bool{}
	for {
		round := false
		ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range v.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok || fn.authDerived[id.Name] {
						continue
					}
					var rhs ast.Expr
					if len(v.Rhs) == len(v.Lhs) {
						rhs = v.Rhs[i]
					} else if len(v.Rhs) == 1 {
						rhs = v.Rhs[0]
					}
					if rhs != nil && fn.exprAuthDerived(rhs) {
						fn.authDerived[id.Name] = true
						round = true
					}
				}
			case *ast.ValueSpec:
				for i, name := range v.Names {
					if fn.authDerived[name.Name] || i >= len(v.Values) {
						continue
					}
					if fn.exprAuthDerived(v.Values[i]) {
						fn.authDerived[name.Name] = true
						round = true
					}
				}
			case *ast.RangeStmt:
				if !fn.exprAuthDerived(v.X) {
					return true
				}
				for _, e := range []ast.Expr{v.Key, v.Value} {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !fn.authDerived[id.Name] {
						fn.authDerived[id.Name] = true
						round = true
					}
				}
			}
			return true
		})
		if !round {
			return
		}
	}
}

// behaviorContractNeutralArg reports whether a required-id-call argument is
// inert (a literal, true/false/nil): it cannot carry an id space.
func behaviorContractNeutralArg(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return v.Name == "true" || v.Name == "false" || v.Name == "nil"
	}
	return false
}

func behaviorContractIDResolutionCensus(files []behaviorContractCensusFile) (offenders []string, resolutionChecked int, err error) {
	fset := token.NewFileSet()
	var funcs []*behaviorContractCensusFunc
	byPkgName := map[string]map[string]*behaviorContractCensusFunc{}
	// §40.43 round-six #8: per-package shared field-taint maps (see the
	// fieldTaint doc on behaviorContractCensusFunc).
	fieldTaintByPkg := map[string]map[string]bool{}
	pkgFieldMaps := func(pkg string) map[string]bool {
		if fieldTaintByPkg[pkg] == nil {
			fieldTaintByPkg[pkg] = map[string]bool{}
		}
		return fieldTaintByPkg[pkg]
	}
	// String-named types (`type rejection string`, aliases included) so a
	// refs gate cannot escape rule (c) by naming its string result type.
	stringTypes := map[string]map[string]bool{}
	setStringType := func(pkg, name string) {
		if stringTypes[pkg] == nil {
			stringTypes[pkg] = map[string]bool{}
		}
		stringTypes[pkg][name] = true
	}
	type pendingStringType struct{ pkg, name, depPkg, depName string }
	var pendingTypes []pendingStringType
	for _, f := range files {
		file, perr := parser.ParseFile(fset, f.name, f.src, 0)
		if perr != nil {
			return nil, 0, perr
		}
		where := func(n ast.Node) string { return f.name + ":" + strconv.Itoa(fset.Position(n.Pos()).Line) }
		// Import block → census package key, so `pkg.Callee(...)` resolves to
		// the censused function it names in another scanned package.
		importKey := map[string]string{}
		for _, spec := range file.Imports {
			path, perr := strconv.Unquote(spec.Path.Value)
			if perr != nil || !strings.HasPrefix(path, behaviorContractCensusModulePath) {
				continue
			}
			rel := strings.TrimPrefix(path, behaviorContractCensusModulePath)
			local := filepath.Base(rel)
			if spec.Name != nil && spec.Name.Name != "" && spec.Name.Name != "_" && spec.Name.Name != "." {
				local = spec.Name.Name
			}
			importKey[local] = behaviorContractCensusPackageKey(rel)
		}
		// (a) over the whole file.
		ast.Inspect(file, func(n ast.Node) bool {
			if v, ok := n.(*ast.Ident); ok && v.Name == "WriteBehaviorContractIDs" {
				offenders = append(offenders, where(v)+" references WriteBehaviorContractIDs (raw id set; resolve through the projection)")
			}
			return true
		})
		for _, decl := range file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					switch typ := ts.Type.(type) {
					case *ast.Ident:
						if typ.Name == "string" {
							setStringType(f.pkg, ts.Name.Name)
						} else {
							pendingTypes = append(pendingTypes, pendingStringType{f.pkg, ts.Name.Name, f.pkg, typ.Name})
						}
					case *ast.SelectorExpr:
						if qual, ok := typ.X.(*ast.Ident); ok {
							if key, ok := importKey[qual.Name]; ok {
								pendingTypes = append(pendingTypes, pendingStringType{f.pkg, ts.Name.Name, key, typ.Sel.Name})
							}
						}
					}
				}
				continue
			}
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := &behaviorContractCensusFunc{
				name: fd.Name.Name, file: f.name, pkg: f.pkg, decl: fd,
				tainted: map[string]bool{}, taintedReq: map[string]bool{},
				directTainted: map[string]bool{}, fieldTaint: pkgFieldMaps(f.pkg),
				importKey: importKey, lookup: byPkgName,
			}
			if fd.Recv != nil && len(fd.Recv.List) == 1 {
				recv := fd.Recv.List[0]
				if len(recv.Names) == 1 {
					fn.recvIdent = recv.Names[0].Name
				}
				rt := recv.Type
				if star, ok := rt.(*ast.StarExpr); ok {
					rt = star.X
				}
				if idx, ok := rt.(*ast.IndexExpr); ok { // generic receiver
					rt = idx.X
				}
				if id, ok := rt.(*ast.Ident); ok {
					fn.recvType = id.Name
				}
			}
			if fd.Type.Params != nil {
				for _, field := range fd.Type.Params.List {
					if len(field.Names) == 0 {
						fn.params = append(fn.params, "")
						continue
					}
					for _, name := range field.Names {
						fn.params = append(fn.params, name.Name)
					}
				}
			}
			if fd.Type.Results != nil {
				for _, r := range fd.Type.Results.List {
					fn.resultTypes = append(fn.resultTypes, r.Type)
					for _, name := range r.Names {
						fn.namedResults = append(fn.namedResults, name.Name)
					}
				}
			}
			// Return statements of THIS function only: a func literal's
			// returns are its own, never the enclosing function's.
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if _, ok := n.(*ast.FuncLit); ok {
					return false
				}
				if ret, ok := n.(*ast.ReturnStmt); ok {
					fn.returns = append(fn.returns, ret)
				}
				return true
			})
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					switch v.Sel.Name {
					case "ContractRefs", "PlacementRefs":
						fn.readsRefs = true
					case "BehaviorContracts":
						fn.idSource = true
						if x, ok := v.X.(*ast.SelectorExpr); ok && x.Sel.Name == "Request" {
							fn.readsIR = true
						}
					}
				case *ast.CallExpr:
					name := calleeName(v)
					calleePkg := f.pkg
					if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
						if qual, ok := sel.X.(*ast.Ident); ok {
							if key, ok := importKey[qual.Name]; ok {
								calleePkg = key
							}
						}
					}
					fn.calls = append(fn.calls, behaviorContractCensusCall{callee: name, pkg: calleePkg, args: v.Args, node: v})
					if strings.HasSuffix(name, "WriteBehaviorContractIDs") {
						fn.idSource = true
					}
					if behaviorContractIDAuthorities[name] {
						// An authority IS an id source: a helper that only
						// reads the projection is resolution-checked too.
						fn.authority = true
						fn.idSource = true
					}
					if name == "WriteAnalysisIR" {
						fn.readsIR = true
					}
				case *ast.AssignStmt:
					for _, lhs := range v.Lhs {
						if sel, ok := lhs.(*ast.SelectorExpr); ok && behaviorContractTombstoneWriterFields[sel.Sel.Name] {
							fn.writesField = true
						}
					}
				case *ast.KeyValueExpr:
					if key, ok := v.Key.(*ast.Ident); ok && behaviorContractTombstoneWriterFields[key.Name] {
						fn.writesField = true
					}
				}
				return true
			})
			funcs = append(funcs, fn)
			if byPkgName[f.pkg] == nil {
				byPkgName[f.pkg] = map[string]*behaviorContractCensusFunc{}
			}
			byPkgName[f.pkg][fn.name] = fn
		}
	}
	// Named-string-type fixpoint (`type A string; type B A; type C = pkg.B`).
	for changed := true; changed; {
		changed = false
		for _, p := range pendingTypes {
			if !stringTypes[p.pkg][p.name] && stringTypes[p.depPkg][p.depName] {
				setStringType(p.pkg, p.name)
				changed = true
			}
		}
	}
	// stringResult is judged over the whole scan: a bare `string`, a named
	// string type of the function's own package, or a qualified named string
	// type of another scanned package all make the result a gate result.
	for _, fn := range funcs {
		for _, r := range fn.resultTypes {
			switch id := r.(type) {
			case *ast.Ident:
				if id.Name == "string" || stringTypes[fn.pkg][id.Name] {
					fn.stringResult = true
				}
			case *ast.SelectorExpr:
				if qual, ok := id.X.(*ast.Ident); ok {
					if key, ok := fn.importKey[qual.Name]; ok && stringTypes[key][id.Sel.Name] {
						fn.stringResult = true
					}
				}
			}
		}
	}
	// Taint fixpoint: locals from the snapshot / tainted values / Request
	// carriers, range variables, copy() destinations, callee parameters fed
	// a tainted argument, and callee RETURNS back into every call expression
	// — across every censused function, in the same package or through a
	// qualified call into another scanned package.
	for changed := true; changed; {
		changed = false
		for _, fn := range funcs {
			if fn.propagateTaintLocally() {
				changed = true
			}
			if fn.recomputeReturnTaint() {
				changed = true
			}
			for _, call := range fn.calls {
				callee, ok := byPkgName[call.pkg][call.callee]
				if !ok || callee == fn {
					continue
				}
				for i, arg := range call.args {
					if i >= len(callee.params) || callee.params[i] == "" {
						continue
					}
					if !callee.tainted[callee.params[i]] && fn.exprTainted(arg) {
						callee.tainted[callee.params[i]] = true
						changed = true
					}
					if !callee.taintedReq[callee.params[i]] && fn.exprReqTainted(arg) {
						callee.taintedReq[callee.params[i]] = true
						changed = true
					}
				}
			}
		}
	}
	// idSource propagates through same-package helpers (a gate whose ids come
	// from a helper is resolution-checked through that helper).
	for changed := true; changed; {
		changed = false
		for _, fn := range funcs {
			if fn.idSource {
				continue
			}
			for _, call := range fn.calls {
				if helper, ok := byPkgName[fn.pkg][call.callee]; ok && helper != fn && helper.idSource {
					fn.idSource = true
					changed = true
					break
				}
			}
		}
	}
	for _, fn := range funcs {
		where := func(n ast.Node) string { return fn.file + ":" + strconv.Itoa(fset.Position(n.Pos()).Line) }
		taintedIdents := make([]string, 0, len(fn.tainted))
		for id := range fn.tainted {
			taintedIdents = append(taintedIdents, id)
		}
		sort.Strings(taintedIdents)
		anyTaint := len(taintedIdents) > 0
		for _, call := range fn.calls {
			// (b) sinks: the required-id set functions.
			if behaviorContractRequiredIDFuncs[call.callee] {
				for _, arg := range call.args {
					if fn.exprTainted(arg) {
						offenders = append(offenders, where(call.node)+" feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to "+call.callee)
					}
				}
			}
			// (e) the pure projection is a types/writeflow-seed API.
			if call.callee == "ProjectWriteBehaviorContractGeneration" && fn.pkg != "writeflow" {
				offenders = append(offenders, where(call.node)+" calls the pure projection; use Mutable.ProjectBehaviorContractGeneration so the run's tombstone ledger is supplied from state")
			}
		}
		// (d) applies to every censused package, assignment and literal.
		if fn.writesField && fn.name != "attachWriteBehaviorContracts" {
			offenders = append(offenders, fn.file+": "+fn.name+" writes a tombstone/generation field (only attachWriteBehaviorContracts may)")
		}
		// (f) rendering packages never iterate/index the snapshot.
		if fn.pkg == "agent" || fn.pkg == "writeflow" {
			ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.RangeStmt:
					if fn.exprTainted(v.X) {
						offenders = append(offenders, where(v)+" iterates the pre-rebase analyzer snapshot (directly or via alias/parameter); render the projection")
					}
				case *ast.IndexExpr:
					if fn.exprTainted(v.X) {
						offenders = append(offenders, where(v)+" indexes the pre-rebase analyzer snapshot (directly or via alias/parameter); render the projection")
					}
				}
				return true
			})
		}
		// (c) refs gates in package tool.
		if fn.pkg != "tool" || !fn.stringResult || !fn.readsRefs {
			continue
		}
		if fn.readsIR || anyTaint {
			offenders = append(offenders, fn.file+": "+fn.name+" is a refs gate that consults the analyzer snapshot / WriteAnalysisIR() (directly or via alias/parameter "+strings.Join(taintedIdents, ",")+")")
			continue
		}
		if !fn.idSource {
			continue
		}
		resolutionChecked++
		// Per-expression resolution: EVERY id source the gate consults must
		// be an accepted authority (an authority call, a required-id call
		// whose arguments are authority-derived, or a clean authority
		// helper). An authority called for something else resolves nothing:
		// one raw source — a `.BehaviorContracts` union, a required-id call
		// fed a non-authority value, or a helper that consults the snapshot
		// — makes the gate an offender.
		fn.propagateAuthDerived()
		clean := 0
		var raw []string
		ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				name := calleeName(v)
				if behaviorContractIDAuthorities[name] {
					clean++
					return true
				}
				if strings.HasSuffix(name, "WriteBehaviorContractIDs") {
					ok := true
					for _, arg := range v.Args {
						if !fn.exprAuthDerived(arg) && !behaviorContractNeutralArg(arg) {
							ok = false
						}
					}
					if ok {
						clean++
					} else {
						raw = append(raw, name+" fed a non-authority id source")
					}
					return true
				}
				if callee := fn.calleeFunc(v); callee != nil && callee != fn && callee.idSource {
					if callee.authority && !callee.readsRefs && !callee.readsIR && len(callee.tainted) == 0 {
						clean++
					} else {
						raw = append(raw, "helper "+name+" consults a raw id source")
					}
				}
			case *ast.SelectorExpr:
				if v.Sel.Name == "BehaviorContracts" {
					raw = append(raw, "raw .BehaviorContracts union")
				}
			}
			return true
		})
		if clean == 0 || len(raw) > 0 {
			msg := fn.file + ": " + fn.name + " resolves contract ids without resolveBehaviorContractIDs / the projection"
			if len(raw) > 0 {
				msg += " for every judged id (" + strings.Join(raw, "; ") + ")"
			}
			offenders = append(offenders, msg)
		}
	}
	// §40.43 round-six #8 fail-loud lane: stores of tainted values the
	// engine could not bind to a base identifier (deduped — the fixpoint
	// revisits the same store every round).
	untracked := map[string]bool{}
	for _, fn := range funcs {
		for _, store := range fn.untrackedStores {
			if !untracked[store] {
				untracked[store] = true
				offenders = append(offenders, store)
			}
		}
	}
	sort.Strings(offenders)
	return offenders, resolutionChecked, nil
}

func TestBehaviorContractIDResolutionCensus(t *testing.T) {
	// Every non-test Go file of every package under internal/ and cmd/ except
	// internal/types (the authority itself) is scanned: the guarded APIs are
	// exported, so totality means "every package that can import types", not
	// the four packages that consume them today. Rules (c)/(f) are keyed on
	// the package; rules (a)/(b)/(d)/(e) and the cross-package call taint
	// apply everywhere.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var files []behaviorContractCensusFile
	counts := map[string]int{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if !(rel == "internal" || rel == "cmd" || strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "cmd/")) {
				return filepath.SkipDir
			}
			if rel == "internal/types" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		files = append(files, behaviorContractCensusFile{name: rel, src: string(src), pkg: behaviorContractCensusPackageKey(dir)})
		counts[dir]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"internal/tool", "internal/agent", "internal/writeflow", "internal/orchestrator"} {
		if counts[dir] < 10 {
			t.Fatalf("census floor: only %d non-test files under %s", counts[dir], dir)
		}
	}
	if len(counts) < 20 || counts["cmd"] == 0 {
		t.Fatalf("census floor: only %d packages scanned (cmd=%d) — walk drift", len(counts), counts["cmd"])
	}
	offenders, checked, err := behaviorContractIDResolutionCensus(files)
	if err != nil {
		t.Fatal(err)
	}
	// Drift floor over RESOLUTION-CHECKED gates: the combined refs gate and
	// the proof follow-up gate (through its id helper) must both be judged.
	if checked < 2 {
		t.Fatalf("census resolution-checked only %d refs gates — collector drift", checked)
	}
	if len(offenders) > 0 {
		t.Fatalf("behavior-contract id resolution census:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// TestBehaviorContractIDResolutionCensusSelfRed: every evasion shape the
// census claims to close is flagged on a probe source (red witnesses of the
// tripwire itself). The three shapes named in §40.46 C4 — alias, parameter-
// passing helper, composite-literal writer — plus the pure-projection call and
// the original direct shapes.
func TestBehaviorContractIDResolutionCensusSelfRed(t *testing.T) {
	run := func(t *testing.T, name, pkg, src string, wantChecked int, wants ...string) {
		t.Helper()
		offenders, checked, err := behaviorContractIDResolutionCensus([]behaviorContractCensusFile{{name: name, src: src, pkg: pkg}})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(offenders, "\n")
		for _, want := range wants {
			if !strings.Contains(joined, want) {
				t.Fatalf("self-check missed %q in:\n%s", want, joined)
			}
		}
		if checked != wantChecked {
			t.Fatalf("self-check resolution-checked %d gates, want %d (offenders:\n%s)", checked, wantChecked, joined)
		}
	}
	t.Run("direct", func(t *testing.T) {
		run(t, "probe.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func probeRefsGate(ctx *types.BusContext, probes []types.VerificationProbe) string {
	ir := ctx.Mutable.WriteAnalysisIR()
	ids := types.WriteBehaviorContractIDs(ir.Request.BehaviorContracts)
	required := types.RequiredWriteBehaviorContractIDs(ir.Request.BehaviorContracts, true)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if _, ok := ids[ref]; !ok {
				return ref
			}
			if _, ok := required[ref]; !ok {
				return ref
			}
		}
	}
	return ""
}
func probeWriter(plan *types.ChangePlan) { plan.SupersededBehaviorContractIDs = nil }
`, 0, "references WriteBehaviorContractIDs", "feeds the pre-rebase analyzer snapshot", "is a refs gate that consults the analyzer snapshot", "writes a tombstone/generation field")
	})
	t.Run("unresolved_gate", func(t *testing.T) {
		run(t, "probe.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func probeGate(plan *types.ChangePlan, probes []types.VerificationProbe) string {
	ids := map[string]bool{}
	for _, c := range plan.BehaviorContracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
`, 1, "resolves contract ids without")
	})
	t.Run("agent_alias", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
func render(ir *types.WriteAnalysisIR) string {
	contracts := ir.Request.BehaviorContracts
	out := ""
	for _, c := range contracts {
		out += c.ID
	}
	_ = types.RequiredWriteBehaviorContractIDs(contracts, true)
	return out
}
`, 0, "feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to RequiredWriteBehaviorContractIDs", "iterates the pre-rebase analyzer snapshot")
	})
	// §40.43 round-six #8: struct-field stores used to launder the snapshot
	// colour — the LHS was not an Ident, so propagateTaintLocally dropped
	// the assignment and `s.c` read back untainted, evading both the
	// rule-(f) iteration sink and the rule-(b) Required* argument sink; the
	// receiver-field variant evaded identically across methods.
	t.Run("agent_struct_field_stash", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
type stash struct{ c []types.WriteBehaviorContract }
func render(ir *types.WriteAnalysisIR) string {
	var s stash
	s.c = ir.Request.BehaviorContracts
	out := ""
	for _, c := range s.c {
		out += c.ID
	}
	_ = types.RequiredWriteBehaviorContractIDs(s.c, true)
	return out
}
`, 0, "feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to RequiredWriteBehaviorContractIDs", "iterates the pre-rebase analyzer snapshot")
	})
	// §40.43 round-seven #6: a KEYED composite literal used to colour only the
	// whole local (broad lane), so the field copy `t.c = s.c` was skipped by
	// the precise store rule and `range t.c` read back uncoloured — 0
	// offenders on 79ca2f98b for exactly this shape. The literal's field key
	// now carries the direct colour ("s.c"), so the two-hop copy is followed.
	t.Run("agent_composite_literal_then_field_copy", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
type stash struct{ c []types.WriteBehaviorContract }
func render(ir *types.WriteAnalysisIR) string {
	s := stash{c: ir.Request.BehaviorContracts}
	var t stash
	t.c = s.c
	out := ""
	for _, c := range t.c {
		out += c.ID
	}
	_ = types.RequiredWriteBehaviorContractIDs(t.c, true)
	return out
}
`, 0, "feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to RequiredWriteBehaviorContractIDs", "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("agent_pointer_composite_literal_then_field_copy", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
type stash struct{ c []types.WriteBehaviorContract }
func render(ir *types.WriteAnalysisIR) string {
	var s = &stash{c: ir.Request.BehaviorContracts}
	t := &stash{}
	t.c = s.c
	out := ""
	for _, c := range t.c {
		out += c.ID
	}
	return out
}
`, 0, "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("agent_receiver_field_stash_across_methods", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
type holder struct {
	ir *types.WriteAnalysisIR
	c  []types.WriteBehaviorContract
}
func (h *holder) stashProbe() { h.c = h.ir.Request.BehaviorContracts }
func (h *holder) renderProbe() string {
	out := ""
	for _, c := range h.c {
		out += c.ID
	}
	return out
}
`, 0, "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("tool_parameter_helper", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func validateRefsRaw(contracts []types.WriteBehaviorContract, probes []types.VerificationProbe) string {
	ids := map[string]bool{}
	for _, c := range contracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
func caller(ctx *types.BusContext, probes []types.VerificationProbe) (string, error) {
	ir := ctx.Mutable.WriteAnalysisIR()
	return validateRefsRaw(ir.Request.BehaviorContracts, probes), nil
}
`, 0, "validateRefsRaw is a refs gate that consults the analyzer snapshot / WriteAnalysisIR() (directly or via alias/parameter c,contracts)")
	})
	t.Run("tool_two_hop_parameter", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func idsOf(contracts []types.WriteBehaviorContract) map[string]bool {
	ids := map[string]bool{}
	for _, c := range contracts {
		ids[c.ID] = true
	}
	return ids
}
func gate(contracts []types.WriteBehaviorContract, probes []types.VerificationProbe) string {
	ids := idsOf(contracts)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
func caller(ctx *types.BusContext, probes []types.VerificationProbe) (string, error) {
	snapshot := ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts
	return gate(snapshot, probes), nil
}
`, 0, "gate is a refs gate that consults the analyzer snapshot")
	})
	t.Run("cross_package_parameter_taint", func(t *testing.T) {
		// An orchestrator caller feeding the snapshot into an exported tool
		// refs gate through a qualified call: the taint crosses the package
		// boundary via the import block, so the gate is judged tainted.
		offenders, checked, err := behaviorContractIDResolutionCensus([]behaviorContractCensusFile{
			{name: "internal/tool/x.go", pkg: "tool", src: `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func ValidateRefsRaw(contracts []types.WriteBehaviorContract, probes []types.VerificationProbe) string {
	ids := map[string]bool{}
	for _, c := range contracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
`},
			{name: "internal/orchestrator/y.go", pkg: "orchestrator", src: `package orchestrator
import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)
func run(ir *types.WriteAnalysisIR, probes []types.VerificationProbe) (string, error) {
	return tool.ValidateRefsRaw(ir.Request.BehaviorContracts, probes), nil
}
`},
			{name: "internal/writeflow/z.go", pkg: "writeflow", src: `package writeflow
import wt "github.com/hanchaoqun/codrax/internal/tool"
func alias(contracts []int) string { return wt.Other(contracts) }
`},
		})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(offenders, "\n")
		if !strings.Contains(joined, "ValidateRefsRaw is a refs gate that consults the analyzer snapshot / WriteAnalysisIR() (directly or via alias/parameter c,contracts)") {
			t.Fatalf("cross-package taint missed:\n%s", joined)
		}
		if checked != 0 {
			t.Fatalf("tainted gate must not count as resolution-checked (%d)", checked)
		}
	})
	t.Run("orchestrator_composite_literal_writer", func(t *testing.T) {
		run(t, "x.go", "orchestrator", `package orchestrator
import "github.com/hanchaoqun/codrax/internal/types"
func mint() *types.ChangePlan {
	return &types.ChangePlan{SupersededBehaviorContractIDs: nil, BehaviorContractGeneration: ""}
}
`, 0, "mint writes a tombstone/generation field")
	})
	t.Run("tool_pure_projection", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func project(ctx *types.BusContext) types.WriteBehaviorContractResolution {
	ir := ctx.Mutable.WriteAnalysisIR()
	return types.ProjectWriteBehaviorContractGeneration(ir.Request.BehaviorContracts, nil, ctx.Mutable.VerifyFailureHandoff(), nil, nil)
}
`, 0, "calls the pure projection; use Mutable.ProjectBehaviorContractGeneration")
	})
	t.Run("proof_followup_helper_is_resolution_checked", func(t *testing.T) {
		// A gate that sources ids through a helper is judged through the
		// helper: unresolved helper ⇒ offender, resolved helper ⇒ checked.
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func requiredIDs(ctx *types.BusContext) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts {
		out[c.ID] = struct{}{}
	}
	return out
}
func gate(ctx *types.BusContext, probes []types.VerificationProbe) string {
	required := requiredIDs(ctx)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if _, ok := required[ref]; !ok {
				return ref
			}
		}
	}
	return ""
}
`, 1, "gate resolves contract ids without")
		run(t, "y.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func requiredIDs(ctx *types.BusContext) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range ctx.Mutable.ProjectBehaviorContractGeneration(nil, nil).Contracts {
		out[c.ID] = struct{}{}
	}
	return out
}
func gate(ctx *types.BusContext, probes []types.VerificationProbe) string {
	required := requiredIDs(ctx)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if _, ok := required[ref]; !ok {
				return ref
			}
		}
	}
	return ""
}
`, 1)
	})
	// EVOLUTION RECORD (§40.46 合流复核收编, G-contract-ids): the taint engine
	// propagated call arguments into callee parameters but never a callee's
	// RETURN into the call expression, ignored method returns, `req :=
	// ir.Request` aliases, copy() destinations, and named string result
	// types, and a gate counted as "resolved" if it called ANY authority.
	// The six subtests below were RED against that engine (each shape kept
	// the census green); the engine now taints to fixpoint through return
	// values (functions and methods), Request-carrier aliases and copy()
	// destinations, judges named-string results as refs gates, and resolves
	// a gate per id-source EXPRESSION (every judged id must come from an
	// accepted authority; one authority call elsewhere resolves nothing).
	t.Run("agent_return_value_helper", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
func snapshot(ctx *types.AgentContext) []types.WriteBehaviorContract {
	return ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts
}
func render(ctx *types.AgentContext) string {
	out := ""
	for _, c := range snapshot(ctx) {
		out += c.ID
	}
	_ = types.RequiredWriteBehaviorContractIDs(snapshot(ctx), true)
	return out
}
`, 0, "iterates the pre-rebase analyzer snapshot", "feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to RequiredWriteBehaviorContractIDs")
	})
	t.Run("agent_method_return", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
type frame struct{ ir *types.WriteAnalysisIR }
func (f *frame) contracts() []types.WriteBehaviorContract { return f.ir.Request.BehaviorContracts }
func (f *frame) render() string {
	out := ""
	for _, c := range f.contracts() {
		out += c.ID
	}
	return out
}
`, 0, "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("agent_request_alias", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
func render(ir *types.WriteAnalysisIR) string {
	req := ir.Request
	out := ""
	for _, c := range req.BehaviorContracts {
		out += c.ID
	}
	return out
}
`, 0, "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("agent_copy_builtin", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
func render(ir *types.WriteAnalysisIR) string {
	dst := make([]types.WriteBehaviorContract, len(ir.Request.BehaviorContracts))
	copy(dst, ir.Request.BehaviorContracts)
	out := ""
	for _, c := range dst {
		out += c.ID
	}
	return out
}
`, 0, "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("tool_named_string_result_gate", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
type rejection string
func probeGate(plan *types.ChangePlan, probes []types.VerificationProbe) rejection {
	ids := map[string]bool{}
	for _, c := range plan.BehaviorContracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return rejection(ref)
			}
		}
	}
	return ""
}
`, 1, "resolves contract ids without")
	})
	t.Run("tool_mixed_source_gate", func(t *testing.T) {
		// The gate calls an accepted authority for something else while the
		// ids it actually judges come from a raw WriteAnalysisIR() helper:
		// per-expression resolution flags the raw source; the authority call
		// resolves nothing.
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func rawIDs(ctx *types.BusContext) map[string]bool {
	ids := map[string]bool{}
	for _, c := range ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts {
		ids[c.ID] = true
	}
	return ids
}
func gate(ctx *types.BusContext, plan *types.ChangePlan, probes []types.VerificationProbe) string {
	res := resolveBehaviorContractIDs(plan)
	_ = res
	ids := rawIDs(ctx)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
`, 1, "gate resolves contract ids without")
	})
}
