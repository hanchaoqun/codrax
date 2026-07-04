package dataquery

// DQA O1 bounded-read pin (2026-07-05).
//
// The data lane reads user-named local material files (CSV/JSON/JSONL/text,
// images). An unbounded whole-file slurp of such a path is the same OOM
// class that killed a customer run when the citation-quote normalizer
// slurped a 1104 MiB trace (Session 36 / width.SourceReadMaxBytes history).
// Every whole-file read in this package therefore goes through
// width.ReadFileBounded with the data-lane bound
// (EffectiveMaxFileBytes: runner override → data_task_max_file_bytes →
// 32 MiB default) and refuses oversized files fail-loud. Silent truncation
// is forbidden — a truncated CSV computes wrong sums.
//
// Scan surface (DQA F2): the pin scans BOTH this package's production files
// AND the repl-side data-lane consumers listed in dataLaneExternalPinFiles
// ("../repl/..." relative paths, same precedent as the NKR display pin in
// internal/tool/trace_note_keys_display_pin_test.go). The repl files carry a
// per-file bounded-read witness count: if a listed file stops containing any
// bounded read (rename/refactor moved the read elsewhere), the pin fails
// loudly instead of silently guarding nothing.
//
// The scanner parses each file's ImportSpecs and rejects:
//   - <os-or-ioutil local name>.ReadFile calls (unbounded slurp), where the
//     local name set is collected from the import table — plain imports AND
//     renamed imports (import osx "os") are both matched (DQA F5); a
//     dot-import of os / io / io/ioutil is rejected outright because it
//     defeats selector-based matching entirely, and
//   - <io local name>.ReadAll calls whose argument is not an
//     io.LimitReader(...) call expression (io.ReadAll(io.LimitReader(f, n+1))
//     is bounded by construction),
//
// unless the site is whitelisted below with an audit rationale.
//
// Known residual escapes (DQA F5, recorded honestly):
//   - import-alias renaming of os/io/ioutil: CLOSED — local names are
//     resolved from the ImportSpec table, and dot-imports are rejected.
//   - indirect reader form: OPEN — io.ReadAll(r) where r is a variable
//     previously assigned io.LimitReader(...) (or any other reader) is not
//     resolved; distinguishing it from an unbounded reader needs type/
//     dataflow analysis whose cost is out of proportion for this pin. Such
//     a call is flagged as a violation (the argument is not a literal
//     LimitReader call), so the pressure is toward the inline bounded form,
//     not toward silence.
//
// Deliberately NOT pinned: os.Open feeding csv.Reader / bufio.Scanner
// streaming consumers. Those hold one record/line at a time (csv.Reader
// memory is bounded by the largest single record, bufio.Scanner by its
// 1 MiB token cap), which the 2026-07-05 audit classified as
// streaming-bounded. A new slurp API would have to be added here.

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/width"
)

// bareReadWhitelist lists AUDITED production call sites that may keep a
// bare slurp call. Key is "<file>:<enclosing func>", value is the audit
// rationale (input provably not a user-controlled material path, or the
// read bounded by other means). Adding an entry requires re-running the
// DQA reachability audit for that site.
var bareReadWhitelist = map[string]string{
	// (empty — every audited slurp site was converted to
	// width.ReadFileBounded or an io.LimitReader stream in the DQA batch.)
}

// dataLaneExternalPinFiles lists data-lane consumer files OUTSIDE this
// package that read user-reachable material/checkpoint/payload paths and
// must stay on bounded reads (DQA F2: the two repl files silently fell out
// of the package-directory scan; DQA F6 added the operation-payload excerpt
// reader). Paths are relative to this package directory. Every listed file
// must parse AND contain at least one bounded-read call (see
// boundedReadWitness) or the pin fails as stale.
var dataLaneExternalPinFiles = []string{
	"../repl/data_material_extractor.go",
	"../repl/data_task_cli.go",
	"../repl/command_operation_planner.go",
}

type bareReadScanResult struct {
	violations       []string
	boundedWitnesses int
}

