package hitraceconv

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleasePrivateConversionDirProviderSingleAuthorityStructure(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve release contract source path")
	}
	dir := filepath.Dir(current)
	cases := []struct {
		file     string
		function string
	}{
		{file: "simpleperf_text.go", function: "maybeConvertSimpleperfPerfData"},
		{file: "hiperf_proto.go", function: "maybeConvertHiperfPerfDataFromInput"},
		{file: "trace_streamer_provider.go", function: "prepareTraceStreamerDBTarget"},
	}
	for _, tc := range cases {
		t.Run(tc.function, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			var declaration *ast.FuncDecl
			for _, item := range parsed.Decls {
				candidate, ok := item.(*ast.FuncDecl)
				if ok && candidate.Name.Name == tc.function {
					declaration = candidate
					break
				}
			}
			if declaration == nil || declaration.Body == nil {
				t.Fatalf("function %s not found in %s", tc.function, path)
			}
			authorityCalls := 0
			ast.Inspect(declaration.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "newPrivateConversionDir" {
					authorityCalls++
				}
				return true
			})
			wantAuthorityCalls := 1
			if tc.function == "maybeConvertHiperfPerfDataFromInput" {
				wantAuthorityCalls = 0
			}
			if authorityCalls != wantAuthorityCalls {
				t.Fatalf("%s private directory authority calls=%d, want %d", tc.function, authorityCalls, wantAuthorityCalls)
			}
			var body bytes.Buffer
			if err := format.Node(&body, fset, declaration.Body); err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"os.MkdirTemp", ".Mode().Perm()", "os.Chmod"} {
				if strings.Contains(body.String(), forbidden) {
					t.Fatalf("%s reintroduced platform-specific private-directory check %q", tc.function, forbidden)
				}
			}
			bodyText := body.String()
			normalizedBodyText := strings.Join(strings.Fields(bodyText), " ")
			switch tc.function {
			case "maybeConvertSimpleperfPerfData":
				for _, required := range []string{
					".ChildPath(", ".Validate()", ".FinalizeCleanup()",
					"newExternalToolInputLeaseWithPublicProgress(",
					"inputLease.Command(",
					"runCommandWithProgressUntilExit(",
					"finishExternalToolCommand(ctx, inputLease, reportDir, runErr)",
					`beforeInput := []string{"-i"}`,
					`beforeInput = []string{tool, "-i"}`,
					`afterInput := []string{"-o", reportPath}`,
				} {
					if !strings.Contains(normalizedBodyText, required) {
						t.Fatalf("%s no longer consumes its single private-directory/input authority through %q", tc.function, required)
					}
				}
				for _, forbidden := range []string{"privateConversionDirCommandBoundaryError(", "exec.CommandContext(", `[]string{"-i", perfPath`} {
					if strings.Contains(normalizedBodyText, forbidden) {
						t.Fatalf("%s regained public-path/staging-only command construction %q", tc.function, forbidden)
					}
				}
				leaseAt := strings.Index(normalizedBodyText, "newExternalToolInputLeaseWithPublicProgress(")
				commandAt := strings.Index(normalizedBodyText, "inputLease.Command(")
				runAt := strings.Index(normalizedBodyText, "runCommandWithProgressUntilExit(")
				finishAt := strings.Index(normalizedBodyText, "finishExternalToolCommand(ctx, inputLease, reportDir, runErr)")
				fallbackAt := strings.Index(normalizedBodyText, "if runErr != nil")
				adoptAt := strings.Index(normalizedBodyText, `reportDir.AdoptRegularChild("report_sample.txt", true)`)
				if !(leaseAt < commandAt && commandAt < runAt && runAt < finishAt && finishAt < fallbackAt && fallbackAt < adoptAt) {
					t.Fatalf("%s lease/command/boundary/fallback/adopt order drifted:\n%s", tc.function, normalizedBodyText)
				}
			case "maybeConvertHiperfPerfDataFromInput":
				for _, required := range []string{"adapterDir.ChildPath(", "adapterDir.Validate()", "inputLease.Command(", "runCommandWithProgressUntilExit(", "validateExternalToolCommandBoundary(ctx, inputLease, adapterDir, runErr)", `adapterDir.AdoptRegularChild("report_sample.proto", true)`} {
					if !strings.Contains(normalizedBodyText, required) {
						t.Fatalf("%s no longer consumes its single private-directory authority through %q", tc.function, required)
					}
				}
				for _, forbidden := range []string{"newPrivateConversionDir(", ".FinalizeCleanup()", "privateConversionDirCommandBoundaryError(", "exec.CommandContext("} {
					if strings.Contains(normalizedBodyText, forbidden) {
						t.Fatalf("%s regained a second/private-path authority through %q", tc.function, forbidden)
					}
				}
			case "prepareTraceStreamerDBTarget":
				for _, required := range []string{"stagingDir.ChildPath(", "stagingDir.FinalizeCleanup", "stagingDir: stagingDir"} {
					if !strings.Contains(normalizedBodyText, required) {
						t.Fatalf("%s no longer ferries its single private-directory authority through %q", tc.function, required)
					}
				}
				if count := strings.Count(normalizedBodyText, "stagingDir.FinalizeCleanup"); count != 2 {
					t.Fatalf("%s terminal cleanup uses=%d, want child-path failure plus target cleanup", tc.function, count)
				}
			}
		})
	}

	traceStreamerRun := releasePrivateConversionDirFunctionBody(t, filepath.Join(dir, "trace_streamer_provider.go"), "runTraceStreamerExport")
	for _, required := range []string{"dbTarget.validateStaging()", "newExternalToolInputLeaseWithProgress(", "finishExternalToolCommand(ctx, inputLease, dbTarget.stagingDir, runErr)", "cleanupTraceStreamerDBTarget(cleanup)"} {
		if !strings.Contains(traceStreamerRun, required) {
			t.Fatalf("runTraceStreamerExport no longer consumes the ferried staging authority through %q", required)
		}
	}
	if strings.Contains(traceStreamerRun, "privateConversionDirCommandBoundaryError(") {
		t.Fatalf("trace_streamer regained the staging-only command boundary instead of the input lease boundary:\n%s", traceStreamerRun)
	}
}

