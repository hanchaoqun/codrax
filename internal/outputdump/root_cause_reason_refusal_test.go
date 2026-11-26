package outputdump

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// root_cause_reason_refusal_test.go — the capacity reason has one derivation
// and never leaves the side file.

func refusalRecord(t *testing.T, requestsSent int, earlierDispatchSent bool) *types.FinalizerRequestRefusal {
	t.Helper()
	bus := &types.BusContext{Mutable: types.NewMutableState("question")}
	if earlierDispatchSent {
		bus.Mutable.RecordFinalizeDispatchNotRefused()
	}
	inputs := types.SampleAnswerRequestInputs(bus, types.AnswerDispatchStamp{})
	return bus.Mutable.RecordFinalizeRequestRefusal(742912, 700000, requestsSent, inputs)
}

// The capacity reason is given only for a first request refused with nothing
// sent. A refused repair or follow-up request, and a run without a refusal,
// keep the reason they have today.
func TestUnformedAnswerReasonIsTheCapacityReasonOnlyWhenNothingWasSent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refusal *types.FinalizerRequestRefusal
		want    string
	}{
		{"no refusal", nil, "final_answer_transcript_not_available"},
		{"first request, nothing sent", refusalRecord(t, 0, false), "final_answer_request_exceeds_capacity"},
		{"repair request of the refused dispatch", refusalRecord(t, 1, false), "final_answer_transcript_not_available"},
		{"first request of a later dispatch", refusalRecord(t, 0, true), "final_answer_transcript_not_available"},
	} {
		if got := RootCauseReasonForUnformedAnswer(tc.refusal); got != tc.want {
			t.Errorf("%s: reason = %q, want %q", tc.name, got, tc.want)
		}
	}
	// A hand-built record that contradicts itself is not "nothing sent".
	if got := RootCauseReasonForUnformedAnswer(&types.FinalizerRequestRefusal{Kind: types.FinalizerRefusalFirstRequest, RequestsSent: 1}); got != RootCauseReasonTranscriptNotAvailable {
		t.Errorf("a record with requests sent must keep the transcript reason, got %q", got)
	}
	codes := AllRootCauseUnavailableReasonCodes()
	if codes[len(codes)-1] != "final_answer_request_exceeds_capacity" || codes[len(codes)-2] != "root_cause_selection_follow_up_not_sent" {
		t.Fatalf("the vocabulary is append-only; the capacity reason is its newest entry: %v", codes)
	}
}

func moduleGoFiles(t *testing.T, dirs ...string) map[string]*ast.File {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			files[filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))] = file
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// literalsSpellingAReasonCode returns the string literals that contain one
// of the codes anywhere in their text, outside the constant file. The
// producer census matches whole literals; a code embedded in a longer text
// (a prompt, a schema description, a hint) is what this one is for.
func literalsSpellingAReasonCode(files map[string]*ast.File, codes []string) (offenders []string, literals int) {
	for name, file := range files {
		if filepath.Base(name) == "root_cause_reason_code.go" {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			literals++
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, code := range codes {
				if strings.Contains(value, code) {
					offenders = append(offenders, name+" spells "+code)
				}
			}
			return true
		})
	}
	return offenders, literals
}

// Negative pin: a reason code is a side-file value for the customer's
// program. No text the program can show a model contains one. All
// model-facing text here is built from Go string literals (the module embeds
// no prompt assets), so the pin has two halves: no literal anywhere spells a
// code inside a longer text, and the packages that assemble model-facing
// text cannot name the constants because they do not import this package.
func TestNoReasonCodeAppearsInModelFacingText(t *testing.T) {
	files := moduleGoFiles(t, "internal", "cmd")
	if len(files) < 100 {
		t.Fatalf("vacuous scan: %d files", len(files))
	}
	offenders, literals := literalsSpellingAReasonCode(files, AllRootCauseUnavailableReasonCodes())
	if literals < 10000 {
		t.Fatalf("vacuous scan: %d string literals", literals)
	}
	for _, offender := range offenders {
		t.Errorf("a reason code is spelled inside program text: %s", offender)
	}

	modelFacing := []string{"internal/agent/", "internal/skill/", "internal/tool/", "internal/context/", "internal/analysis/", "internal/llm/", "internal/mcp/"}
	scanned := 0
	for name, file := range files {
		facing := false
		for _, prefix := range modelFacing {
			facing = facing || strings.HasPrefix(name, prefix)
		}
		if !facing {
			continue
		}
		scanned++
		for _, spec := range file.Imports {
			if strings.HasSuffix(strings.Trim(spec.Path.Value, `"`), "/internal/outputdump") {
				t.Errorf("%s imports the side-file package: a model-facing package must not be able to name a reason code", name)
			}
		}
	}
	if scanned < 200 {
		t.Fatalf("vacuous scan: %d model-facing files", scanned)
	}
}

// Self-red: a code embedded in a longer literal is found; the constant file
// and an unrelated literal are not.
func TestReasonCodeTextCensusFlagsAnEmbeddedCode(t *testing.T) {
	fset := token.NewFileSet()
	parse := func(name, src string) *ast.File {
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	codes := AllRootCauseUnavailableReasonCodes()
	files := map[string]*ast.File{
		"internal/outputdump/root_cause_reason_code.go": parse("root_cause_reason_code.go", "package outputdump\nconst c = \""+RootCauseReasonRequestExceedsCapacity+"\""),
		"internal/skill/teaching.go":                    parse("teaching.go", "package skill\nconst hint = \"when the side file says "+RootCauseReasonRequestExceedsCapacity+", shorten the answer\""),
		"internal/skill/other.go":                       parse("other.go", "package skill\nconst other = \"the answer request was not sent\""),
	}
	offenders, literals := literalsSpellingAReasonCode(files, codes)
	if literals != 2 || len(offenders) != 1 || !strings.Contains(offenders[0], "internal/skill/teaching.go") {
		t.Fatalf("exactly the embedded code must be flagged: %v (literals %d)", offenders, literals)
	}
}