func TestDataLaneProductionFilesHaveNoUnboundedSlurpReads(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	var violations []string
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		result, err := scanFileForBareReads(fset, name)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		violations = append(violations, result.violations...)
	}
	if scanned == 0 {
		t.Fatal("scanned no production files; pin is vacuous")
	}
	// External data-lane consumers (repl side), with a per-file matched
	// counter so a rename/refactor cannot silently drop a file out of the
	// guarded surface.
	for _, name := range dataLaneExternalPinFiles {
		result, err := scanFileForBareReads(fset, name)
		if err != nil {
			t.Fatalf("parse external pin file %s: %v (renamed/moved? update dataLaneExternalPinFiles alongside the refactor)", name, err)
		}
		if result.boundedWitnesses == 0 {
			t.Fatalf("%s: no bounded-read call site matched (width.ReadFileBounded or io.ReadAll(io.LimitReader(...))) — the pin list is stale and this file is no longer guarded; update dataLaneExternalPinFiles alongside the refactor", name)
		}
		violations = append(violations, result.violations...)
	}
	for key := range bareReadWhitelist {
		if strings.TrimSpace(bareReadWhitelist[key]) == "" {
			violations = append(violations, fmt.Sprintf("whitelist entry %q has an empty rationale", key))
		}
	}
	if len(violations) > 0 {
		t.Fatalf("unbounded slurp reads in data-lane production code:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// scanFileForBareReads parses one production file and returns bare-slurp
// violations plus the count of bounded-read witness sites. Matching is
// import-table-aware (DQA F5): local names for the os / io / io/ioutil
// packages are collected from the file's ImportSpecs so renamed imports
// cannot dodge the pin; dot-imports of those packages are violations.
func scanFileForBareReads(fset *token.FileSet, filename string) (bareReadScanResult, error) {
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return bareReadScanResult{}, err
	}
	base := filepath.ToSlash(filename)
	slurpNames := map[string]bool{} // local names of os / io/ioutil → .ReadFile is a slurp
	ioNames := map[string]bool{}    // local names of io → .ReadAll needs a LimitReader arg
	var result bareReadScanResult
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		local := ""
		if imp.Name != nil {
			local = imp.Name.Name
		}
		switch importPath {
		case "os", "io/ioutil", "io":
			if local == "_" {
				continue
			}
			if local == "." {
				pos := fset.Position(imp.Pos())
				result.violations = append(result.violations, fmt.Sprintf(
					"%s:%d: dot-import of %q defeats the selector-based bounded-read pin — import it with a package name",
					base, pos.Line, importPath))
				continue
			}
			if local == "" {
				// Default local name: last path element.
				local = importPath[strings.LastIndex(importPath, "/")+1:]
			}
			if importPath == "io" {
				ioNames[local] = true
			} else {
				slurpNames[local] = true
			}
		}
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkg, sel, ok := selectorCallTarget(call)
			if !ok {
				return true
			}
			if sel == "ReadFileBounded" {
				result.boundedWitnesses++
				return true
			}
			bare := false
			switch {
			case slurpNames[pkg] && sel == "ReadFile":
				bare = true
			case ioNames[pkg] && sel == "ReadAll":
				if readAllArgIsLimitReader(call, ioNames) {
					result.boundedWitnesses++
				} else {
					bare = true
				}
			}
			if !bare {
				return true
			}
			key := base + ":" + fn.Name.Name
			if _, allowed := bareReadWhitelist[key]; allowed {
				return true
			}
			pos := fset.Position(call.Pos())
			result.violations = append(result.violations, fmt.Sprintf(
				"%s:%d (%s): bare %s.%s — data-lane material reads must go through width.ReadFileBounded with EffectiveMaxFileBytes (fail-loud oversize refusal, no silent truncation) or an io.LimitReader stream where truncation is a declared part of the contract; if this site is provably not user-reachable, whitelist %q with an audit rationale",
				base, pos.Line, fn.Name.Name, pkg, sel, key))
			return true
		})
	}
	return result, nil
}

func selectorCallTarget(call *ast.CallExpr) (pkg, sel string, ok bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return ident.Name, selector.Sel.Name, true
}