func TestReleasePrivateConversionDirDarwinPointerEscapeBoundaryStructure(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve release contract source path")
	}
	path := filepath.Join(filepath.Dir(current), "private_conversion_dir_unix_security_darwin.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	var authority *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		candidate, ok := declaration.(*ast.FuncDecl)
		if ok && candidate.Name.Name == "privateConversionDirDarwinIntCall" {
			authority = candidate
			break
		}
	}
	if authority == nil || authority.Doc == nil {
		t.Fatal("Darwin integer-call authority or its compiler contract is missing")
	}
	directives := 0
	for _, comment := range authority.Doc.List {
		if comment.Text == "//go:uintptrescapes" {
			directives++
		}
	}
	if directives != 1 {
		t.Fatalf("Darwin integer-call uintptr escape directives=%d, want exactly 1", directives)
	}

	type intCallContract struct {
		pointerArg       int
		scalarUintptrArg int
	}
	expectedIntCalls := map[string]intCallContract{
		"set Darwin filesec mode":                {pointerArg: 4, scalarUintptrArg: -1},
		"set Darwin filesec no-inherit ACL":      {pointerArg: 4, scalarUintptrArg: -1},
		"mkdirx_np private conversion directory": {pointerArg: 2, scalarUintptrArg: -1},
		"get Darwin ACL flagset":                 {pointerArg: 3, scalarUintptrArg: -1},
		"add Darwin ACL no-inherit flag":         {pointerArg: -1, scalarUintptrArg: -1},
		"set empty Darwin ACL on held directory": {pointerArg: -1, scalarUintptrArg: 2},
	}
	intCallCounts := make(map[string]int, len(expectedIntCalls))
	directConversions := make(map[*ast.CallExpr]bool, 4)
	scalarConversions := make(map[*ast.CallExpr]bool, 2)
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := call.Fun.(*ast.Ident)
		if !ok || callee.Name != "privateConversionDirDarwinIntCall" || len(call.Args) == 0 {
			return true
		}
		op, ok := call.Args[0].(*ast.BasicLit)
		if !ok || op.Kind != token.STRING {
			t.Fatalf("Darwin integer-call authority has a non-literal operation at %s", fset.Position(call.Pos()))
		}
		opName := strings.Trim(op.Value, "\"")
		contract, tracked := expectedIntCalls[opName]
		if !tracked {
			t.Fatalf("Darwin integer-call authority has an unregistered operation %q", opName)
		}
		intCallCounts[opName]++
		if intCallCounts[opName] != 1 {
			t.Fatalf("Darwin integer-call operation %q is duplicated", opName)
		}
		if contract.pointerArg >= 0 {
			if len(call.Args) <= contract.pointerArg {
				t.Fatalf("Darwin pointer-bearing call %s lost argument %d", op.Value, contract.pointerArg)
			}
			conversion, ok := call.Args[contract.pointerArg].(*ast.CallExpr)
			if !ok || !isDirectDarwinUnsafePointerUintptrConversion(conversion) {
				t.Fatalf("Darwin pointer-bearing call %s must convert unsafe.Pointer directly in the annotated authority argument", op.Value)
			}
			directConversions[conversion] = true
		}
		if contract.scalarUintptrArg >= 0 {
			if len(call.Args) <= contract.scalarUintptrArg {
				t.Fatalf("Darwin scalar call %s lost argument %d", op.Value, contract.scalarUintptrArg)
			}
			conversion, ok := call.Args[contract.scalarUintptrArg].(*ast.CallExpr)
			if !ok || !isDirectDarwinIdentifierUintptrConversion(conversion, "fd") {
				t.Fatalf("Darwin scalar call %s lost its direct uintptr(fd) conversion", op.Value)
			}
			scalarConversions[conversion] = true
		}
		return true
	})
	for op := range expectedIntCalls {
		if intCallCounts[op] != 1 {
			t.Fatalf("Darwin integer-call authority operation %q count=%d, want 1", op, intCallCounts[op])
		}
	}

	type lowLevelCall struct {
		callee string
		target string
	}
	expectedLowLevelCalls := map[lowLevelCall]int{
		{callee: "privateConversionDirDarwinLibcCallPtr", target: "privateConversionDirDarwinFileSecInitTrampolineAddr"}: 1,
		{callee: "privateConversionDirDarwinLibcCallPtr", target: "privateConversionDirDarwinACLInitTrampolineAddr"}:     2,
		{callee: "privateConversionDirDarwinLibcCallPtr", target: "privateConversionDirDarwinACLGetFDTrampolineAddr"}:    1,
		{callee: "privateConversionDirDarwinLibcCall", target: "privateConversionDirDarwinACLFreeTrampolineAddr"}:        1,
		{callee: "privateConversionDirDarwinLibcCall", target: "privateConversionDirDarwinFileSecFreeTrampolineAddr"}:    1,
		{callee: "privateConversionDirDarwinLibcCall", target: "fn"}:                                                     1,
	}
	lowLevelCallCounts := make(map[lowLevelCall]int, len(expectedLowLevelCalls))
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := call.Fun.(*ast.Ident)
		if !ok || (callee.Name != "privateConversionDirDarwinLibcCall" && callee.Name != "privateConversionDirDarwinLibcCallPtr") {
			return true
		}
		if len(call.Args) == 0 {
			t.Fatalf("Darwin low-level call %s has no target", callee.Name)
		}
		target, ok := call.Args[0].(*ast.Ident)
		if !ok {
			t.Fatalf("Darwin low-level call %s has a non-identifier target at %s", callee.Name, fset.Position(call.Pos()))
		}
		key := lowLevelCall{callee: callee.Name, target: target.Name}
		want, tracked := expectedLowLevelCalls[key]
		if !tracked {
			t.Fatalf("Darwin low-level call %s(%s, ...) is outside the closed authority", callee.Name, target.Name)
		}
		lowLevelCallCounts[key]++
		if lowLevelCallCounts[key] > want {
			t.Fatalf("Darwin low-level call %s(%s, ...) count exceeds %d", callee.Name, target.Name, want)
		}
		if key == (lowLevelCall{callee: "privateConversionDirDarwinLibcCallPtr", target: "privateConversionDirDarwinACLGetFDTrampolineAddr"}) {
			if len(call.Args) <= 1 {
				t.Fatal("Darwin ACL get-FD call lost its descriptor argument")
			}
			conversion, ok := call.Args[1].(*ast.CallExpr)
			if !ok || !isDirectDarwinIdentifierUintptrConversion(conversion, "fd") {
				t.Fatal("Darwin ACL get-FD call lost its direct uintptr(fd) conversion")
			}
			scalarConversions[conversion] = true
		}
		return true
	})
	for key, want := range expectedLowLevelCalls {
		if lowLevelCallCounts[key] != want {
			t.Fatalf("Darwin low-level call %s(%s, ...) count=%d, want %d", key.callee, key.target, lowLevelCallCounts[key], want)
		}
	}

	allConversions := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := call.Fun.(*ast.Ident)
		if !ok || callee.Name != "uintptr" {
			return true
		}
		allConversions++
		if !directConversions[call] && !scalarConversions[call] {
			t.Fatalf("Darwin uintptr conversion at %s is outside the closed direct-call boundary", fset.Position(call.Pos()))
		}
		return true
	})
	if allConversions != len(directConversions)+len(scalarConversions) {
		t.Fatalf("Darwin uintptr conversion census=%d, want %d", allConversions, len(directConversions)+len(scalarConversions))
	}

	entries, err := os.ReadDir(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == filepath.Base(path) {
			continue
		}
		candidatePath := filepath.Join(filepath.Dir(current), name)
		candidate, err := parser.ParseFile(token.NewFileSet(), candidatePath, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(candidate, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, ok := call.Fun.(*ast.Ident)
			if ok && (callee.Name == "privateConversionDirDarwinIntCall" || callee.Name == "privateConversionDirDarwinLibcCall" || callee.Name == "privateConversionDirDarwinLibcCallPtr") {
				t.Fatalf("Darwin libc escape authority %s has an out-of-file caller at %s", callee.Name, candidatePath)
			}
			return true
		})
	}
}

