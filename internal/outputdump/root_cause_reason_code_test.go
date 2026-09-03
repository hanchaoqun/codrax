package outputdump

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// root_cause_reason_code_test.go — §40.44 residual (a): the customer-facing
// `reason_code` vocabulary of the `.root-causes.json` sidecar is ONE closed,
// append-only list (root_cause_reason_code.go). Two pins keep it single-source:
//
//  1. GUIDE TABLE — the implementation guide's reason_code table lists exactly
//     the codes of AllRootCauseUnavailableReasonCodes, in the same order (a new
//     code without its customer meaning, or a documented code the program
//     cannot emit, is red).
//  2. PRODUCER CENSUS — repo-wide over every non-test Go file, no string literal
//     spells one of the codes outside the constant file, so every producer
//     names the constant (a producer that inlines a code — the pre-§40.44
//     shape at five sites — is red). Test files may spell the wire strings on
//     purpose (they pin the wire).
//
// Each has a self-red fixture arm.

const rootCauseReasonCodeGuide = "docs/guides/trace_short_root_cause_implementation_zh.md"

// reasonCodeTableRows extracts the code column of the guide's
// `| reason_code | 含义 |` table: the first cell of each row after the header
// and its separator, with the backticks stripped.
func reasonCodeTableRows(guide string) []string {
	lines := strings.Split(guide, "\n")
	var rows []string
	for i, line := range lines {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		// The HEADER row (followed by the `|---|---|` separator), not a
		// `reason_code` row inside the sidecar field table.
		if len(cells) < 2 || strings.TrimSpace(cells[0]) != "`reason_code`" || i+1 >= len(lines) || !isMarkdownTableSeparator(lines[i+1]) {
			continue
		}
		for _, row := range lines[i+2:] {
			row = strings.TrimSpace(row)
			if !strings.HasPrefix(row, "|") {
				break
			}
			first := strings.TrimSpace(strings.Split(strings.Trim(row, "|"), "|")[0])
			rows = append(rows, strings.Trim(first, "`"))
		}
		break
	}
	return rows
}

func isMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return false
	}
	return strings.Trim(line, "|:- ") == ""
}

func TestRootCauseReasonCodesMatchTheGuideTable(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", rootCauseReasonCodeGuide))
	if err != nil {
		t.Fatal(err)
	}
	got := reasonCodeTableRows(string(body))
	want := AllRootCauseUnavailableReasonCodes()
	if len(want) < 8 || len(got) == 0 {
		t.Fatalf("vacuous: program list %v, guide table %v", want, got)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the guide's reason_code table must list exactly the program's closed vocabulary in order:\nguide   %v\nprogram %v", got, want)
	}
	seen := map[string]bool{}
	for _, code := range want {
		if code == "" || seen[code] {
			t.Fatalf("closed list must be non-empty and duplicate-free: %v", want)
		}
		seen[code] = true
	}
}

// Self-red: a table missing one code, or carrying an undocumented one, is
// detected by the same extractor.
func TestRootCauseReasonCodeGuideExtractorFlagsDrift(t *testing.T) {
	want := AllRootCauseUnavailableReasonCodes()
	build := func(codes []string) string {
		var b strings.Builder
		b.WriteString("intro\n\n| 字段 | 含义 |\n|---|---|\n| `status` | x |\n| `reason_code` | y |\n\n| `reason_code` | 含义 |\n|---|---|\n")
		for _, code := range codes {
			b.WriteString("| `" + code + "` | meaning |\n")
		}
		b.WriteString("\ntrailing prose\n")
		return b.String()
	}
	if got := reasonCodeTableRows(build(want)); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("canonical table must round-trip: %v", got)
	}
	missing := reasonCodeTableRows(build(want[1:]))
	if strings.Join(missing, "\n") == strings.Join(want, "\n") {
		t.Fatal("a table missing a code must not equal the program list")
	}
	extra := reasonCodeTableRows(build(append(append([]string{}, want...), "made_up_reason")))
	if strings.Join(extra, "\n") == strings.Join(want, "\n") {
		t.Fatal("a table with an undocumented code must not equal the program list")
	}
}

// reasonCodeLiteralOffenders returns every string literal in the given files
// that spells one of the codes, except inside the constant file itself.
func reasonCodeLiteralOffenders(fset *token.FileSet, files map[string]*ast.File, codes []string) (offenders []string, consumers int) {
	set := map[string]bool{}
	for _, code := range codes {
		set[code] = true
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		owner := filepath.Base(name) == "root_cause_reason_code.go"
		ast.Inspect(files[name], func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BasicLit:
				if node.Kind != token.STRING || owner {
					return true
				}
				if value, err := strconv.Unquote(node.Value); err == nil && set[value] {
					offenders = append(offenders, fset.Position(node.Pos()).String()+" spells "+node.Value+" — name the outputdump.RootCauseReason* constant")
				}
			case *ast.Ident:
				if !owner && strings.HasPrefix(node.Name, "RootCauseReason") {
					consumers++
				}
			}
			return true
		})
	}
	return offenders, consumers
}

func TestRootCauseReasonCodeProducersNameTheConstant(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found at %s: %v", root, err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			// Only the module's source trees are producers: root-level
			// files, internal/ and cmd/. Sibling directories such as
			// eval/ hold fixture repos with deliberately broken Go.
			if filepath.Dir(path) == root && name != "internal" && name != "cmd" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		files[path] = f
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 100 {
		t.Fatalf("repo scan found only %d non-test Go files — the census scan is broken", len(files))
	}
	offenders, consumers := reasonCodeLiteralOffenders(fset, files, AllRootCauseUnavailableReasonCodes())
	for _, offender := range offenders {
		t.Errorf("reason_code inlined outside the closed list: %s", offender)
	}
	// Vacuity guard: the live producers (orchestrator finalize / default dump,
	// CLI explicit report, this package's two fallbacks and the encoding
	// failure) reference the constants.
	if consumers < 8 {
		t.Fatalf("expected the producers to name the constants (>= 8 references), found %d", consumers)
	}
}

// Self-red: an inlined code in a producer file is flagged; the constant file
// itself is exempt.
func TestRootCauseReasonCodeCensusFlagsInlinedLiteral(t *testing.T) {
	fset := token.NewFileSet()
	parse := func(name, src string) *ast.File {
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	codes := AllRootCauseUnavailableReasonCodes()
	files := map[string]*ast.File{
		"root_cause_reason_code.go": parse("root_cause_reason_code.go", `package outputdump
const RootCauseReasonSelectionRejected = "`+RootCauseReasonSelectionRejected+`"`),
		"producer.go": parse("producer.go", `package other
func reason() string { return "`+RootCauseReasonSelectionRejected+`" }`),
	}
	offenders, consumers := reasonCodeLiteralOffenders(fset, files, codes)
	if len(offenders) != 1 || !strings.Contains(offenders[0], "producer.go") || consumers != 0 {
		t.Fatalf("the inlined producer must be the one offender: %v (consumers %d)", offenders, consumers)
	}
	files["producer.go"] = parse("producer.go", `package other
import "x/outputdump"
func reason() string { return outputdump.RootCauseReasonSelectionRejected }`)
	if offenders, consumers := reasonCodeLiteralOffenders(fset, files, codes); len(offenders) != 0 || consumers != 1 {
		t.Fatalf("naming the constant must pass: %v (consumers %d)", offenders, consumers)
	}
}