func readAllArgIsLimitReader(call *ast.CallExpr, ioNames map[string]bool) bool {
	if len(call.Args) != 1 {
		return false
	}
	inner, ok := call.Args[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	pkg, sel, ok := selectorCallTarget(inner)
	return ok && ioNames[pkg] && sel == "LimitReader"
}

// Oversize behavior witness: a material larger than the data-lane bound is
// refused with the typed width.ErrSourceReadOversized carrying the observed
// size and the cap — the pre-fix behavior was an unbounded slurp.
func TestActionRecordReadersRefuseOversizeMaterialTyped(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"m.json":  `[{"a":"1"},{"a":"2"},{"a":"` + strings.Repeat("x", 400) + `"}]`,
		"m.jsonl": `{"a":"1"}` + "\n" + `{"a":"` + strings.Repeat("y", 400) + `"}`,
		"m.txt":   "line one\n" + strings.Repeat("z", 400) + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runner := ActionRunner{RepoRoot: dir, MaxFileBytes: 256}
	for name, body := range files {
		_, _, _, _, err := runner.readActionRecords(name, 10)
		if err == nil {
			t.Fatalf("%s: expected oversize refusal at cap=256, got nil error", name)
		}
		var oversize *width.ErrSourceReadOversized
		if !errors.As(err, &oversize) {
			t.Fatalf("%s: expected typed *width.ErrSourceReadOversized, got %T: %v", name, err, err)
		}
		if oversize.Cap != 256 || oversize.Size != int64(len(body)) {
			t.Fatalf("%s: oversize error must carry actual size and cap: got size=%d cap=%d want size=%d cap=256",
				name, oversize.Size, oversize.Cap, len(body))
		}
		for _, needle := range []string{fmt.Sprintf("%d", oversize.Size), "256"} {
			if !strings.Contains(err.Error(), needle) {
				t.Fatalf("%s: fail-loud message must contain %q, got %q", name, needle, err.Error())
			}
		}
	}
	// extract_records lane refuses the same way.
	if _, err := runner.extractRecordsFromPath("m.json", 10); err == nil {
		t.Fatal("extractRecordsFromPath: expected oversize refusal at cap=256, got nil error")
	} else {
		var oversize *width.ErrSourceReadOversized
		if !errors.As(err, &oversize) {
			t.Fatalf("extractRecordsFromPath: expected typed oversize error, got %T: %v", err, err)
		}
	}
}

// Small-file behavior witness: materials within the bound parse exactly as
// before the fix (width.ReadFileBounded returns the identical bytes).
func TestActionRecordReadersUnchangedWithinBound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.json"), []byte(`[{"a":"1"},{"a":"2"}]`), 0600); err != nil {
		t.Fatalf("write m.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "m.jsonl"), []byte("{\"b\":\"3\"}\n{\"b\":\"4\"}\n"), 0600); err != nil {
		t.Fatalf("write m.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "m.txt"), []byte("alpha\nbeta\n"), 0600); err != nil {
		t.Fatalf("write m.txt: %v", err)
	}
	runner := ActionRunner{RepoRoot: dir}
	records, _, total, _, err := runner.readActionRecords("m.json", 10)
	if err != nil || total != 2 || len(records) != 2 || records[0].Fields["a"] != "1" || records[1].Fields["a"] != "2" {
		t.Fatalf("json records changed under bound: records=%v total=%d err=%v", records, total, err)
	}
	records, _, total, _, err = runner.readActionRecords("m.jsonl", 10)
	if err != nil || total != 2 || len(records) != 2 || records[0].Fields["b"] != "3" || records[1].Fields["b"] != "4" {
		t.Fatalf("jsonl records changed under bound: records=%v total=%d err=%v", records, total, err)
	}
	records, _, total, _, err = runner.readActionRecords("m.txt", 10)
	if err != nil || total != 2 || len(records) != 2 || records[0].Fields["text"] != "alpha" || records[1].Fields["text"] != "beta" {
		t.Fatalf("text records changed under bound: records=%v total=%d err=%v", records, total, err)
	}
}

// Knob resolution witness: explicit runner override → configured yaml knob →
// code default; candidate inspection honors the configured knob and refuses
// oversize via InspectError instead of slurping.
func TestEffectiveMaxFileBytesResolutionAndInspectRefusal(t *testing.T) {
	SetDefaultMaxFileBytes(256)
	defer SetDefaultMaxFileBytes(0)
	if got := EffectiveMaxFileBytes(0); got != 256 {
		t.Fatalf("configured knob not honored: got %d want 256", got)
	}
	if got := EffectiveMaxFileBytes(100); got != 100 {
		t.Fatalf("explicit override must win over knob: got %d want 100", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "big.json")
	if err := os.WriteFile(path, []byte(`{"k":"`+strings.Repeat("v", 400)+`"}`), 0600); err != nil {
		t.Fatalf("write big.json: %v", err)
	}
	f := inspectJSONCandidate(path, CandidateFile{Path: "big.json", Kind: "json"})
	if f.InspectError == "" || !strings.Contains(f.InspectError, "256") {
		t.Fatalf("inspect must refuse oversize fail-loud with the cap in the message, got %q", f.InspectError)
	}
	SetDefaultMaxFileBytes(0)
	if got := EffectiveMaxFileBytes(0); got != defaultMaxFileBytes {
		t.Fatalf("reset must restore code default: got %d want %d", got, defaultMaxFileBytes)
	}
}

// Classification witness (DQA F1): the oversize refusal is a TYPED leaf in
// classifyExecutionFailureLeaf — code source_oversized, source
// typed_contract — and its repair hint names the real levers (observed
// size, active bound, data_task_max_file_bytes knob, smaller file). The
// pre-fix behavior fell through to the error-text branch as runtime_failure
// with the generic "emit a corrected bounded plan" hint, which cannot fix
// an oversized input file.
func TestClassifyExecutionFailureTypesOversizeRefusalLeaf(t *testing.T) {
	oversize := &width.ErrSourceReadOversized{Path: "/data/huge.csv", Size: 987654, Cap: 4096}
	err := DataActionError{
		ActionID:   "a3",
		ActionKind: DataActionExtractRecords,
		Err:        fmt.Errorf("read material: %w", oversize),
	}
	v := ClassifyExecutionFailure(err)
	if v.Code != "source_oversized" {
		t.Fatalf("oversize refusal must classify as source_oversized, got code=%q (pre-fix bug: fell through to %q)", v.Code, "runtime_failure")
	}
	if v.Source != DataViolationSourceTypedContract {
		t.Fatalf("oversize refusal must be a typed-source violation, got source=%q", v.Source)
	}
	if v.ActionID != "a3" || v.ActionKind != string(DataActionExtractRecords) {
		t.Fatalf("action identity must survive classification: got action_id=%q action_kind=%q", v.ActionID, v.ActionKind)
	}
	for _, needle := range []string{"987654", "4096", "data_task_max_file_bytes", "smaller"} {
		if !strings.Contains(v.RepairHint, needle) {
			t.Fatalf("repair hint must state size/cap/knob/smaller-file levers; missing %q in %q", needle, v.RepairHint)
		}
	}
	if strings.Contains(v.RepairHint, "emit a corrected bounded plan") {
		t.Fatalf("repair hint must not suggest re-emitting a plan — a plan cannot shrink the file: %q", v.RepairHint)
	}
	if v.Repairability != RepairabilityNeedsClarification {
		t.Fatalf("oversize refusal is not plan-repairable; want needs_clarification, got %q", v.Repairability)
	}
	if strings.TrimSpace(v.Summary) == "" || !strings.Contains(v.Summary, "huge.csv") {
		t.Fatalf("summary must carry the refusal text, got %q", v.Summary)
	}
}

// Internal-artifact context witness (DQA F4): when the oversized path is a
// runner-written temp artifact resolved through an alias, the refusal
// message says so instead of pointing the user at a temp file they never
// named — and the typed oversize error stays visible through the wrap.
func TestReadActionRecordsAnnotatesInternalArtifactOversize(t *testing.T) {
	dir := t.TempDir()
	artifactAbs := filepath.Join(dir, "artifacts", "clean.json")
	if err := os.MkdirAll(filepath.Dir(artifactAbs), 0700); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := os.WriteFile(artifactAbs, []byte(`[{"a":"`+strings.Repeat("x", 400)+`"}]`), 0600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	runner := ActionRunner{
		RepoRoot:      filepath.Join(dir, "workspace"),
		MaxFileBytes:  256,
		artifactFiles: map[string]string{"clean#records": artifactAbs},
	}
	_, _, _, _, err := runner.readActionRecords("clean#records", 10)
	if err == nil {
		t.Fatal("expected oversize refusal for internal artifact, got nil error")
	}
	var oversize *width.ErrSourceReadOversized
	if !errors.As(err, &oversize) {
		t.Fatalf("annotation must preserve the typed oversize error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "internal artifact") || !strings.Contains(err.Error(), "clean#records") {
		t.Fatalf("internal-artifact oversize message must carry the alias and the internal-artifact context, got %q", err.Error())
	}
	// A plain user material (not alias-resolved) must NOT get the internal
	// artifact framing.
	if err := os.MkdirAll(runner.RepoRoot, 0700); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	userAbs := filepath.Join(runner.RepoRoot, "user.json")
	if err := os.WriteFile(userAbs, []byte(`[{"a":"`+strings.Repeat("y", 400)+`"}]`), 0600); err != nil {
		t.Fatalf("write user material: %v", err)
	}
	_, _, _, _, err = runner.readActionRecords("user.json", 10)
	if err == nil || strings.Contains(err.Error(), "internal artifact") {
		t.Fatalf("user-material oversize refusal must not claim internal-artifact context, got %v", err)
	}
}
