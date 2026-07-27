package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestTraceDBSyncSpanAuthorityProductionClosure pins the B1-c production
// boundary. SQL-derived synchronous B/E rows have one authority, one bounded
// typed stage, one physical lane identity, and one pass-2 publication point.
// Official source-event renderers remain outside that boundary because they
// reproduce captured wire data rather than synthesize SQL spans.
func TestTraceDBSyncSpanAuthorityProductionClosure(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve structure test source")
	}
	dir := filepath.Dir(current)
	fset := token.NewFileSet()

	type callSite struct {
		file     string
		function string
		call     *ast.CallExpr
	}
	type literalSite struct {
		file     string
		function string
		value    string
	}
	type functionInfo struct {
		file string
		decl *ast.FuncDecl
	}
	type laneLiteralSite struct {
		file     string
		function string
		literal  *ast.CompositeLit
	}

	isIdent := func(expr ast.Expr, name string) bool {
		ident, ok := expr.(*ast.Ident)
		return ok && ident.Name == name
	}
	selectorParts := func(expr ast.Expr) (receiver, field string, ok bool) {
		selector, ok := expr.(*ast.SelectorExpr)
		if !ok {
			return "", "", false
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return "", "", false
		}
		return ident.Name, selector.Sel.Name, true
	}
	var expressionPath func(ast.Expr) string
	expressionPath = func(expr ast.Expr) string {
		switch typed := expr.(type) {
		case *ast.Ident:
			return typed.Name
		case *ast.SelectorExpr:
			prefix := expressionPath(typed.X)
			if prefix == "" {
				return ""
			}
			return prefix + "." + typed.Sel.Name
		default:
			return ""
		}
	}
	callName := func(call *ast.CallExpr) string {
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			return callee.Name
		case *ast.SelectorExpr:
			return callee.Sel.Name
		default:
			return ""
		}
	}
	receiverName := func(call *ast.CallExpr) string {
		receiver, _, ok := selectorParts(call.Fun)
		if !ok {
			return ""
		}
		return receiver
	}
	typeName := func(expr ast.Expr) string {
		switch typed := expr.(type) {
		case *ast.Ident:
			return typed.Name
		case *ast.StarExpr:
			if ident, ok := typed.X.(*ast.Ident); ok {
				return "*" + ident.Name
			}
		}
		return ""
	}
	receiverTypeName := func(declaration *ast.FuncDecl) string {
		if declaration.Recv == nil || len(declaration.Recv.List) != 1 {
			return ""
		}
		return typeName(declaration.Recv.List[0].Type)
	}
	compositeTypeName := func(literal *ast.CompositeLit) string {
		if ident, ok := literal.Type.(*ast.Ident); ok {
			return ident.Name
		}
		return ""
	}

	targetCalls := map[string]bool{
		"newTraceDBSyncSpanAuthority":              true,
		"newTraceDBSyncSpanStage":                  true,
		"exportTraceDBSchedulerFamilies":           true,
		"exportTraceDBExtendedFamilies":            true,
		"exportTraceDBThreadRegistrations":         true,
		"exportTraceDBCallstack":                   true,
		"exportTraceDBSyscall":                     true,
		"exportTraceDBAppStartup":                  true,
		"exportTraceDBStaticInitialize":            true,
		"submit":                                   true,
		"poisonExactLane":                          true,
		"fenceExactLane":                           true,
		"poisonGlobally":                           true,
		"addCandidate":                             true,
		"addPoison":                                true,
		"addFence":                                 true,
		"finalize":                                 true,
		"cleanup":                                  true,
		"seal":                                     true,
		"newBadLaneJournal":                        true,
		"auditFrozenLanes":                         true,
		"publishFrozenCleanLanes":                  true,
		"traceDBPublishSyncSpanEndpoint":           true,
		"candidateIterator":                        true,
		"forcedIterator":                           true,
		"fenceIterator":                            true,
		"reader":                                   true,
		"reconcileTraceDBSyncSpanCoverage":         true,
		"traceDBSyncSpanEndpoints":                 true,
		"prepareTraceDBRenderedRow":                true,
		"prepareTraceDBCPUUnavailableTraceMarkRow": true,
		"addTraceDBSpanRows":                       true,
		"addTraceDBInstantRow":                     true,
		"addTraceDBAsyncSpanRows":                  true,
		"newTraceDBRowSink":                        true,
		"writeTo":                                  true,
		"add":                                      true,
		"CreateTemp":                               true,
		"MkdirTemp":                                true,
		"flushChunk":                               true,
	}
	calls := map[string][]callSite{}
	authorityCalls := map[string][]callSite{}
	functions := map[string][]functionInfo{}
	var markerLiterals []literalSite
	var laneLiterals []laneLiteralSite
	var laneType *ast.StructType
	var authorityType *ast.StructType
	authorityImports := map[string]bool{}
	var topRowsAccepted token.Pos
	var topSyncCleanupDefer token.Pos

	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		base := filepath.Base(path)
		if base == "streamerdb_sync_span_authority.go" {
			for _, item := range file.Imports {
				value, err := strconv.Unquote(item.Path.Value)
				if err != nil {
					t.Fatalf("decode import %s: %v", item.Path.Value, err)
				}
				authorityImports[value] = true
			}
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					switch typeSpec.Name.Name {
					case "traceDBSyncSpanLane":
						laneType = structure
					case "traceDBSyncSpanAuthority":
						authorityType = structure
					}
				}
			case *ast.FuncDecl:
				if declaration.Body == nil {
					continue
				}
				name := declaration.Name.Name
				functions[name] = append(functions[name], functionInfo{file: base, decl: declaration})
				syncAuthorityVariables := map[string]bool{}
				for _, fields := range []*ast.FieldList{declaration.Recv, declaration.Type.Params} {
					if fields == nil {
						continue
					}
					for _, field := range fields.List {
						if typeName(field.Type) != "*traceDBSyncSpanAuthority" {
							continue
						}
						for _, fieldName := range field.Names {
							syncAuthorityVariables[fieldName.Name] = true
						}
					}
				}
				// Resolve the single constructor result and any simple local aliases
				// before classifying method calls. This keeps the hard gate typed:
				// unrelated methods named submit/finalize do not become false hits.
				ast.Inspect(declaration.Body, func(node ast.Node) bool {
					switch node := node.(type) {
					case *ast.AssignStmt:
						for index, rhs := range node.Rhs {
							call, isCall := rhs.(*ast.CallExpr)
							if isCall && callName(call) == "newTraceDBSyncSpanAuthority" && len(node.Lhs) > 0 {
								if lhs, ok := node.Lhs[0].(*ast.Ident); ok {
									syncAuthorityVariables[lhs.Name] = true
								}
								continue
							}
							if index >= len(node.Lhs) {
								continue
							}
							rhsIdent, rhsIsIdent := rhs.(*ast.Ident)
							lhsIdent, lhsIsIdent := node.Lhs[index].(*ast.Ident)
							if rhsIsIdent && lhsIsIdent && syncAuthorityVariables[rhsIdent.Name] {
								syncAuthorityVariables[lhsIdent.Name] = true
							}
						}
					case *ast.ValueSpec:
						for index, rhs := range node.Values {
							if index >= len(node.Names) {
								continue
							}
							if call, ok := rhs.(*ast.CallExpr); ok && callName(call) == "newTraceDBSyncSpanAuthority" {
								syncAuthorityVariables[node.Names[index].Name] = true
								continue
							}
							if ident, ok := rhs.(*ast.Ident); ok && syncAuthorityVariables[ident.Name] {
								syncAuthorityVariables[node.Names[index].Name] = true
							}
						}
					}
					return true
				})
				ast.Inspect(declaration.Body, func(node ast.Node) bool {
					switch node := node.(type) {
					case *ast.DeferStmt:
						if name != "exportTraceDBToSystraceFromOpenWithLedger" {
							return true
						}
						foundCleanup := false
						ast.Inspect(node.Call, func(child ast.Node) bool {
							call, ok := child.(*ast.CallExpr)
							if ok && callName(call) == "cleanup" && expressionPath(call.Fun) == "syncSpans.cleanup" {
								foundCleanup = true
							}
							return true
						})
						if foundCleanup {
							if topSyncCleanupDefer != 0 {
								t.Fatal("multiple top-level sync authority cleanup defers")
							}
							topSyncCleanupDefer = node.Pos()
						}
					case *ast.BasicLit:
						if node.Kind != token.STRING {
							return true
						}
						value, err := strconv.Unquote(node.Value)
						if err != nil {
							t.Fatalf("decode string literal in %s.%s: %v", base, name, err)
						}
						if strings.Contains(value, "B|") || strings.Contains(value, "E|") {
							markerLiterals = append(markerLiterals, literalSite{file: base, function: name, value: value})
						}
					case *ast.CompositeLit:
						if compositeTypeName(node) == "traceDBSyncSpanLane" {
							laneLiterals = append(laneLiterals, laneLiteralSite{file: base, function: name, literal: node})
						}
					case *ast.SelectorExpr:
						if name == "exportTraceDBToSystraceFromOpenWithLedger" && node.Sel.Name == "RowsAccepted" {
							if receiver, _, ok := selectorParts(node.X); ok && receiver == "sink" && topRowsAccepted == 0 {
								topRowsAccepted = node.Pos()
							}
						}
					case *ast.CallExpr:
						callee := callName(node)
						if targetCalls[callee] {
							site := callSite{file: base, function: name, call: node}
							calls[callee] = append(calls[callee], site)
							if receiver, _, ok := selectorParts(node.Fun); ok && syncAuthorityVariables[receiver] {
								authorityCalls[callee] = append(authorityCalls[callee], site)
							}
						}
					}
					return true
				})
			}
		}
	}

	callerCounts := func(name string) map[string]int {
		out := map[string]int{}
		for _, site := range calls[name] {
			out[site.function]++
		}
		return out
	}
	authorityCallerCounts := func(name string) map[string]int {
		out := map[string]int{}
		for _, site := range authorityCalls[name] {
			out[site.function]++
		}
		return out
	}
	onlyCall := func(name, caller string) *ast.CallExpr {
		t.Helper()
		var found *ast.CallExpr
		for _, site := range calls[name] {
			if site.function != caller {
				continue
			}
			if found != nil {
				t.Fatalf("multiple %s calls in %s", name, caller)
			}
			found = site.call
		}
		if found == nil {
			t.Fatalf("missing %s call in %s", name, caller)
		}
		return found
	}
	onlyAuthorityCall := func(name, caller string) *ast.CallExpr {
		t.Helper()
		var found *ast.CallExpr
		for _, site := range authorityCalls[name] {
			if site.function != caller {
				continue
			}
			if found != nil {
				t.Fatalf("multiple sync authority %s calls in %s", name, caller)
			}
			found = site.call
		}
		if found == nil {
			t.Fatalf("missing sync authority %s call in %s", name, caller)
		}
		return found
	}
	onlyCallOn := func(name, caller, receiverPath string) *ast.CallExpr {
		t.Helper()
		var found *ast.CallExpr
		for _, site := range calls[name] {
			if site.function != caller {
				continue
			}
			selector, ok := site.call.Fun.(*ast.SelectorExpr)
			if !ok || expressionPath(selector.X) != receiverPath {
				continue
			}
			if found != nil {
				t.Fatalf("multiple %s calls on %s in %s", name, receiverPath, caller)
			}
			found = site.call
		}
		if found == nil {
			t.Fatalf("missing %s call on %s in %s", name, receiverPath, caller)
		}
		return found
	}
	countParamType := func(function, want string) int {
		t.Helper()
		infos := functions[function]
		if len(infos) != 1 {
			t.Fatalf("function %s declarations=%d", function, len(infos))
		}
		count := 0
		for _, field := range infos[0].decl.Type.Params.List {
			if typeName(field.Type) == want {
				count += len(field.Names)
			}
		}
		return count
	}
	onlyMethod := func(function, receiverType string) functionInfo {
		t.Helper()
		var found *functionInfo
		for _, info := range functions[function] {
			if receiverTypeName(info.decl) != receiverType {
				continue
			}
			if found != nil {
				t.Fatalf("multiple %s methods on %s", function, receiverType)
			}
			copy := info
			found = &copy
		}
		if found == nil {
			t.Fatalf("missing %s method on %s", function, receiverType)
		}
		return *found
	}
	countDeclParamType := func(declaration *ast.FuncDecl, want string) int {
		count := 0
		for _, field := range declaration.Type.Params.List {
			if typeName(field.Type) == want {
				count += len(field.Names)
			}
		}
		return count
	}

	// The top-level owns one authority. The exact same pointer crosses both
	// collection stages and is finalized once, after both stages and before any
	// empty-output or file-write decision.
	if !reflect.DeepEqual(callerCounts("newTraceDBSyncSpanAuthority"), map[string]int{"exportTraceDBToSystraceFromOpenWithLedger": 1}) {
		t.Fatalf("sync authority constructors=%v", callerCounts("newTraceDBSyncSpanAuthority"))
	}
	syncFinalizers := authorityCallerCounts("finalize")
	if !reflect.DeepEqual(syncFinalizers, map[string]int{"exportTraceDBToSystraceFromOpenWithLedger": 1}) {
		t.Fatalf("sync authority finalizers=%v", syncFinalizers)
	}
	constructor := onlyCall("newTraceDBSyncSpanAuthority", "exportTraceDBToSystraceFromOpenWithLedger")
	scheduler := onlyCall("exportTraceDBSchedulerFamilies", "exportTraceDBToSystraceFromOpenWithLedger")
	extended := onlyCall("exportTraceDBExtendedFamilies", "exportTraceDBToSystraceFromOpenWithLedger")
	finalize := onlyAuthorityCall("finalize", "exportTraceDBToSystraceFromOpenWithLedger")
	reconcile := onlyCall("reconcileTraceDBSyncSpanCoverage", "exportTraceDBToSystraceFromOpenWithLedger")
	writeTo := onlyCall("writeTo", "exportTraceDBToSystraceFromOpenWithLedger")
	if len(constructor.Args) != 2 || !isIdent(constructor.Args[0], "ctx") || !isIdent(constructor.Args[1], "output") {
		t.Fatal("sync authority constructor does not use (ctx, final output artifact)")
	}
	if len(scheduler.Args) != 4 || !isIdent(scheduler.Args[0], "ctx") || !isIdent(scheduler.Args[1], "tdb") ||
		!isIdent(scheduler.Args[2], "sink") || !isIdent(scheduler.Args[3], "syncSpans") {
		t.Fatal("scheduler stage does not receive the top-level syncSpans pointer")
	}
	if len(extended.Args) != 5 || !isIdent(extended.Args[0], "ctx") || !isIdent(extended.Args[1], "tdb") ||
		!isIdent(extended.Args[2], "sink") || !isIdent(extended.Args[3], "authority") || !isIdent(extended.Args[4], "syncSpans") {
		t.Fatal("extended stage does not receive the top-level syncSpans pointer")
	}
	if receiverName(finalize) != "syncSpans" || len(finalize.Args) != 2 || !isIdent(finalize.Args[0], "ctx") || !isIdent(finalize.Args[1], "sink") {
		t.Fatal("top-level finalize does not use syncSpans.finalize(ctx, sink)")
	}
	if len(reconcile.Args) != 2 || !isIdent(reconcile.Args[0], "coverage") || !isIdent(reconcile.Args[1], "syncReport") {
		t.Fatal("sync coverage is not reconciled from the unique finalization report")
	}
	if receiverName(writeTo) != "sink" || topRowsAccepted == 0 ||
		topSyncCleanupDefer == 0 || constructor.Pos() >= topSyncCleanupDefer || topSyncCleanupDefer >= scheduler.Pos() ||
		scheduler.Pos() >= extended.Pos() || extended.Pos() >= finalize.Pos() ||
		finalize.Pos() >= reconcile.Pos() || reconcile.Pos() >= topRowsAccepted || topRowsAccepted >= writeTo.Pos() {
		t.Fatalf("sync authority order constructor=%d cleanup_defer=%d scheduler=%d extended=%d finalize=%d reconcile=%d empty=%d write=%d",
			constructor.Pos(), topSyncCleanupDefer, scheduler.Pos(), extended.Pos(), finalize.Pos(), reconcile.Pos(), topRowsAccepted, writeTo.Pos())
	}
	if countParamType("exportTraceDBSchedulerFamilies", "*traceDBSyncSpanAuthority") != 1 ||
		countParamType("exportTraceDBExtendedFamilies", "*traceDBSyncSpanAuthority") != 1 {
		t.Fatal("scheduler/extended stages do not each accept one shared sync authority pointer")
	}

	// Exactly the five governed SQL/metadata producers submit. Every submit uses
	// the shared pointer and a typed candidate literal.
	wantSubmitters := map[string]int{
		"exportTraceDBThreadRegistrations": 1,
		"exportTraceDBCallstack":           1,
		"exportTraceDBSyscall":             1,
		"exportTraceDBAppStartup":          1,
		"exportTraceDBStaticInitialize":    1,
	}
	if !reflect.DeepEqual(authorityCallerCounts("submit"), wantSubmitters) {
		t.Fatalf("sync span submitters=%v want=%v", authorityCallerCounts("submit"), wantSubmitters)
	}
	for _, site := range authorityCalls["submit"] {
		if receiverName(site.call) != "syncSpans" || len(site.call.Args) != 2 || !isIdent(site.call.Args[0], "ctx") {
			t.Fatalf("%s submit does not use syncSpans.submit(ctx, candidate)", site.function)
		}
		literal, ok := site.call.Args[1].(*ast.CompositeLit)
		if !ok || compositeTypeName(literal) != "traceDBSyncSpanCandidate" {
			t.Fatalf("%s submit does not use a typed sync candidate literal", site.function)
		}
		if countParamType(site.function, "*traceDBSyncSpanAuthority") != 1 {
			t.Fatalf("%s does not accept exactly one sync authority pointer", site.function)
		}
	}
	if !reflect.DeepEqual(authorityCallerCounts("poisonExactLane"), map[string]int{
		"exportTraceDBCallstack": 1,
		"exportTraceDBSyscall":   1,
	}) {
		t.Fatalf("exact lane poison callers=%v", authorityCallerCounts("poisonExactLane"))
	}
	poison := onlyAuthorityCall("poisonExactLane", "exportTraceDBCallstack")
	if receiverName(poison) != "syncSpans" || len(poison.Args) != 2 || !isIdent(poison.Args[0], "ctx") {
		t.Fatal("callstack poison does not use syncSpans.poisonExactLane(ctx, poison)")
	}
	poisonLiteral, ok := poison.Args[1].(*ast.CompositeLit)
	if !ok || compositeTypeName(poisonLiteral) != "traceDBSyncSpanLanePoison" {
		t.Fatal("callstack poison is not a typed exact-lane declaration")
	}
	poisonFields := map[string]ast.Expr{}
	for _, element := range poisonLiteral.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			t.Fatal("callstack poison uses an unkeyed field")
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok {
			t.Fatal("callstack poison uses a non-identifier field")
		}
		poisonFields[key.Name] = pair.Value
	}
	threadReceiver, threadField, threadOK := selectorParts(poisonFields["HeaderTID"])
	if !isIdent(poisonFields["Producer"], "traceDBSyncSpanProducerCallstack") ||
		!threadOK || threadReceiver != "thread" || threadField != "TID" ||
		!isIdent(poisonFields["CanonicalITID"], "itid") ||
		!isIdent(poisonFields["CanonicalITIDKnown"], "true") ||
		!isIdent(poisonFields["Reason"], "traceDBSyncSpanLanePoisonRejectedCallstackCandidate") {
		t.Fatalf("callstack poison lost exact typed identity mapping: fields=%v", poisonFields)
	}
	if !reflect.DeepEqual(authorityCallerCounts("fenceExactLane"), map[string]int{
		"exportTraceDBCallstack": 1,
	}) {
		t.Fatalf("exact lane fence callers=%v", authorityCallerCounts("fenceExactLane"))
	}
	fence := onlyAuthorityCall("fenceExactLane", "exportTraceDBCallstack")
	if receiverName(fence) != "syncSpans" || len(fence.Args) != 2 ||
		!isIdent(fence.Args[0], "ctx") || !isIdent(fence.Args[1], "fence") {
		t.Fatal("callstack fence does not use syncSpans.fenceExactLane(ctx, fence)")
	}
	syscallPoison := onlyAuthorityCall("poisonExactLane", "exportTraceDBSyscall")
	if receiverName(syscallPoison) != "syncSpans" || len(syscallPoison.Args) != 2 || !isIdent(syscallPoison.Args[0], "ctx") {
		t.Fatal("syscall poison does not use syncSpans.poisonExactLane(ctx, poison)")
	}
	syscallPoisonLiteral, ok := syscallPoison.Args[1].(*ast.CompositeLit)
	if !ok || compositeTypeName(syscallPoisonLiteral) != "traceDBSyncSpanLanePoison" {
		t.Fatal("syscall poison is not a typed exact-lane declaration")
	}
	syscallPoisonFields := map[string]ast.Expr{}
	for _, element := range syscallPoisonLiteral.Elts {
		pair, pairOK := element.(*ast.KeyValueExpr)
		if !pairOK {
			t.Fatal("syscall poison uses an unkeyed field")
		}
		key, keyOK := pair.Key.(*ast.Ident)
		if !keyOK {
			t.Fatal("syscall poison uses a non-identifier field")
		}
		syscallPoisonFields[key.Name] = pair.Value
	}
	syscallThreadReceiver, syscallThreadField, syscallThreadOK := selectorParts(syscallPoisonFields["HeaderTID"])
	if !isIdent(syscallPoisonFields["Producer"], "traceDBSyncSpanProducerSyscall") ||
		!syscallThreadOK || syscallThreadReceiver != "thread" || syscallThreadField != "TID" ||
		!isIdent(syscallPoisonFields["CanonicalITID"], "itid") ||
		!isIdent(syscallPoisonFields["CanonicalITIDKnown"], "true") ||
		!isIdent(syscallPoisonFields["Reason"], "traceDBSyncSpanLanePoisonRejectedSyscallCandidate") {
		t.Fatalf("syscall poison lost exact typed identity mapping: fields=%v", syscallPoisonFields)
	}
	if !reflect.DeepEqual(authorityCallerCounts("poisonGlobally"), map[string]int{"exportTraceDBSyscall": 1}) {
		t.Fatalf("global poison callers=%v", authorityCallerCounts("poisonGlobally"))
	}
	globalPoison := onlyAuthorityCall("poisonGlobally", "exportTraceDBSyscall")
	if receiverName(globalPoison) != "syncSpans" || len(globalPoison.Args) != 2 || !isIdent(globalPoison.Args[0], "ctx") {
		t.Fatal("syscall global poison does not use syncSpans.poisonGlobally(ctx, poison)")
	}
	globalPoisonLiteral, ok := globalPoison.Args[1].(*ast.CompositeLit)
	if !ok || compositeTypeName(globalPoisonLiteral) != "traceDBSyncSpanGlobalPoison" {
		t.Fatal("syscall global poison is not the shared typed authority declaration")
	}
	globalPoisonFields := map[string]ast.Expr{}
	for _, element := range globalPoisonLiteral.Elts {
		pair, pairOK := element.(*ast.KeyValueExpr)
		if !pairOK {
			t.Fatal("syscall global poison uses an unkeyed field")
		}
		key, keyOK := pair.Key.(*ast.Ident)
		if !keyOK {
			t.Fatal("syscall global poison uses a non-identifier field")
		}
		globalPoisonFields[key.Name] = pair.Value
	}
	if len(globalPoisonFields) != 2 ||
		!isIdent(globalPoisonFields["Producer"], "traceDBSyncSpanProducerSyscall") ||
		!isIdent(globalPoisonFields["Reason"], "traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate") {
		t.Fatalf("syscall global poison lost closed producer/reason: %v", globalPoisonFields)
	}
	if countParamType("poisonGlobally", "*traceDBRowSink") != 0 {
		t.Fatal("global poison authority acquired a row sink")
	}
	for _, forbidden := range []string{"traceDBPublishSyncSpanEndpoint", "prepareTraceDBRenderedRow", "add", "writeTo"} {
		for _, site := range calls[forbidden] {
			if site.function == "poisonGlobally" {
				t.Fatalf("global poison authority publishes through %s", forbidden)
			}
		}
	}

	// Pin the stage-to-producer handoff as well; this prevents a second pointer
	// from being substituted after the top-level call appears structurally sound.
	dispatches := []struct {
		name   string
		caller string
		argc   int
		index  int
	}{
		{"exportTraceDBThreadRegistrations", "exportTraceDBSchedulerFamilies", 5, 2},
		{"exportTraceDBCallstack", "exportTraceDBExtendedFamilies", 6, 5},
		{"exportTraceDBSyscall", "exportTraceDBExtendedFamilies", 6, 5},
		{"exportTraceDBAppStartup", "exportTraceDBExtendedFamilies", 6, 3},
		{"exportTraceDBStaticInitialize", "exportTraceDBExtendedFamilies", 5, 3},
	}
	for _, dispatch := range dispatches {
		call := onlyCall(dispatch.name, dispatch.caller)
		if len(call.Args) != dispatch.argc || !isIdent(call.Args[dispatch.index], "syncSpans") {
			t.Fatalf("%s does not pass the stage's exact syncSpans pointer", dispatch.name)
		}
	}

	// TaskPool and async/resource producers are deliberately outside the B/E
	// authority. Their S/F or instant contracts must not acquire a sync pointer.
	for _, function := range []string{"exportTraceDBTaskPool", "exportTraceDBFrameSliceWithRows", "exportTraceDBNativeHook"} {
		if countParamType(function, "*traceDBSyncSpanAuthority") != 0 {
			t.Fatalf("%s incorrectly entered the sync span authority", function)
		}
		for method, sites := range authorityCalls {
			for _, site := range sites {
				if site.function == function {
					t.Fatalf("%s calls sync authority method %s", function, method)
				}
			}
		}
		decl := functions[function][0].decl
		ast.Inspect(decl.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if ok && (compositeTypeName(literal) == "traceDBSyncSpanCandidate" || compositeTypeName(literal) == "traceDBSyncSpanLanePoison") {
				t.Fatalf("%s constructs a governed sync span value", function)
			}
			return true
		})
	}

	// Mechanical B/E takeover must not imply source-admission correctness for
	// the three legacy SQL producers. Their R1b-C disclosure stays explicit
	// until that separately scoped batch closes.
	for _, function := range []string{"exportTraceDBAppStartup", "exportTraceDBStaticInitialize"} {
		decl := functions[function][0].decl
		fieldSources := map[string]string{}
		fieldSourceAssignments := 0
		ast.Inspect(decl.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			receiver, field, ok := selectorParts(assignment.Lhs[0])
			if !ok || receiver != "coverage" || field != "FieldSources" {
				return true
			}
			fieldSourceAssignments++
			if fieldSourceAssignments != 1 {
				t.Fatalf("%s reassigns coverage.FieldSources %d times", function, fieldSourceAssignments)
			}
			literal, ok := assignment.Rhs[0].(*ast.CompositeLit)
			if !ok {
				t.Fatalf("%s coverage.FieldSources is not a literal closed map", function)
			}
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					t.Fatalf("%s coverage.FieldSources contains an unkeyed entry", function)
				}
				keyLiteral, keyOK := pair.Key.(*ast.BasicLit)
				valueLiteral, valueOK := pair.Value.(*ast.BasicLit)
				if !keyOK || !valueOK || keyLiteral.Kind != token.STRING || valueLiteral.Kind != token.STRING {
					t.Fatalf("%s coverage.FieldSources contains a non-literal entry", function)
				}
				key, keyErr := strconv.Unquote(keyLiteral.Value)
				value, valueErr := strconv.Unquote(valueLiteral.Value)
				if keyErr != nil || valueErr != nil {
					t.Fatalf("%s coverage.FieldSources contains an invalid string", function)
				}
				fieldSources[key] = value
			}
			return false
		})
		if fieldSourceAssignments != 1 ||
			!strings.Contains(fieldSources["source_admission"], "remain open as R1b-C") ||
			!strings.Contains(fieldSources["wire_laminar"], "shared authority") ||
			!strings.Contains(fieldSources["wire_laminar"], "no endpoint is published") {
			t.Fatalf("%s coverage.FieldSources lost B1-b/R1b-C disclosure: %v", function, fieldSources)
		}
	}
	syscallDecl := functions["exportTraceDBSyscall"][0].decl
	syscallFieldSources := map[string]string{}
	ast.Inspect(syscallDecl.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		receiver, field, ok := selectorParts(assignment.Lhs[0])
		if !ok || receiver != "coverage" || field != "FieldSources" {
			return true
		}
		literal, ok := assignment.Rhs[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("syscall coverage.FieldSources is not a literal closed map")
		}
		for _, element := range literal.Elts {
			pair := element.(*ast.KeyValueExpr)
			key, keyErr := strconv.Unquote(pair.Key.(*ast.BasicLit).Value)
			value, valueErr := strconv.Unquote(pair.Value.(*ast.BasicLit).Value)
			if keyErr != nil || valueErr != nil {
				t.Fatal("syscall coverage.FieldSources contains invalid strings")
			}
			syscallFieldSources[key] = value
		}
		return false
	})
	if !strings.Contains(syscallFieldSources["source_admission"], "bounded INTEGER transport") ||
		!strings.Contains(syscallFieldSources["wire_laminar"], "single shared authority") ||
		!strings.Contains(syscallFieldSources["wire_laminar"], "no endpoint is published") ||
		strings.Contains(syscallFieldSources["source_admission"], "remain open") {
		t.Fatalf("syscall C1 closure provenance missing: %v", syscallFieldSources)
	}

	// The physical stack key is exactly artifact source + row-header TID. It has
	// no payload TGID, canonical identity, producer, or display field.
	if laneType == nil {
		t.Fatal("traceDBSyncSpanLane type not found")
	}
	laneFields := map[string]string{}
	for _, field := range laneType.Fields.List {
		if len(field.Names) != 1 {
			t.Fatal("sync span lane contains an embedded or grouped field")
		}
		laneFields[field.Names[0].Name] = typeName(field.Type)
	}
	if !reflect.DeepEqual(laneFields, map[string]string{"ArtifactSource": "string", "HeaderTID": "int64"}) {
		t.Fatalf("sync span physical lane fields=%v", laneFields)
	}
	for _, site := range laneLiterals {
		if site.file != "streamerdb_sync_span_authority.go" && site.file != "streamerdb_sync_span_stage.go" {
			t.Fatalf("sync lane constructed outside authority: %s.%s", site.file, site.function)
		}
		keys := map[string]bool{}
		for _, element := range site.literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				t.Fatalf("unkeyed sync lane literal in %s", site.function)
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok {
				t.Fatalf("non-identifier sync lane key in %s", site.function)
			}
			keys[key.Name] = true
		}
		if !reflect.DeepEqual(keys, map[string]bool{"ArtifactSource": true, "HeaderTID": true}) {
			t.Fatalf("sync lane literal keys in %s=%v", site.function, keys)
		}
	}

	// Synthetic SQL B/E wire tokens live only in the authority. The sole
	// exception is the typed direct-ftrace source-event renderer, which renders
	// captured trace-marker payloads and is not a SQL span producer.
	allowedMarkerFunctions := map[string]map[string]bool{
		"streamerdb_sync_span_authority.go": {
			"validateTraceDBSyncSpanCandidate": true,
			"traceDBPublishSyncSpanEndpoint":   true,
		},
		"marker_payload.go": {
			"decodeDirectMarkerPayload": true,
		},
	}
	authorityMarkerFunctions := map[string]bool{}
	for _, literal := range markerLiterals {
		if !allowedMarkerFunctions[literal.file][literal.function] {
			t.Fatalf("B/E wire literal escaped authority/source-render whitelist: %s.%s %q", literal.file, literal.function, literal.value)
		}
		if literal.file == "streamerdb_sync_span_authority.go" {
			authorityMarkerFunctions[literal.function] = true
		}
	}
	if !reflect.DeepEqual(authorityMarkerFunctions, map[string]bool{
		"validateTraceDBSyncSpanCandidate": true,
		"traceDBPublishSyncSpanEndpoint":   true,
	}) {
		t.Fatalf("authority B/E marker functions=%v", authorityMarkerFunctions)
	}
	if len(functions["addTraceDBSpanRows"]) != 0 || len(calls["addTraceDBSpanRows"]) != 0 {
		t.Fatalf("legacy direct SQL B/E helper survived: decls=%d calls=%v", len(functions["addTraceDBSpanRows"]), callerCounts("addTraceDBSpanRows"))
	}

	// The authority owns exactly one bounded typed stage and no candidate
	// collection of its own. That prevents a second in-memory authority from
	// silently bypassing spill, duplicate arbitration, or the frozen iterators.
	if !reflect.DeepEqual(callerCounts("newTraceDBSyncSpanStage"), map[string]int{"newTraceDBSyncSpanAuthorityWithOptions": 1}) {
		t.Fatalf("sync typed stage constructors=%v", callerCounts("newTraceDBSyncSpanStage"))
	}
	if authorityType == nil {
		t.Fatal("traceDBSyncSpanAuthority type not found")
	}
	stageFields := 0
	for _, field := range authorityType.Fields.List {
		if _, isMap := field.Type.(*ast.MapType); isMap {
			t.Fatal("sync authority regained a map alongside its bounded stage")
		}
		if array, isArray := field.Type.(*ast.ArrayType); isArray && array.Len == nil {
			t.Fatal("sync authority regained a slice alongside its bounded stage")
		}
		for _, name := range field.Names {
			if name.Name == "stage" && typeName(field.Type) == "*traceDBSyncSpanStage" {
				stageFields++
			}
		}
		ast.Inspect(field.Type, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if ok && (ident.Name == "traceDBSyncSpanCandidate" || ident.Name == "traceDBSyncSpanIdentity" || ident.Name == "traceDBSyncSpanLanePoison") {
				t.Fatalf("sync authority stores %s outside its bounded stage", ident.Name)
			}
			return true
		})
	}
	if stageFields != 1 {
		t.Fatalf("sync authority typed stage fields=%d, want exactly one", stageFields)
	}
	for _, forbiddenImport := range []string{"database/sql", "os", "io", "bufio", "encoding/binary", "hash/crc32"} {
		if authorityImports[forbiddenImport] {
			t.Fatalf("sync authority bypasses its typed stage with storage package %q", forbiddenImport)
		}
	}
	for _, forbidden := range []string{"CreateTemp", "MkdirTemp", "flushChunk", "newTraceDBRowSink", "writeTo"} {
		for _, site := range calls[forbidden] {
			if site.file == "streamerdb_sync_span_authority.go" {
				t.Fatalf("sync authority bypasses its typed stage via %s in %s", forbidden, site.function)
			}
		}
	}

	// submit/poison/fence may validate typed envelopes but cannot reach a row
	// sink. All delegate into the authority's sole stage.
	submitInfo := onlyMethod("submit", "*traceDBSyncSpanAuthority")
	poisonInfo := onlyMethod("poisonExactLane", "*traceDBSyncSpanAuthority")
	fenceInfo := onlyMethod("fenceExactLane", "*traceDBSyncSpanAuthority")
	finalizeInfo := onlyMethod("finalize", "*traceDBSyncSpanAuthority")
	if submitInfo.file != "streamerdb_sync_span_authority.go" ||
		poisonInfo.file != "streamerdb_sync_span_authority.go" ||
		fenceInfo.file != "streamerdb_sync_span_authority.go" ||
		finalizeInfo.file != "streamerdb_sync_span_authority.go" {
		t.Fatal("submit/poison/fence/finalize methods are not uniquely owned by sync span authority")
	}
	if countDeclParamType(submitInfo.decl, "*traceDBRowSink") != 0 ||
		countDeclParamType(poisonInfo.decl, "*traceDBRowSink") != 0 ||
		countDeclParamType(fenceInfo.decl, "*traceDBRowSink") != 0 ||
		countDeclParamType(finalizeInfo.decl, "*traceDBRowSink") != 1 {
		t.Fatal("submit/poison/fence/finalize row-sink parameter boundary changed")
	}
	for _, method := range []functionInfo{submitInfo, poisonInfo, fenceInfo} {
		sinkWrites := 0
		ast.Inspect(method.decl.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callName(call)
			if receiverName(call) == "sink" || name == "addTraceDBInstantRow" || name == "addTraceDBAsyncSpanRows" ||
				name == "prepareTraceDBRenderedRow" || name == "traceDBPublishSyncSpanEndpoint" {
				sinkWrites++
			}
			return true
		})
		if sinkWrites != 0 {
			t.Fatalf("%s reaches row rendering/publication %d time(s)", method.decl.Name.Name, sinkWrites)
		}
	}
	if call := onlyCallOn("addCandidate", "submit", "authority.stage"); len(call.Args) != 2 || !isIdent(call.Args[0], "ctx") || !isIdent(call.Args[1], "candidate") {
		t.Fatal("submit does not delegate its typed candidate to authority.stage")
	}
	if call := onlyCallOn("addPoison", "poisonExactLane", "authority.stage"); len(call.Args) != 2 || !isIdent(call.Args[0], "ctx") || !isIdent(call.Args[1], "poison") {
		t.Fatal("poisonExactLane does not delegate its typed poison to authority.stage")
	}
	if call := onlyCallOn("addFence", "fenceExactLane", "authority.stage"); len(call.Args) != 2 || !isIdent(call.Args[0], "ctx") || !isIdent(call.Args[1], "fence") {
		t.Fatal("fenceExactLane does not delegate its typed fence to authority.stage")
	}

	// Finalization is a strict two-pass protocol: freeze the typed stage, audit
	// all lanes into a journal, seal that journal, then and only then publish.
	stageSeal := onlyCallOn("seal", "finalize", "authority.stage")
	newJournal := onlyCallOn("newBadLaneJournal", "finalize", "authority.stage")
	pass1 := onlyCallOn("auditFrozenLanes", "finalize", "authority")
	journalSeal := onlyCallOn("seal", "finalize", "journal")
	pass2 := onlyCallOn("publishFrozenCleanLanes", "finalize", "authority")
	if len(stageSeal.Args) != 1 || !isIdent(stageSeal.Args[0], "ctx") || len(newJournal.Args) != 0 ||
		len(pass1.Args) != 3 || !isIdent(pass1.Args[0], "ctx") || !isIdent(pass1.Args[2], "journal") ||
		len(journalSeal.Args) != 1 || !isIdent(journalSeal.Args[0], "ctx") ||
		len(pass2.Args) != 3 || !isIdent(pass2.Args[0], "ctx") || !isIdent(pass2.Args[1], "sink") || !isIdent(pass2.Args[2], "journal") {
		t.Fatal("sync two-pass calls lost their exact ctx/stage/journal/sink handoff")
	}
	if !(stageSeal.Pos() < newJournal.Pos() && newJournal.Pos() < pass1.Pos() && pass1.Pos() < journalSeal.Pos() && journalSeal.Pos() < pass2.Pos()) {
		t.Fatalf("sync two-pass order seal=%d journal=%d pass1=%d journal_seal=%d pass2=%d",
			stageSeal.Pos(), newJournal.Pos(), pass1.Pos(), journalSeal.Pos(), pass2.Pos())
	}
	directFinalizePublication := 0
	ast.Inspect(finalizeInfo.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if callName(call) == "prepareTraceDBRenderedRow" || callName(call) == "traceDBPublishSyncSpanEndpoint" ||
			(callName(call) == "add" && receiverName(call) == "sink") {
			directFinalizePublication++
		}
		return true
	})
	if directFinalizePublication != 0 {
		t.Fatalf("finalize bypasses pass-2 publication %d time(s)", directFinalizePublication)
	}
	auditInfo := onlyMethod("auditFrozenLanes", "*traceDBSyncSpanAuthority")
	publishInfo := onlyMethod("publishFrozenCleanLanes", "*traceDBSyncSpanAuthority")
	if countDeclParamType(auditInfo.decl, "*traceDBRowSink") != 0 || countDeclParamType(publishInfo.decl, "*traceDBRowSink") != 1 {
		t.Fatal("pass-1/pass-2 row-sink boundary changed")
	}
	if onlyCallOn("candidateIterator", "auditFrozenLanes", "authority.stage") == nil ||
		onlyCallOn("forcedIterator", "auditFrozenLanes", "authority.stage") == nil ||
		onlyCallOn("fenceIterator", "auditFrozenLanes", "authority.stage") == nil ||
		onlyCallOn("candidateIterator", "publishFrozenCleanLanes", "authority.stage") == nil ||
		onlyCallOn("fenceIterator", "publishFrozenCleanLanes", "authority.stage") == nil ||
		onlyCallOn("reader", "publishFrozenCleanLanes", "journal") == nil {
		t.Fatal("two-pass iterators no longer consume the sealed typed stage and journal")
	}
	endpointPublisherInfos := functions["traceDBPublishSyncSpanEndpoint"]
	if len(endpointPublisherInfos) != 1 || endpointPublisherInfos[0].file != "streamerdb_sync_span_authority.go" {
		t.Fatalf("sync endpoint publisher declarations=%v", endpointPublisherInfos)
	}
	publisherCalls := callerCounts("traceDBPublishSyncSpanEndpoint")
	if len(publisherCalls) != 1 || publisherCalls["publishFrozenCleanLanes"] == 0 {
		t.Fatalf("sync endpoint publisher callers=%v", publisherCalls)
	}
	prepareCount, unavailablePrepareCount, exactPrepareCount, addCount := 0, 0, 0, 0
	var preparePos, addPos token.Pos
	ast.Inspect(endpointPublisherInfos[0].decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if callName(call) == "prepareTraceDBRenderedRow" {
			prepareCount++
			preparePos = call.Pos()
		}
		if callName(call) == "prepareTraceDBCPUUnavailableTraceMarkRow" {
			unavailablePrepareCount++
		}
		if callName(call) == "prepareTraceDBExactTraceMarkRow" {
			exactPrepareCount++
		}
		if callName(call) == "add" && receiverName(call) == "sink" {
			addCount++
			addPos = call.Pos()
		}
		return true
	})
	if prepareCount != 1 || unavailablePrepareCount != 1 || exactPrepareCount != 1 ||
		addCount != 3 || preparePos >= addPos {
		t.Fatalf("pass-2 endpoint publication concrete_prepare=%d typed_unavailable_prepare=%d exact_prepare=%d/%d add=%d/%d",
			prepareCount, unavailablePrepareCount, exactPrepareCount, preparePos, addCount, addPos)
	}

	// B1-c is closed for the synthetic sync typed stage, and ROW-SORT-BND is
	// closed independently for the final generic row sorter. The legacy SQL
	// source-admission boundary above remains open as R1b-C.
	closureFragments := map[string]bool{
		"bounded pass 1 freezes and audits every governed lane":                false,
		"pass 2 alone may publish clean synthetic B/E endpoints":               false,
		"hybrid candidate-byte-bounded memory to private indexed SQLite stage": false,
		"final generic row sorter bounded at %d retained bytes/%d rows":        false,
		"%d input runs/%d run FDs":                                             false,
		"%d active/%d live temp bytes":                                         false,
	}
	ast.Inspect(finalizeInfo.decl.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		if (strings.Contains(value, "B1-c") || strings.Contains(value, "ROW-SORT-BND")) &&
			strings.Contains(strings.ToLower(value), "open") {
			t.Fatalf("sync authority still discloses a closed boundedness batch as open: %q", value)
		}
		for fragment := range closureFragments {
			if strings.Contains(value, fragment) {
				closureFragments[fragment] = true
			}
		}
		return true
	})
	for fragment, found := range closureFragments {
		if !found {
			t.Fatalf("sync B1-c/ROW-SORT-BND disclosure missing %q", fragment)
		}
	}
}