func isDirectDarwinUnsafePointerUintptrConversion(call *ast.CallExpr) bool {
	outer, ok := call.Fun.(*ast.Ident)
	if !ok || outer.Name != "uintptr" || len(call.Args) != 1 {
		return false
	}
	inner, ok := call.Args[0].(*ast.CallExpr)
	if !ok || len(inner.Args) != 1 {
		return false
	}
	selector, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Pointer" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Name == "unsafe"
}

func isDirectDarwinIdentifierUintptrConversion(call *ast.CallExpr, identifier string) bool {
	outer, ok := call.Fun.(*ast.Ident)
	if !ok || outer.Name != "uintptr" || len(call.Args) != 1 {
		return false
	}
	value, ok := call.Args[0].(*ast.Ident)
	return ok && value.Name == identifier
}

func TestReleaseSimpleperfExternalInputProfileIsSnapshotOnly(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve release contract source path")
	}
	dir := filepath.Dir(current)
	resolver := releasePrivateConversionDirFunctionBody(t, filepath.Join(dir, "simpleperf_text.go"), "resolveSimpleperfProviderTool")
	if strings.Count(resolver, "externalToolInputSnapshotOnly") != 1 || strings.Contains(resolver, "externalToolInputVerifiedLinuxFD") {
		t.Fatalf("simpleperf resolver profile is no longer exactly snapshot-only:\n%s", resolver)
	}
	provider := releasePrivateConversionDirFunctionBody(t, filepath.Join(dir, "simpleperf_text.go"), "maybeConvertSimpleperfPerfData")
	if !strings.Contains(provider, "resolution.ExternalInputProfile") {
		t.Fatalf("simpleperf provider stopped consuming the typed input profile:\n%s", provider)
	}
	helper := releasePrivateConversionDirFunctionBody(t, filepath.Join(dir, "external_tool_input_lease.go"), "newExternalToolInputLeaseWithPublicProgress")
	if !strings.Contains(helper, "profile != externalToolInputSnapshotOnly") {
		t.Fatalf("perf snapshot progress helper no longer rejects non-snapshot transports:\n%s", helper)
	}
}

