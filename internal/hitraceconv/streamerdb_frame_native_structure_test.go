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

func TestTraceDBFrameAndNativeAuthoritiesAreStructurallyPinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	dir := filepath.Dir(current)
	isIdent := func(expr ast.Expr, name string) bool {
		ident, ok := expr.(*ast.Ident)
		return ok && ident.Name == name
	}
	isSelector := func(expr ast.Expr, receiver, field string) bool {
		selector, ok := expr.(*ast.SelectorExpr)
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

	type callSite struct {
		function string
		call     *ast.CallExpr
	}
	targets := map[string]bool{
		"exportTraceDBFrameSlice":            true,
		"prepareTraceDBFrameSliceRow":        true,
		"exportTraceDBNativeHook":            true,
		"prepareTraceDBNativeHookEvent":      true,
		"newTraceDBSchedulerRunningIndex":    true,
		"threadClosedEndpointAllows":         true,
		"threadPointAllows":                  true,
		"lookupCPUAt":                        true,
		"resolveThreadSubject":               true,
		"traceDBExtendedRunningCPUAt":        true,
		"traceDBKnownCPUAt":                  true,
		"traceDBActivityProfile":             true,
		"loadThreadIndex":                    true,
		"collectTraceDBLifecycle":            true,
		"loadRunningIntervals":               true,
		"loadSchedulerRunningIndex":          true,
		"loadExtendedLegacyRunningIntervals": true,
		"decode":                             true,
	}
	calls := map[string][]callSite{}
	functionTypes := map[string]map[string]int{}
	compositeRefs := map[string]int{}
	var callstackDispatch, frameDispatch, dmaDispatch token.Pos
	var staticDispatch, nativeDispatch, processMeasureDispatch token.Pos

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
			if function.Name.Name == "exportTraceDBFrameSlice" || function.Name.Name == "prepareTraceDBFrameSliceRow" ||
				function.Name.Name == "exportTraceDBNativeHook" || function.Name.Name == "prepareTraceDBNativeHookEvent" {
				functionTypes[function.Name.Name] = map[string]int{}
				for _, field := range function.Type.Params.List {
					switch fieldType := field.Type.(type) {
					case *ast.Ident:
						functionTypes[function.Name.Name][fieldType.Name] += len(field.Names)
					case *ast.MapType:
						key, keyOK := fieldType.Key.(*ast.Ident)
						value, valueOK := fieldType.Value.(*ast.Ident)
						if keyOK && valueOK && key.Name == "int64" && value.Name == "bool" {
							functionTypes[function.Name.Name]["map[int64]bool"] += len(field.Names)
						} else {
							functionTypes[function.Name.Name]["map"] += len(field.Names)
						}
					}
				}
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if composite, ok := node.(*ast.CompositeLit); ok && function.Name.Name == "exportTraceDBExtendedFamilies" {
					ast.Inspect(composite, func(child ast.Node) bool {
						if ident, ok := child.(*ast.Ident); ok {
							if ident.Name == "exportTraceDBFrameSlice" || ident.Name == "exportTraceDBNativeHook" {
								compositeRefs[ident.Name]++
							}
							if ident.Name == "exportTraceDBProcessMeasures" {
								processMeasureDispatch = ident.Pos()
							}
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
					calls[name] = append(calls[name], callSite{function: function.Name.Name, call: call})
				}
				if function.Name.Name == "exportTraceDBExtendedFamilies" {
					switch name {
					case "exportTraceDBCallstack":
						callstackDispatch = call.Pos()
					case "exportTraceDBFrameSlice":
						frameDispatch = call.Pos()
					case "exportTraceDBDMAFence":
						dmaDispatch = call.Pos()
					case "exportTraceDBStaticInitialize":
						staticDispatch = call.Pos()
					case "exportTraceDBNativeHook":
						nativeDispatch = call.Pos()
					case "exportTraceDBProcessMeasures":
						processMeasureDispatch = call.Pos()
					}
				}
				return true
			})
		}
	}

	callerCounts := func(name string) map[string]int {
		out := map[string]int{}
		for _, site := range calls[name] {
			out[site.function]++
		}
		return out
	}
	for function, caller := range map[string]string{
		"exportTraceDBFrameSlice":       "exportTraceDBExtendedFamilies",
		"prepareTraceDBFrameSliceRow":   "exportTraceDBFrameSlice",
		"exportTraceDBNativeHook":       "exportTraceDBExtendedFamilies",
		"prepareTraceDBNativeHookEvent": "exportTraceDBNativeHook",
	} {
		if !reflect.DeepEqual(callerCounts(function), map[string]int{caller: 1}) {
			t.Fatalf("%s production callers=%v", function, callerCounts(function))
		}
	}
	if compositeRefs["exportTraceDBFrameSlice"] != 0 || compositeRefs["exportTraceDBNativeHook"] != 0 {
		t.Fatalf("typed exporters escaped into generic dispatch: %v", compositeRefs)
	}
	for _, function := range []string{"exportTraceDBFrameSlice", "prepareTraceDBFrameSliceRow", "exportTraceDBNativeHook", "prepareTraceDBNativeHookEvent"} {
		got := functionTypes[function]
		if got["traceDBSchedulerAuthority"] != 1 || got["traceDBSchedulerRunningIndex"] != 1 ||
			got["traceDBThreadIndex"] != 0 || got["map"] != 0 {
			t.Fatalf("%s typed authority parameters=%v", function, got)
		}
		wantDuplicateMap := 0
		if strings.HasPrefix(function, "prepare") {
			wantDuplicateMap = 1
		}
		if got["map[int64]bool"] != wantDuplicateMap {
			t.Fatalf("%s duplicate cohort parameter types=%v", function, got)
		}
	}
	if !(callstackDispatch < frameDispatch && frameDispatch < dmaDispatch) ||
		!(staticDispatch < nativeDispatch && nativeDispatch < processMeasureDispatch) {
		t.Fatalf("extended typed dispatch order callstack=%d frame=%d dma=%d static=%d native=%d process=%d",
			callstackDispatch, frameDispatch, dmaDispatch, staticDispatch, nativeDispatch, processMeasureDispatch)
	}

	frameClosed := 0
	frameRunning := map[string]int{}
	for _, site := range calls["threadClosedEndpointAllows"] {
		if site.function != "prepareTraceDBFrameSliceRow" {
			continue
		}
		frameClosed++
		if receiverName(site.call) != "authority" || len(site.call.Args) != 3 ||
			!isSelector(site.call.Args[0], "frame", "ITID") || !isSelector(site.call.Args[1], "frame", "TS") ||
			!isSelector(site.call.Args[2], "frame", "End") {
			t.Fatal("frame closed endpoint gate arguments changed")
		}
	}
	for _, site := range calls["lookupCPUAt"] {
		if site.function != "prepareTraceDBFrameSliceRow" {
			continue
		}
		if receiverName(site.call) != "running" || len(site.call.Args) != 2 || !isSelector(site.call.Args[0], "frame", "ITID") {
			t.Fatal("frame Running lookup authority or identity changed")
		}
		switch {
		case isSelector(site.call.Args[1], "frame", "TS"):
			frameRunning["start"]++
		case isSelector(site.call.Args[1], "frame", "End"):
			frameRunning["end"]++
		default:
			t.Fatal("frame Running lookup must use exact TS or End")
		}
	}
	if frameClosed != 1 || !reflect.DeepEqual(frameRunning, map[string]int{"start": 1, "end": 1}) {
		t.Fatalf("frame endpoint authority closed=%d running=%v", frameClosed, frameRunning)
	}
	frameProfileDecodes := map[string]int{}
	for _, site := range calls["decode"] {
		if site.function != "prepareTraceDBFrameSliceRow" {
			continue
		}
		method, ok := site.call.Fun.(*ast.SelectorExpr)
		profile, profileOK := method.X.(*ast.SelectorExpr)
		if !ok || !profileOK || method.Sel.Name != "decode" || !isIdent(profile.X, "authority") ||
			profile.Sel.Name != "frameProfile" || len(site.call.Args) != 1 {
			t.Fatal("frame producer field bypassed the collector-selected decoder")
		}
		argument, ok := site.call.Args[0].(*ast.Ident)
		if !ok {
			t.Fatal("frame profile decoder argument is not an exact raw field")
		}
		frameProfileDecodes[argument.Name]++
	}
	if !reflect.DeepEqual(frameProfileDecodes, map[string]int{"vsyncRaw": 1, "ipidRaw": 1, "itidRaw": 1}) {
		t.Fatalf("frame collector-selected decoder closure=%v", frameProfileDecodes)
	}

	nativePoint, nativeRunning := 0, 0
	for _, site := range calls["threadPointAllows"] {
		if site.function != "prepareTraceDBNativeHookEvent" {
			continue
		}
		nativePoint++
		if receiverName(site.call) != "authority" || len(site.call.Args) != 2 ||
			!isSelector(site.call.Args[0], "event", "EmitterITID") || !isSelector(site.call.Args[1], "event", "TS") {
			t.Fatal("native origin point gate arguments changed")
		}
	}
	for _, site := range calls["lookupCPUAt"] {
		if site.function != "prepareTraceDBNativeHookEvent" {
			continue
		}
		nativeRunning++
		if receiverName(site.call) != "running" || len(site.call.Args) != 2 ||
			!isSelector(site.call.Args[0], "event", "EmitterITID") || !isSelector(site.call.Args[1], "event", "TS") {
			t.Fatal("native CPU lookup must use exact origin")
		}
	}
	if nativePoint != 1 || nativeRunning != 1 {
		t.Fatalf("native origin authority point=%d running=%d", nativePoint, nativeRunning)
	}

	for _, function := range []string{"exportTraceDBFrameSlice", "prepareTraceDBFrameSliceRow", "exportTraceDBNativeHook", "prepareTraceDBNativeHookEvent"} {
		for _, forbidden := range []string{"traceDBExtendedRunningCPUAt", "traceDBKnownCPUAt", "traceDBActivityProfile", "loadThreadIndex", "collectTraceDBLifecycle", "loadRunningIntervals", "loadSchedulerRunningIndex", "loadExtendedLegacyRunningIntervals"} {
			if callerCounts(forbidden)[function] != 0 {
				t.Fatalf("%s reopened forbidden authority %s: %v", function, forbidden, callerCounts(forbidden))
			}
		}
	}
	if !reflect.DeepEqual(callerCounts("traceDBExtendedRunningCPUAt"), map[string]int{"exportTraceDBRawFtraceFamilies": 1}) {
		t.Fatalf("legacy Running lookup escaped raw-only boundary: %v", callerCounts("traceDBExtendedRunningCPUAt"))
	}

	extendedBuilds := []callSite{}
	for _, site := range calls["newTraceDBSchedulerRunningIndex"] {
		if site.function == "exportTraceDBExtendedFamilies" {
			extendedBuilds = append(extendedBuilds, site)
		}
	}
	if len(extendedBuilds) != 1 {
		t.Fatalf("extended typed Running builds=%d", len(extendedBuilds))
	}
	for _, name := range []string{"exportTraceDBFrameSlice", "exportTraceDBNativeHook"} {
		call := calls[name][0].call
		if len(call.Args) != 5 || !isIdent(call.Args[3], "authority") || !isIdent(call.Args[4], "lifecycleRunning") {
			t.Fatalf("%s does not receive the shared typed values", name)
		}
	}
}
