package tool

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const defaultVerificationProbeLanguage = "python"

// verificationProbeAuthoringBoundary is injected into every provider-visible
// probe schema. Keep the optional-field escape here, next to the runtime
// registry, so adding a runtime cannot leave the planner teaching a stale
// workaround. A probe is source-level executable evidence, not a generic
// command wrapper.
const verificationProbeAuthoringBoundary = "Emit this optional field only when one listed runtime can directly import or execute the changed production behavior. If it cannot, omit verification_probes and put the native build/test command in acceptance_tests for project verification; do not launch an external compiler or test runner from a listed-runtime wrapper merely to bypass the language enum."

type verificationProbeRuntimeSpec struct {
	Language    string
	Aliases     []string
	Description string
}

var verificationProbeRuntimeSpecs = []verificationProbeRuntimeSpec{
	{Language: "python", Aliases: []string{"py"}, Description: "python"},
	{Language: "javascript", Aliases: []string{"js", "node"}, Description: "javascript (Node.js)"},
	{Language: "ruby", Aliases: []string{"rb"}, Description: "ruby"},
	{Language: "java", Aliases: []string{"javac"}, Description: "java (JDK javac/java)"},
	{Language: "go", Aliases: []string{"golang"}, Description: "go"},
}

func supportedVerificationProbeLanguages() []string {
	out := make([]string, 0, len(verificationProbeRuntimeSpecs))
	for _, spec := range verificationProbeRuntimeSpecs {
		out = append(out, spec.Language)
	}
	return out
}

func supportedVerificationProbeLanguageSet() map[string][]string {
	return map[string][]string{
		"$.verification_probes[].language": supportedVerificationProbeLanguages(),
	}
}

func supportedVerificationProbeLanguageList() string {
	return strings.Join(supportedVerificationProbeLanguages(), ", ")
}

func supportedVerificationProbeRuntimeDescription() string {
	parts := make([]string, 0, len(verificationProbeRuntimeSpecs))
	for _, spec := range verificationProbeRuntimeSpecs {
		parts = append(parts, spec.Description)
	}
	return strings.Join(parts, ", ")
}

// VerificationProbeRuntimeAvailable reports whether the exact interpreter(s)
// used by an inline probe language are discoverable before a controller asks
// the planner to author a mandatory proof. Source-language compatibility and
// runtime presence are separate authorities: JavaScript may exercise a
// TypeScript package, but that opportunity does not exist when node is absent.
// Repo roots are consulted only for the Python virtualenv layouts supported by
// the executor. Module/loader/import failures remain execution-time typed
// unavailable observations.
func VerificationProbeRuntimeAvailable(language string, roots ...string) bool {
	language, ok := normalizeVerificationProbeLanguage(language)
	if !ok {
		return false
	}
	switch language {
	case "python":
		for _, root := range roots {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			for _, dir := range []string{".venv", "venv", "env", ".virtualenv"} {
				for _, sub := range []string{filepath.Join("bin", "python"), filepath.Join("bin", "python3"), filepath.Join("Scripts", "python.exe")} {
					if _, err := exec.LookPath(filepath.Join(root, dir, sub)); err == nil {
						return true
					}
				}
			}
		}
		return verificationProbeExecutableOnPath("python3") || verificationProbeExecutableOnPath("python")
	case "javascript":
		return verificationProbeExecutableOnPath("node")
	case "ruby":
		return verificationProbeExecutableOnPath("ruby")
	case "java":
		return verificationProbeExecutableOnPath("javac") && verificationProbeExecutableOnPath("java")
	case "go":
		return verificationProbeExecutableOnPath("go")
	default:
		return false
	}
}

func verificationProbeExecutableOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func verificationProbeLanguageEnumJSON() string {
	data, err := json.Marshal(supportedVerificationProbeLanguages())
	if err != nil {
		return `["python","javascript","ruby","go"]`
	}
	return string(data)
}

func injectVerificationProbeLanguageSchema(schema string) json.RawMessage {
	schema = strings.ReplaceAll(schema, "__VERIFICATION_PROBE_LANGUAGE_ENUM__", verificationProbeLanguageEnumJSON())
	schema = strings.ReplaceAll(schema, "__VERIFICATION_PROBE_LANGUAGE_DESCRIPTION__", supportedVerificationProbeRuntimeDescription())
	schema = strings.ReplaceAll(schema, "__VERIFICATION_PROBE_AUTHORING_BOUNDARY__", verificationProbeAuthoringBoundary)
	return json.RawMessage(schema)
}