func releasePrivateConversionDirFunctionBody(t *testing.T, path, function string) string {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range parsed.Decls {
		declaration, ok := item.(*ast.FuncDecl)
		if !ok || declaration.Name.Name != function || declaration.Body == nil {
			continue
		}
		var body bytes.Buffer
		if err := format.Node(&body, fset, declaration.Body); err != nil {
			t.Fatal(err)
		}
		return body.String()
	}
	t.Fatalf("function %s not found in %s", function, path)
	return ""
}

func TestReleasePrivateConversionDirPlatformImplementationStructure(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve release contract source path")
	}
	dir := filepath.Dir(current)
	read := func(name string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	commonBody := read("private_conversion_dir.go")
	for _, required := range []string{"dir.FinalizeCleanup()", "dir.closeHandlesLocked()", "privateConversionDirCommandBoundaryError"} {
		if !strings.Contains(commonBody, required) {
			t.Fatalf("common private-directory terminal authority lost %q", required)
		}
	}

	unixBody := read("private_conversion_dir_unix.go")
	for _, required := range []string{"createPrivateConversionDirUnixPlatform", "unix.Openat", "unix.O_NOFOLLOW", "unix.O_CLOEXEC", "unix.Fchmod(state.guardFD, 0o700)", "unix.Unlinkat", "unix.AT_REMOVEDIR"} {
		if !strings.Contains(unixBody, required) {
			t.Fatalf("POSIX private directory implementation lost %q", required)
		}
	}
	if strings.Contains(unixBody, "os.MkdirTemp") || strings.Contains(unixBody, "os.RemoveAll") {
		t.Fatal("POSIX private directory authority returned to public path create/remove")
	}
	for _, required := range []string{"unix.Fstat(state.guardFD", "os.Geteuid()", "securePrivateConversionDirUnixPlatform", "validatePrivateConversionDirUnixSecurityPlatform"} {
		if !strings.Contains(unixBody, required) {
			t.Fatalf("POSIX private directory exact-security gate lost %q", required)
		}
	}
	bindingIndex := strings.Index(unixBody, "os.Lstat(path)")
	birthSecurityIndex := strings.Index(unixBody, "validatePrivateConversionDirUnixBirthSecurityPlatform(state.guardFD)")
	birthOwnerIndex := strings.Index(unixBody, "birth owner/type mismatch")
	mutationIndex := strings.Index(unixBody, "unix.Fchmod(state.guardFD, 0o700)")
	if bindingIndex < 0 || birthSecurityIndex < 0 || birthOwnerIndex < 0 || mutationIndex < 0 || bindingIndex > mutationIndex || birthSecurityIndex > mutationIndex || birthOwnerIndex > mutationIndex {
		t.Fatal("POSIX private directory no longer binds held/public identity and Darwin birth security before mutation")
	}
	nonDarwinCreateBody := read("private_conversion_dir_unix_create_other.go")
	for _, required := range []string{"unix.Mkdirat(parentFD, leaf, 0o700)", "if !creatorBound", "unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR)"} {
		if !strings.Contains(nonDarwinCreateBody, required) {
			t.Fatalf("non-Darwin POSIX private directory creation authority lost %q", required)
		}
	}
	darwinBody := read("private_conversion_dir_unix_security_darwin.go")
	for _, required := range []string{
		"acl_init",
		"acl_set_fd_np",
		"acl_get_fd_np",
		"acl_get_flagset_np",
		"acl_add_flag_np",
		"filesec_set_property",
		"mkdirx_np",
		"privateConversionDirDarwinACLNoInherit",
		"filepath.Join(canonicalParent, leaf)",
		"if !creatorBound",
		"syscall.syscallPtr",
		"privateConversionDirDarwinLibcCallPtr",
		"if flags == 0",
		"acl_get_flagset_np returned NULL",
	} {
		if !strings.Contains(darwinBody, required) {
			t.Fatalf("Darwin private directory ACL authority lost %q", required)
		}
	}
	for _, forbidden := range []string{"pthread_fchdir_np", "runtime.LockOSThread"} {
		if strings.Contains(darwinBody, forbidden) {
			t.Fatalf("Darwin private directory returned to private/thread-cwd API %q", forbidden)
		}
	}

	windowsBody := read("private_conversion_dir_windows.go")
	for _, required := range []string{
		"windows.NtCreateFile",
		"RootDirectory:      parent",
		"SecurityDescriptor: sd",
		`"O:" + user.String() + "D:P`,
		"windows.FILE_CREATE",
		"windows.FILE_OPEN_REPARSE_POINT",
		"windows.WRITE_DAC",
		"windows.FILE_WRITE_ATTRIBUTES",
		"windows.PROTECTED_DACL_SECURITY_INFORMATION",
		"windows.SetSecurityInfo",
		"windows.FileFullDirectoryRestartInfo",
		"windows.SetFileInformationByHandle",
		"windows.FileDispositionInfo",
		"windows.FileBasicInfo",
		"windows.FILE_ATTRIBUTE_READONLY",
		"deleteFile := byte(1)",
		`clean = ` + "`" + `\\?\UNC\` + "`",
	} {
		if !strings.Contains(windowsBody, required) {
			t.Fatalf("Windows private directory implementation lost %q", required)
		}
	}
	for _, forbidden := range []string{
		"os.MkdirTemp",
		"os.Chmod",
		"os.OpenRoot",
		"os.RemoveAll",
		"windows.CreateDirectory",
		"windows.SetNamedSecurityInfo",
	} {
		if strings.Contains(windowsBody, forbidden) {
			t.Fatalf("Windows private directory implementation uses post-create/path-based permission mutation %q", forbidden)
		}
	}
}
