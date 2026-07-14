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
		{file: "hiperf_proto.go", function: "maybeConvertHiperfPerfData"},
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
			if authorityCalls != 1 {
				t.Fatalf("%s private directory authority calls=%d, want exactly one", tc.function, authorityCalls)
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
			case "maybeConvertSimpleperfPerfData", "maybeConvertHiperfPerfData":
				for _, required := range []string{".ChildPath(", ".Validate()", ".FinalizeCleanup()", "privateConversionDirCommandBoundaryError(ctx, runErr,"} {
					if !strings.Contains(normalizedBodyText, required) {
						t.Fatalf("%s no longer consumes its single private-directory authority through %q", tc.function, required)
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
	for _, required := range []string{"dbTarget.validateStaging()", "privateConversionDirCommandBoundaryError(ctx, runErr, dbTarget.stagingDir)", "cleanupTraceStreamerDBTarget(cleanup)"} {
		if !strings.Contains(traceStreamerRun, required) {
			t.Fatalf("runTraceStreamerExport no longer consumes the ferried staging authority through %q", required)
		}
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