func normalizeVerificationProbeLanguage(raw string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return defaultVerificationProbeLanguage, true
	}
	for _, spec := range verificationProbeRuntimeSpecs {
		if key == spec.Language {
			return spec.Language, true
		}
		for _, alias := range spec.Aliases {
			if key == alias {
				return spec.Language, true
			}
		}
	}
	return "", false
}

func verificationProbeHasExecutableFailureSignal(language, code string) bool {
	switch language {
	case "python":
		return pythonVerificationProbeHasExecutableFailureSignal(code)
	case "javascript":
		return javascriptVerificationProbeHasExecutableFailureSignal(code)
	case "ruby":
		return rubyVerificationProbeHasExecutableFailureSignal(code)
	case "java":
		return javaVerificationProbeHasExecutableFailureSignal(code)
	case "go":
		return goVerificationProbeHasExecutableFailureSignal(code)
	default:
		return false
	}
}

func javascriptVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripCLikeProbeStringsAndComments(code))
	for _, signal := range []string{
		"assert(",
		"assert.",
		"console.assert(",
		"throw ",
		"throw(",
		"process.exit(",
	} {
		if strings.Contains(surface, compactProbeSignalSurface(signal)) {
			return true
		}
	}
	return false
}

func rubyVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripRubyProbeStringsAndComments(code))
	for _, signal := range []string{
		"raise ",
		"raise(",
		"fail ",
		"fail(",
		"abort(",
		"exit(",
	} {
		if strings.Contains(surface, compactProbeSignalSurface(signal)) {
			return true
		}
	}
	return false
}

func javaVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripCLikeProbeStringsAndComments(code))
	for _, signal := range []string{
		"assert ",
		"assert(",
		"throw ",
		"throw new",
		"System.exit(",
		"Assertions.",
		"Assert.",
		"assertEquals(",
		"assertTrue(",
		"assertFalse(",
		"fail(",
	} {
		if strings.Contains(surface, compactProbeSignalSurface(signal)) {
			return true
		}
	}
	return false
}

func goVerificationProbeHasExecutableFailureSignal(code string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), "codrax_verification_probe.go", code, 0)
	if err != nil || file == nil {
		return false
	}
	packageAliases := map[string]string{}
	for _, spec := range file.Imports {
		if spec == nil || spec.Path == nil {
			continue
		}
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path == "" {
			continue
		}
		alias := path
		if idx := strings.LastIndexByte(alias, '/'); idx >= 0 {
			alias = alias[idx+1:]
		}
		if spec.Name != nil {
			alias = strings.TrimSpace(spec.Name.Name)
		}
		if alias != "" && alias != "_" && alias != "." {
			packageAliases[alias] = path
		}
	}
	testingParams := goTestingParameterObjects(file, packageAliases)
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || call == nil {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			found = fun != nil && fun.Name == "panic" && fun.Obj == nil
		case *ast.SelectorExpr:
			qualifier, ok := fun.X.(*ast.Ident)
			if !ok || qualifier == nil || fun.Sel == nil {
				return true
			}
			switch {
			case qualifier.Obj == nil && packageAliases[qualifier.Name] == "os":
				found = fun.Sel.Name == "Exit"
			case qualifier.Obj == nil && packageAliases[qualifier.Name] == "log":
				found = fun.Sel.Name == "Fatal" || fun.Sel.Name == "Fatalf" || fun.Sel.Name == "Fatalln"
			default:
				if _, ok := testingParams[qualifier.Obj]; ok {
					switch fun.Sel.Name {
					case "Error", "Errorf", "Fail", "FailNow", "Fatal", "Fatalf":
						found = true
					}
				}
			}
		}
		return !found
	})
	return found
}

func goTestingParameterObjects(file *ast.File, packageAliases map[string]string) map[*ast.Object]struct{} {
	out := map[*ast.Object]struct{}{}
	if file == nil {
		return out
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !goTestFunctionDecl(fn, packageAliases) {
			continue
		}
		param := fn.Type.Params.List[0]
		if len(param.Names) == 1 && param.Names[0] != nil && param.Names[0].Obj != nil {
			out[param.Names[0].Obj] = struct{}{}
		}
	}
	return out
}

