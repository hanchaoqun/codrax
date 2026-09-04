package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTraceDBCallstackLifecycleAuthorityIsStructurallyPinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	dir := filepath.Dir(current)
	isIdent := func(expression ast.Expr, name string) bool {
		ident, ok := expression.(*ast.Ident)
		return ok && ident.Name == name
	}
	isSelector := func(expression ast.Expr, receiver, field string) bool {
		selector, ok := expression.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == field && isIdent(selector.X, receiver)
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
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return ""
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return ""
		}
		return ident.Name
	}

	type site struct {
		function string
		call     *ast.CallExpr
	}
	targets := map[string]bool{
		"exportTraceDBCallstack":                   true,
		"exportTraceDBFrameSliceWithRows":          true,
		"prepareTraceDBCallstackRow":               true,
		"resolveCallstackSchedulerAlias":           true,
		"newTraceDBSchedulerRunningIndex":          true,
		"resolveThreadSubject":                     true,
		"threadPointAllows":                        true,
		"threadClosedEndpointAllows":               true,
		"processClosedEndpointAllows":              true,
		"lookupCPUAt":                              true,
		"traceDBResolveLifecycleCallstackIdentity": true,
		"traceDBCallstackExactEmitterCandidates":   true,
		"traceDBCallstackExactAsyncKey":            true,
		"auditTraceDBCallstackAsyncGroup":          true,
		"newTraceDBRawAsyncMatchLedger":            true,
		"submit":                                   true,
		"poisonExactLane":                          true,
		"fenceExactLane":                           true,
		"traceDBExtendedRunningCPUAt":              true,
		"traceDBKnownCPUAt":                        true,
		"loadRunningIntervals":                     true,
		"loadSchedulerRunningIndex":                true,
		"collectTraceDBLifecycle":                  true,
		"loadThreadIndex":                          true,
	}
	calls := map[string][]site{}
	functionTypes := map[string]map[string]int{}
	callstackCompositeRefs := 0
	failCalls := 0
	coverageErrorAssignments := 0
	exactAsyncKeyRowFields := map[string]bool{}
	var barrierPopulate, centralFence, centralPoison, centralSubmit token.Pos
	var typedBuild, measureDispatch, callstackDispatch, frameDispatch token.Pos

	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if function.Name.Name == "exportTraceDBCallstack" || function.Name.Name == "prepareTraceDBCallstackRow" {
				functionTypes[function.Name.Name] = map[string]int{}
				for _, field := range function.Type.Params.List {
					switch fieldType := field.Type.(type) {
					case *ast.Ident:
						functionTypes[function.Name.Name][fieldType.Name] += len(field.Names)
					case *ast.MapType:
						functionTypes[function.Name.Name]["map"] += len(field.Names)
					case *ast.StarExpr:
						if ident, ok := fieldType.X.(*ast.Ident); ok {
							functionTypes[function.Name.Name]["*"+ident.Name] += len(field.Names)
						}
					}
				}
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if selector, ok := node.(*ast.SelectorExpr); ok && function.Name.Name == "traceDBCallstackExactAsyncKey" && isIdent(selector.X, "row") {
					exactAsyncKeyRowFields[selector.Sel.Name] = true
				}
				if assignment, ok := node.(*ast.AssignStmt); ok && function.Name.Name == "exportTraceDBCallstack" {
					for _, lhs := range assignment.Lhs {
						if isSelector(lhs, "coverage", "Error") {
							coverageErrorAssignments++
						}
					}
				}
				if composite, ok := node.(*ast.CompositeLit); ok && function.Name.Name == "exportTraceDBExtendedFamilies" {
					ast.Inspect(composite, func(child ast.Node) bool {
						ident, ok := child.(*ast.Ident)
						if ok && ident.Name == "exportTraceDBCallstack" {
							callstackCompositeRefs++
						}
						return true
					})
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := callName(call)
				if targets[name] {
					calls[name] = append(calls[name], site{function: function.Name.Name, call: call})
				}
				if function.Name.Name == "exportTraceDBCallstack" && name == "fail" {
					failCalls++
				}
				if function.Name.Name == "exportTraceDBCallstack" && name == "traceDBCallstackExactEmitterCandidates" {
					barrierPopulate = call.Pos()
				}
				if function.Name.Name == "exportTraceDBCallstack" && name == "poisonExactLane" {
					centralPoison = call.Pos()
				}
				if function.Name.Name == "exportTraceDBCallstack" && name == "fenceExactLane" {
					centralFence = call.Pos()
				}
				if function.Name.Name == "exportTraceDBCallstack" && name == "submit" {
					centralSubmit = call.Pos()
				}
				if function.Name.Name == "exportTraceDBExtendedFamilies" {
					switch name {
					case "newTraceDBSchedulerRunningIndex":
						typedBuild = call.Pos()
					case "exportTraceDBMeasureFamilies":
						measureDispatch = call.Pos()
					case "exportTraceDBCallstack":
						callstackDispatch = call.Pos()
					case "exportTraceDBFrameSliceWithRows":
						frameDispatch = call.Pos()
					}
				}
				return true
			})
		}
	}

	callerCounts := func(name string) map[string]int {
		out := map[string]int{}
		for _, item := range calls[name] {
			out[item.function]++
		}
		return out
	}
	if !reflect.DeepEqual(callerCounts("exportTraceDBCallstack"), map[string]int{"exportTraceDBExtendedFamilies": 1}) || callstackCompositeRefs != 0 {
		t.Fatalf("callstack production callers=%v composite_refs=%d", callerCounts("exportTraceDBCallstack"), callstackCompositeRefs)
	}
	if !reflect.DeepEqual(callerCounts("prepareTraceDBCallstackRow"), map[string]int{"exportTraceDBCallstack": 1}) {
		t.Fatalf("callstack prepare callers=%v", callerCounts("prepareTraceDBCallstackRow"))
	}
	if !reflect.DeepEqual(callerCounts("traceDBCallstackExactAsyncKey"), map[string]int{"exportTraceDBCallstack": 1}) ||
		!reflect.DeepEqual(exactAsyncKeyRowFields, map[string]bool{
			"Flag": true, "OwnerIPID": true, "TGID": true, "Name": true, "Cookie": true,
		}) {
		t.Fatalf("callstack exact async key closure callers=%v fields=%v",
			callerCounts("traceDBCallstackExactAsyncKey"), exactAsyncKeyRowFields)
	}
	for _, function := range []string{"exportTraceDBCallstack", "prepareTraceDBCallstackRow"} {
		got := functionTypes[function]
		if got["traceDBSchedulerAuthority"] != 1 || got["traceDBSchedulerRunningIndex"] != 1 ||
			got["traceDBThreadIndex"] != 0 || got["map"] != 0 ||
			(function == "exportTraceDBCallstack" && got["*traceDBSyncSpanAuthority"] != 1) ||
			(function == "prepareTraceDBCallstackRow" && got["*traceDBSyncSpanAuthority"] != 0) {
			t.Fatalf("%s authority parameter types=%v", function, got)
		}
	}

	extendedCall := calls["exportTraceDBCallstack"][0].call
	if len(extendedCall.Args) != 6 || !isIdent(extendedCall.Args[0], "ctx") || !isIdent(extendedCall.Args[1], "tdb") ||
		!isIdent(extendedCall.Args[2], "sink") || !isIdent(extendedCall.Args[3], "authority") || !isIdent(extendedCall.Args[4], "callstackRunning") ||
		!isIdent(extendedCall.Args[5], "syncSpans") {
		t.Fatal("extended callstack dispatch does not pass the shared typed authority plus exact raw CPU fallback")
	}
	var extendedTypedBuild *ast.CallExpr
	for _, item := range calls["newTraceDBSchedulerRunningIndex"] {
		if item.function == "exportTraceDBExtendedFamilies" {
			extendedTypedBuild = item.call
		}
	}
	if extendedTypedBuild == nil || len(extendedTypedBuild.Args) != 4 || !isIdent(extendedTypedBuild.Args[0], "authority") ||
		!isIdent(extendedTypedBuild.Args[1], "running") || !isIdent(extendedTypedBuild.Args[2], "runningIntegrity") ||
		!isIdent(extendedTypedBuild.Args[3], "nil") {
		t.Fatal("extended callstack Running view is not derived from the shared scan/integrity/authority")
	}
	if typedBuild == 0 || measureDispatch == 0 || callstackDispatch == 0 || frameDispatch == 0 ||
		typedBuild >= callstackDispatch || measureDispatch >= callstackDispatch || callstackDispatch >= frameDispatch {
		t.Fatalf("extended callstack order typed=%d measure=%d callstack=%d frame=%d", typedBuild, measureDispatch, callstackDispatch, frameDispatch)
	}

	if !reflect.DeepEqual(callerCounts("threadPointAllows"), map[string]int{
		"auditDBEdges":                    2,
		"exportTraceDBPerfSamples":        1,
		"exportTraceDBWakeups":            2,
		"loadTraceDBBlockedCandidates":    1,
		"prepareTraceDBCallstackRow":      4,
		"prepareTraceDBEBPFCommon":        1,
		"prepareTraceDBNativeHookEvent":   1,
		"prepareTraceDBSyscallRow":        1,
		"resolveCallstackSchedulerAlias":  1,
		"schedulerPointAllows":            1,
		"traceDBAdmitRawCanonicalSubject": 1,
		"traceDBResolveRawPublicTID":      1,
	}) || !reflect.DeepEqual(callerCounts("threadClosedEndpointAllows"), map[string]int{
		"loadTraceDBBlockedSchedBoundaries": 1,
		"prepareTraceDBCallstackRow":        1,
		"prepareTraceDBFrameSliceRow":       1,
		"prepareTraceDBSyscallRow":          1,
		"resolveCallstackSchedulerAlias":    1,
		"schedulerNextPointAllows":          1,
	}) || !reflect.DeepEqual(callerCounts("processClosedEndpointAllows"), map[string]int{
		"auditTraceDBCallstackAsyncGroup": 1,
		"prepareTraceDBSyscallRow":        1,
	}) {
		t.Fatalf("callstack lifecycle call closure point=%v closed=%v process=%v",
			callerCounts("threadPointAllows"), callerCounts("threadClosedEndpointAllows"), callerCounts("processClosedEndpointAllows"))
	}
	if callerCounts("lookupCPUAt")["prepareTraceDBCallstackRow"] != 2 ||
		callerCounts("lookupCPUAt")["resolveCallstackSchedulerAlias"] != 2 ||
		callerCounts("lookupCPUAt")["traceDBResolvePerfSampleCPU"] != 1 ||
		callerCounts("resolveThreadSubject")["traceDBResolveRawPublicTID"] != 1 ||
		callerCounts("resolveThreadSubject")["prepareTraceDBCallstackRow"] != 2 ||
		callerCounts("resolveThreadSubject")["exportTraceDBCallstack"] != 1 ||
		callerCounts("resolveThreadSubject")["traceDBCallstackExactEmitterCandidates"] != 1 ||
		callerCounts("traceDBResolveLifecycleCallstackIdentity")["traceDBCallstackExactEmitterCandidates"] != 1 {
		t.Fatalf("callstack typed resolution closure lookup=%v resolve=%v candidate=%v",
			callerCounts("lookupCPUAt"), callerCounts("resolveThreadSubject"), callerCounts("traceDBResolveLifecycleCallstackIdentity"))
	}
	if !reflect.DeepEqual(callerCounts("resolveCallstackSchedulerAlias"), map[string]int{"prepareTraceDBCallstackRow": 1}) ||
		callerCounts("resolveThreadSubject")["resolveCallstackSchedulerAlias"] != 1 {
		t.Fatalf("callstack scheduler alias escaped its single typed chokepoint: alias=%v resolve=%v",
			callerCounts("resolveCallstackSchedulerAlias"), callerCounts("resolveThreadSubject"))
	}
	lookupArgs := map[string]int{}
	for _, item := range calls["lookupCPUAt"] {
		if item.function != "prepareTraceDBCallstackRow" {
			continue
		}
		if receiverName(item.call) != "running" || len(item.call.Args) != 2 ||
			!isSelector(item.call.Args[0], "row", "EmitterITID") {
			t.Fatal("callstack typed Running lookup authority or identity changed")
		}
		switch {
		case isSelector(item.call.Args[1], "row", "TS"):
			lookupArgs["start"]++
		case isSelector(item.call.Args[1], "row", "End"):
			lookupArgs["end"]++
		default:
			t.Fatal("callstack typed Running lookup timestamp changed")
		}
	}
	if !reflect.DeepEqual(lookupArgs, map[string]int{"start": 1, "end": 1}) {
		t.Fatalf("callstack typed Running endpoint lookups=%v", lookupArgs)
	}
	asyncAudits := callerCounts("auditTraceDBCallstackAsyncGroup")
	if !reflect.DeepEqual(asyncAudits, map[string]int{"exportTraceDBCallstack": 1}) {
		t.Fatalf("callstack async audit callers=%v", asyncAudits)
	}
	rawAsyncBuilders := callerCounts("newTraceDBRawAsyncMatchLedger")
	if !reflect.DeepEqual(rawAsyncBuilders, map[string]int{"exportTraceDBCallstack": 1}) {
		t.Fatalf("callstack raw async replacement authority callers=%v", rawAsyncBuilders)
	}
	asyncAuditCall := calls["auditTraceDBCallstackAsyncGroup"][0].call
	if len(asyncAuditCall.Args) != 2 || !isIdent(asyncAuditCall.Args[0], "authority") || !isIdent(asyncAuditCall.Args[1], "group") {
		t.Fatal("callstack async audit does not consume the shared authority and exact group")
	}
	processGateCall := calls["processClosedEndpointAllows"][0].call
	if receiverName(processGateCall) != "authority" || len(processGateCall.Args) != 3 ||
		!isSelector(processGateCall.Args[0], "open", "OwnerIPID") ||
		!isSelector(processGateCall.Args[1], "open", "TS") ||
		!isSelector(processGateCall.Args[2], "row", "TS") {
		t.Fatal("callstack async process closed-interval gate arguments changed")
	}

	for _, forbidden := range []string{"traceDBExtendedRunningCPUAt", "traceDBKnownCPUAt", "loadRunningIntervals", "loadSchedulerRunningIndex", "collectTraceDBLifecycle", "loadThreadIndex"} {
		if callerCounts(forbidden)["exportTraceDBCallstack"] != 0 || callerCounts(forbidden)["prepareTraceDBCallstackRow"] != 0 {
			t.Fatalf("callstack reopened forbidden authority %s: %v", forbidden, callerCounts(forbidden))
		}
	}
	if barrierPopulate == 0 || centralFence == 0 || centralPoison == 0 || centralSubmit == 0 ||
		barrierPopulate >= centralFence || centralFence >= centralPoison || centralPoison >= centralSubmit {
		t.Fatalf("exact rejected-lane fence/poison does not dominate central submission: candidates=%d fence=%d poison=%d submit=%d",
			barrierPopulate, centralFence, centralPoison, centralSubmit)
	}
	if failCalls != 26 || coverageErrorAssignments != 1 {
		t.Fatalf("callstack error chokepoint calls=%d coverage.Error assignments=%d, want 26/1", failCalls, coverageErrorAssignments)
	}

	logicalOwnerPointGate := 0
	for _, item := range calls["threadPointAllows"] {
		if item.function != "prepareTraceDBCallstackRow" {
			continue
		}
		if receiverName(item.call) != "authority" || len(item.call.Args) != 2 ||
			(!isSelector(item.call.Args[0], "row", "EmitterITID") &&
				!isSelector(item.call.Args[0], "row", "LogicalOwnerITID")) ||
			!isSelector(item.call.Args[1], "row", "TS") {
			t.Fatal("callstack point gate arguments changed")
		}
		if isSelector(item.call.Args[0], "row", "LogicalOwnerITID") {
			logicalOwnerPointGate++
		}
	}
	if logicalOwnerPointGate != 1 {
		t.Fatalf("official async logical owner point gates=%d, want 1", logicalOwnerPointGate)
	}
	for _, item := range calls["threadClosedEndpointAllows"] {
		if item.function != "prepareTraceDBCallstackRow" {
			continue
		}
		if receiverName(item.call) != "authority" || len(item.call.Args) != 3 ||
			!isSelector(item.call.Args[0], "row", "EmitterITID") || !isSelector(item.call.Args[1], "row", "TS") ||
			!isSelector(item.call.Args[2], "row", "End") {
			t.Fatal("callstack closed endpoint gate arguments changed")
		}
	}
}