// goSamePackageTestProbe parses a Go verification probe as a real _test.go
// source file. This is the typed carrier for command packages and unexported
// symbols, which cannot be exercised by the ordinary external-import probe.
// It deliberately accepts only go-test-recognized TestX(*testing.T)
// declarations so syntactically plausible but unexecuted helpers cannot pass;
// every accepted declaration is returned for the bounded runner to execute.
func goSamePackageTestProbe(code string) (packageName string, testNames []string, ok bool) {
	file, err := parser.ParseFile(token.NewFileSet(), "codrax_verification_probe_test.go", code, 0)
	if err != nil || file == nil || file.Name == nil || strings.TrimSpace(file.Name.Name) == "" {
		return "", nil, false
	}
	packageAliases := map[string]string{}
	for _, spec := range file.Imports {
		if spec == nil || spec.Path == nil {
			continue
		}
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path == "" {
			continue
		}
		alias := path
		if idx := strings.LastIndexByte(alias, '/'); idx >= 0 {
			alias = alias[idx+1:]
		}
		if spec.Name != nil {
			alias = strings.TrimSpace(spec.Name.Name)
		}
		if alias != "" && alias != "_" && alias != "." {
			packageAliases[alias] = path
		}
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && goTestFunctionDecl(fn, packageAliases) {
			testNames = append(testNames, fn.Name.Name)
		}
	}
	if len(testNames) == 0 {
		return "", nil, false
	}
	return file.Name.Name, testNames, true
}

func goTestFunctionDecl(fn *ast.FuncDecl, packageAliases map[string]string) bool {
	if fn == nil || fn.Recv != nil || fn.Name == nil || fn.Body == nil || !goTestFunctionName(fn.Name.Name) {
		return false
	}
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || fn.Type.Results != nil && len(fn.Type.Results.List) != 0 {
		return false
	}
	param := fn.Type.Params.List[0]
	if param == nil || len(param.Names) > 1 {
		return false
	}
	star, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "T" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && packageAliases[ident.Name] == "testing"
}

func goTestFunctionName(name string) bool {
	if !strings.HasPrefix(name, "Test") || len(name) == len("Test") {
		return false
	}
	next, _ := utf8.DecodeRuneInString(name[len("Test"):])
	return next != 0 && !unicode.IsLower(next)
}

func compactProbeSignalSurface(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if isPythonProbeASCIILetter(ch) || isPythonProbeASCIIDigit(ch) || ch == '_' || ch == '.' || ch == '(' || ch == ' ' {
			b.WriteByte(ch)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func stripCLikeProbeStringsAndComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	quote := byte(0)
	escaped := false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if inString {
			if ch == '\n' {
				b.WriteByte('\n')
				if quote != '`' {
					inString = false
				}
				escaped = false
				continue
			}
			b.WriteByte(' ')
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < len(src) {
			next := src[i+1]
			if next == '/' {
				b.WriteString("  ")
				i += 2
				for i < len(src) && src[i] != '\n' {
					b.WriteByte(' ')
					i++
				}
				if i < len(src) && src[i] == '\n' {
					b.WriteByte('\n')
				}
				continue
			}
			if next == '*' {
				b.WriteString("  ")
				i += 2
				for i < len(src) {
					if src[i] == '\n' {
						b.WriteByte('\n')
					} else {
						b.WriteByte(' ')
					}
					if i+1 < len(src) && src[i] == '*' && src[i+1] == '/' {
						b.WriteByte(' ')
						i++
						break
					}
					i++
				}
				continue
			}
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			inString = true
			quote = ch
			escaped = false
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func stripRubyProbeStringsAndComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	quote := byte(0)
	escaped := false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if inString {
			if ch == '\n' {
				b.WriteByte('\n')
				if quote != '`' {
					inString = false
				}
				escaped = false
				continue
			}
			b.WriteByte(' ')
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		if ch == '#' {
			for i < len(src) && src[i] != '\n' {
				b.WriteByte(' ')
				i++
			}
			if i < len(src) && src[i] == '\n' {
				b.WriteByte('\n')
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			inString = true
			quote = ch
			escaped = false
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
