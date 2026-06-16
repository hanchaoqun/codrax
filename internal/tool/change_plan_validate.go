package tool

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// change_plan_validate.go bundles the change-plan validators and
// constructors so BOTH the single-shot emit_change_plan path AND the
// structural emit_plan_skeleton + emit_plan_change path can run them
// against the same canonical types.FileChange shape. This file is the
// load-bearing seam that prevents the two emit paths from drifting:
// every validator below is the SOLE producer of its rejection text;
// every caller routes through these helpers, never re-implements the
// rule in-line. If a validation contract changes, it changes here,
// and both emit paths inherit the new behaviour for free.

// emitChangesToFileChanges normalizes the wire-shape coming out of
// emit_change_plan's JSON decoder into the canonical types.FileChange
// shape used by every validator below. Trims whitespace on every
// string field; copies DependsOn into a fresh slice (so later
// in-place edits cannot leak into the JSON-decoded struct).
func emitChangesToFileChanges(changes []emitChangePlanChange) []types.FileChange {
	out := make([]types.FileChange, 0, len(changes))
	for _, c := range changes {
		var deps []string
		if len(c.DependsOn) > 0 {
			deps = make([]string, 0, len(c.DependsOn))
			for _, d := range c.DependsOn {
				deps = append(deps, strings.TrimSpace(d))
			}
		}
		out = append(out, types.FileChange{
			Path:       strings.TrimSpace(c.Path),
			Kind:       strings.TrimSpace(c.Kind),
			NewContent: c.NewContent,
			Patch:      c.Patch,
			Edits:      append([]types.StructuredEdit(nil), c.Edits...),
			NewPath:    strings.TrimSpace(c.NewPath),
			Rationale:  strings.TrimSpace(c.Rationale),
			DependsOn:  deps,
		})
	}
	return out
}

// validatePlanGraphIntegrity bundles the four content-independent
// structural checks: every change has a path; every kind is legal;
// no duplicate paths; no dangling / self / empty depends_on. Cycle
// detection is its own pass (detectDepsCycle) because the caller
// wants the cycle path string in the error message.
//
// Returns "" on success or a short rejection reason. Callers prefix
// the tool name + "rejected: " before surfacing.
//
// Content-independent: this can run on the skeleton path BEFORE any
// new_content arrives, so emit_plan_skeleton catches structural
// problems early without waiting for per-file emits.
func validatePlanGraphIntegrity(changes []types.FileChange) string {
	seenPaths := make(map[string]int, len(changes))
	for i, c := range changes {
		path := strings.TrimSpace(c.Path)
		if path == "" {
			return "one of the changes has an empty path"
		}
		if !isLegalChangeKind(strings.TrimSpace(c.Kind)) {
			return fmt.Sprintf("change %q has illegal kind %q (must be create|modify|delete|patch|rename)", path, c.Kind)
		}
		if _, dup := seenPaths[path]; dup {
			return fmt.Sprintf("duplicate change for path %q (one-change-per-file constraint; combine into a single FileChange)", path)
		}
		seenPaths[path] = i
	}
	// Per-kind shape checks. rename requires NewPath; non-rename
	// kinds must NOT carry NewPath (catches LLM emits that confuse
	// rename with modify+ move).
	for _, c := range changes {
		path := strings.TrimSpace(c.Path)
		kind := strings.TrimSpace(c.Kind)
		newPath := strings.TrimSpace(c.NewPath)
		switch kind {
		case "rename":
			if newPath == "" {
				return fmt.Sprintf("change %q has kind=rename but new_path is empty", path)
			}
			if newPath == path {
				return fmt.Sprintf("change %q has kind=rename with new_path equal to path; remove the rename or pick a different destination", path)
			}
			if _, collision := seenPaths[newPath]; collision {
				return fmt.Sprintf("change %q rename destination %q collides with another change in this plan (one path per plan, even across rename)", path, newPath)
			}
		default:
			if newPath != "" {
				return fmt.Sprintf("change %q has kind=%s but new_path is set (only kind=rename uses new_path)", path, kind)
			}
		}
		if len(c.Edits) > 0 && kind != "patch" {
			return fmt.Sprintf("change %q has edits[] but kind=%s (structured edits are only valid for kind=patch)", path, kind)
		}
		if len(c.Edits) > 0 && strings.TrimSpace(c.Patch) != "" {
			return fmt.Sprintf("change %q has both patch and edits[]; choose exactly one patch source", path)
		}
	}
	for _, c := range changes {
		path := strings.TrimSpace(c.Path)
		for _, dep := range c.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				return fmt.Sprintf("change %q has an empty depends_on entry", path)
			}
			if dep == path {
				return fmt.Sprintf("change %q depends_on itself", path)
			}
			if _, ok := seenPaths[dep]; !ok {
				return fmt.Sprintf("change %q depends_on %q but %q is not in changes[]", path, dep, dep)
			}
		}
	}
	return ""
}

// validatePlanFullContent runs every validator that requires the
// new_content / patch payload to be filled in:
//   - patch pre-flight (`git apply --check`)
//   - V1 deps closure (Go imports vs go.mod)
//   - V3 wiring closure (subsystem fan-in fileset)
//   - V4 summary fidelity (paths + imports referenced in summary)
//   - V2 dry-build (`go vet` on overlay)
//
// Returns "" on success or the first failing validator's reason.
//
// Content-dependent: must NOT run from emit_plan_skeleton (NewContent
// is empty there). emit_plan_change calls this exactly once — when
// the last placeholder slot is filled — and only then promotes the
// PartialChangePlan to ChangePlan.
// validatePlanScopeKindAlignment (Method M, 2026-05-07
// patch-vs-modify forensic) enforces the typed-signal hard gate
// between WriteAnalysisIR.Request.Task.Scope and FileChange.Kind:
//
//	task.scope == "micro"  →  every existing-file change MUST be
//	                          kind=patch (kind=modify rejected)
//	other scopes           →  unconstrained (kind=patch / modify
//	                          decision left to the planner)
//
// "micro" is defined in WriteScope as "change is contained to one
// function or one constant in one file" — exactly the shape where
// kind=patch's surgical-diff guarantee is most valuable. Allowing
// kind=modify on micro-scope tasks routinely produced full-file
// overwrites for one-line typo fixes (eval forensic 2026-05-07
// patch_go_typo r1) — observably worse for the user (line-level
// review collapsed to whole-file diff) and structurally redundant
// (the same content can be expressed as a 1-hunk patch).
//
// Carve-outs (kind != "modify" → no enforcement):
//
//   - kind=create: new file; cannot patch what doesn't exist.
//   - kind=delete: file removal; no content edit needed.
//   - kind=rename: pure rename; the diff happens at git level,
//     not via patch. A rename + content edit needs TWO entries
//     (rename for the move, modify/patch for the content change),
//     so the modify/patch entry is independently scope-checked.
//   - kind=patch: ✓ what the gate enforces.
//
// Returns "" on success or a rejection reason naming the offending
// path and the structured patch alternatives so the LLM can fix without
// guessing whole-file content.
//
// Skip when ctx == nil OR ctx.Mutable == nil OR no WriteAnalysisIR
// has been emitted yet (defensive: the gate is content-dependent
// and pre-IR emits should not crash on a nil dereference). Skip
// when task.scope is empty / unknown (gate fires only on an
// authoritative LLM scope decision, never on absence).
func validatePlanScopeKindAlignment(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return ""
	}
	scope := ir.Request.Task.Scope
	if scope != types.ScopeMicro {
		return ""
	}
	for i, c := range changes {
		if strings.TrimSpace(c.Kind) != "modify" {
			continue
		}
		path := strings.TrimSpace(c.Path)
		if path == "" {
			path = fmt.Sprintf("changes[%d]", i)
		}
		return fmt.Sprintf(
			"task.scope=micro forbids kind=modify on %q — micro-scope tasks (one function / one constant / one line in one file, per the prior classification) MUST use kind=patch with a unified diff. "+
				"A whole-file overwrite for a one-line edit collapses the diff the user reviews. "+
				"Re-emit this change with kind=patch and prefer edits[] for localized line changes; use raw patch only for complex diffs. "+
				"Carve-outs: kind=create (new file) / kind=delete (removal) / kind=rename (pure move) bypass this gate; only kind=modify-on-existing-file is rejected.",
			path)
	}
	return ""
}

func validatePlanFullContent(ctx *types.BusContext, summary string, changes []types.FileChange, verificationProbes []types.VerificationProbe) string {
	if rej := validatePlanContentCarriers(changes); rej != "" {
		return rej
	}
	if rej := validateFullModifyCompleteness(ctx, changes); rej != "" {
		return rej
	}
	if rej := compileStructuredEditPatches(ctx, changes); rej != "" {
		return rej
	}
	if rej := validatePythonDuplicateDefinitionStutter(ctx, changes); rej != "" {
		return rej
	}
	if rej := validatePlanPatchDuplicateInsertions(changes); rej != "" {
		return rej
	}
	if GitAvailable() && ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		for _, c := range changes {
			if strings.TrimSpace(c.Kind) != "patch" {
				continue
			}
			if strings.TrimSpace(c.Patch) == "" {
				return fmt.Sprintf("change %q has kind=patch but Patch is empty (unified-diff required)", strings.TrimSpace(c.Path))
			}
			if err := CheckUnifiedDiff(ctx.RepoRoot, c.Patch); err != nil {
				return composePatchRejectionReason(ctx.RepoRoot, strings.TrimSpace(c.Path), err.Error(), c.Patch)
			}
		}
	}
	if rej := validatePlanDepsClosure(ctx.RepoRoot, changes); rej != "" {
		return rej
	}
	if rej := validatePlanWiringClosure(changes); rej != "" {
		return rej
	}
	if rej := validatePlanSummaryConsistency(summary, changes); rej != "" {
		return rej
	}
	if rej := validatePlanDryBuild(ctx, changes); rej != "" {
		return rej
	}
	if rej := validatePlanLint(ctx, changes); rej != "" {
		return rej
	}
	if rej := validatePlanProjectLint(ctx, changes); rej != "" {
		return rej
	}
	if rej := validateVerificationProbeContractRefs(ctx, verificationProbes); rej != "" {
		return rej
	}
	if rej := validatePythonVerificationProbeCoupling(ctx.RepoRoot, changes, verificationProbes); rej != "" {
		return rej
	}
	return ""
}

func validatePlanContentCarriers(changes []types.FileChange) string {
	for _, change := range changes {
		path := strings.TrimSpace(change.Path)
		kind := strings.TrimSpace(change.Kind)
		hasNewContent := strings.TrimSpace(change.NewContent) != ""
		hasPatch := strings.TrimSpace(change.Patch) != ""
		hasEdits := len(change.Edits) > 0
		switch kind {
		case "create", "modify":
			if hasPatch || hasEdits {
				return fmt.Sprintf("change %q has kind=%s but also carries patch/edits content; create/modify require new_content as the only content carrier", path, kind)
			}
			if !hasNewContent {
				return fmt.Sprintf("change %q has kind=%s but new_content is empty; provide the full file body or choose kind=delete/patch as appropriate", path, kind)
			}
		case "patch":
			if hasNewContent {
				return fmt.Sprintf("change %q has kind=patch but also carries new_content; patch changes must use only patch or edits[] so full-file overwrite bytes cannot leak into a surgical plan", path)
			}
			if !hasPatch && !hasEdits {
				return fmt.Sprintf("change %q has kind=patch but Patch is empty (unified-diff required)", path)
			}
		case "delete":
			if hasNewContent || hasPatch || hasEdits {
				return fmt.Sprintf("change %q has kind=delete but carries content fields; delete changes must not include new_content, patch, or edits[]", path)
			}
		case "rename":
			if hasNewContent || hasPatch || hasEdits {
				return fmt.Sprintf("change %q has kind=rename but carries content fields; use a separate modify/patch change for content edits", path)
			}
		}
	}
	return ""
}

const (
	fullModifyLargeFileLineThreshold     = 400
	fullModifyLargeFileByteThreshold     = 20 * 1024
	fullModifyPrefixTruncationPct        = 80
	fullModifyDrasticLineShrinkPct       = 50
	fullModifyDrasticByteShrinkPct       = 50
	fullModifyMinDeletedLinesForHardGate = 300
	fullModifyMinDeletedBytesForHardGate = 20 * 1024
)

func validateFullModifyCompleteness(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return ""
	}
	repoRootAbs, err := filepath.Abs(ctx.RepoRoot)
	if err != nil {
		return ""
	}
	for _, change := range changes {
		if strings.TrimSpace(change.Kind) != "modify" {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" {
			continue
		}
		abs := filepath.Clean(filepath.Join(repoRootAbs, filepath.FromSlash(path)))
		absCanonical, err := filepath.Abs(abs)
		if err != nil {
			continue
		}
		if absCanonical != repoRootAbs && !strings.HasPrefix(absCanonical, repoRootAbs+string(filepath.Separator)) {
			continue
		}
		oldBytes, err := os.ReadFile(absCanonical)
		if err != nil || len(oldBytes) == 0 {
			continue
		}
		newBytes := []byte(change.NewContent)
		if bytes.Equal(oldBytes, newBytes) {
			continue
		}
		oldLines := countPlanContentLines(string(oldBytes))
		newLines := countPlanContentLines(change.NewContent)
		largeByLines := oldLines >= fullModifyLargeFileLineThreshold
		largeByBytes := len(oldBytes) >= fullModifyLargeFileByteThreshold
		if !largeByLines && !largeByBytes {
			continue
		}
		deletedLines := oldLines - newLines
		deletedBytes := len(oldBytes) - len(newBytes)
		if looksLikePrefixTruncation(oldBytes, newBytes, deletedLines, deletedBytes) {
			return fmt.Sprintf("change %q uses kind=modify on an existing large file but new_content is a strict prefix/truncated subset of the current file (%d→%d lines, %d→%d bytes). Use kind=patch for localized edits or provide the complete full file body for an intentional rewrite.",
				path, oldLines, newLines, len(oldBytes), len(newBytes))
		}
		if looksLikeCatastrophicFullModifyShrink(oldLines, newLines, len(oldBytes), len(newBytes), deletedLines, deletedBytes) {
			return fmt.Sprintf("change %q uses kind=modify on an existing large file and removes most of the file (%d→%d lines, %d→%d bytes). This is likely a partial generated file body. Use kind=patch for surgical edits, kind=delete for intentional removal, or provide a complete full-file rewrite with comparable scope.",
				path, oldLines, newLines, len(oldBytes), len(newBytes))
		}
	}
	return ""
}

func countPlanContentLines(content string) int {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		lines++
	}
	return lines
}

func looksLikePrefixTruncation(oldBytes, newBytes []byte, deletedLines, deletedBytes int) bool {
	if len(newBytes) == 0 || len(newBytes) >= len(oldBytes) {
		return false
	}
	if !bytes.HasPrefix(oldBytes, newBytes) {
		return false
	}
	if len(newBytes)*100 >= len(oldBytes)*fullModifyPrefixTruncationPct {
		return false
	}
	return deletedLines >= 100 || deletedBytes >= 4096
}

func looksLikeCatastrophicFullModifyShrink(oldLines, newLines, oldByteLen, newByteLen, deletedLines, deletedBytes int) bool {
	lineShrink := oldLines > 0 && newLines*100 < oldLines*fullModifyDrasticLineShrinkPct && deletedLines >= fullModifyMinDeletedLinesForHardGate
	byteShrink := oldByteLen > 0 && newByteLen*100 < oldByteLen*fullModifyDrasticByteShrinkPct && deletedBytes >= fullModifyMinDeletedBytesForHardGate
	return lineShrink || byteShrink
}

func validatePythonVerificationProbeCoupling(repoRoot string, changes []types.FileChange, probes []types.VerificationProbe) string {
	if len(probes) == 0 {
		return ""
	}
	targets := pythonProductionModuleCandidates(changes)
	if len(targets) == 0 {
		return ""
	}
	pythonProbeCount := 0
	importsSeen := map[string]struct{}{}
	for _, probe := range probes {
		if strings.ToLower(strings.TrimSpace(probe.Language)) != "python" {
			continue
		}
		pythonProbeCount++
		imports := pythonImportDeclarations(probe.Code)
		if len(imports) == 0 {
			continue
		}
		for imported := range imports {
			importsSeen[imported] = struct{}{}
		}
		if pythonImportsCoverAnyTarget(imports, targets) || pythonImportsCoverRepoLocalPublicPackage(repoRoot, changes, imports) {
			return ""
		}
	}
	if pythonProbeCount == 0 {
		return ""
	}
	if len(importsSeen) == 0 {
		return fmt.Sprintf("verification_probes do not include any Python import declarations for changed production modules %s; at least one Python probe must import the changed module under test instead of checking an isolated copy", formatStringSet(targets))
	}
	return fmt.Sprintf("verification_probes import %s but do not import any changed Python production module %s; at least one Python probe must exercise the changed code, not a copied implementation fragment", formatStringSet(importsSeen), formatStringSet(targets))
}

const pythonDuplicateDefinitionStutterMaxGap = 6

type pythonDefinitionStutter struct {
	Kind       string
	Name       string
	Scope      string
	FirstLine  int
	SecondLine int
}

type pythonDefinitionSeen struct {
	Kind       string
	Line       int
	Indent     int
	Decorators []string
}

type pythonScopeFrame struct {
	Indent  int
	Segment string
}

func validatePythonDuplicateDefinitionStutter(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil {
		return ""
	}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") {
			continue
		}
		content, ok, rej := plannedPythonContent(ctx.RepoRoot, change)
		if rej != "" {
			return rej
		}
		if !ok {
			continue
		}
		dups := findPythonDuplicateDefinitionStutters(content)
		if len(dups) == 0 {
			continue
		}
		originalCounts := map[string]int{}
		if original, ok := readOriginalPythonContent(ctx.RepoRoot, change); ok {
			for _, dup := range findPythonDuplicateDefinitionStutters(original) {
				originalCounts[pythonDefinitionStutterKey(dup)]++
			}
		}
		for _, dup := range dups {
			key := pythonDefinitionStutterKey(dup)
			if originalCounts[key] > 0 {
				originalCounts[key]--
				continue
			}
			scope := "module"
			if strings.TrimSpace(dup.Scope) != "" {
				scope = dup.Scope
			}
			return fmt.Sprintf("change %q would create duplicate Python %s %q in scope %s at lines %d and %d. This usually means a structured insert landed next to the existing definition; replace the existing definition/body instead of inserting a second one.",
				path, dup.Kind, dup.Name, scope, dup.FirstLine, dup.SecondLine)
		}
	}
	return ""
}

func plannedPythonContent(repoRoot string, change types.FileChange) (string, bool, string) {
	kind := strings.TrimSpace(change.Kind)
	switch kind {
	case "create", "modify":
		if strings.TrimSpace(change.NewContent) == "" {
			return "", false, ""
		}
		return change.NewContent, true, ""
	case "patch":
		if len(change.Edits) == 0 {
			return "", false, ""
		}
		_, newContent, err := compileStructuredEditsToContent(repoRoot, &change)
		if err != nil {
			return "", false, enrichStructuredEditReplanDiagnostic(nil, err.Error())
		}
		return newContent, true, ""
	default:
		return "", false, ""
	}
}

func findPythonDuplicateDefinitionStutter(content string) (pythonDefinitionStutter, bool) {
	dups := findPythonDuplicateDefinitionStutters(content)
	if len(dups) == 0 {
		return pythonDefinitionStutter{}, false
	}
	return dups[0], true
}

func findPythonDuplicateDefinitionStutters(content string) []pythonDefinitionStutter {
	seen := map[string]pythonDefinitionSeen{}
	var stack []pythonScopeFrame
	var dups []pythonDefinitionStutter
	inTriple := ""
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lineNo := i + 1
		if inTriple != "" {
			if pythonTripleQuoteCount(line, inTriple)%2 == 1 {
				inTriple = ""
			}
			continue
		}
		code := stripPythonLineComment(line)
		trimmed := strings.TrimSpace(code)
		if trimmed == "" {
			continue
		}
		if delim, ok := pythonOpeningTripleQuote(trimmed); ok {
			if pythonTripleQuoteCount(trimmed, delim)%2 == 1 {
				inTriple = delim
			}
			continue
		}
		kind, name, ok := pythonDefinitionHeader(trimmed)
		if !ok {
			continue
		}
		indent := pythonLeadingIndent(line)
		decorators := pythonDecoratorsForDefinition(lines, i, indent)
		for len(stack) > 0 && indent <= stack[len(stack)-1].Indent {
			stack = stack[:len(stack)-1]
		}
		scope := pythonScopePath(stack)
		key := scope + "\x00" + name
		if prev, exists := seen[key]; exists &&
			lineNo-prev.Line <= pythonDuplicateDefinitionStutterMaxGap &&
			!pythonDuplicateDefinitionHasInterveningControlFlow(lines, prev.Line, lineNo, indent) &&
			!pythonDuplicateDefinitionAccessorPair(name, prev.Decorators, decorators) &&
			!pythonDuplicateDefinitionOverloadPair(prev.Decorators, decorators) {
			dups = append(dups, pythonDefinitionStutter{
				Kind:       kind,
				Name:       name,
				Scope:      scope,
				FirstLine:  prev.Line,
				SecondLine: lineNo,
			})
		}
		seen[key] = pythonDefinitionSeen{Kind: kind, Line: lineNo, Indent: indent, Decorators: decorators}
		stack = append(stack, pythonScopeFrame{
			Indent:  indent,
			Segment: kind + " " + name,
		})
	}
	return dups
}

func pythonDefinitionStutterKey(dup pythonDefinitionStutter) string {
	return dup.Kind + "\x00" + dup.Name + "\x00" + dup.Scope
}

func readOriginalPythonContent(repoRoot string, change types.FileChange) (string, bool) {
	root := strings.TrimSpace(repoRoot)
	path := filepath.ToSlash(strings.TrimSpace(change.Path))
	if root == "" || path == "" {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absPath := filepath.Join(absRoot, filepath.FromSlash(path))
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func pythonDecoratorsForDefinition(lines []string, defIndex int, defIndent int) []string {
	if defIndex <= 0 || defIndex > len(lines) {
		return nil
	}
	var decorators []string
	for i := defIndex - 1; i >= 0; i-- {
		line := stripPythonLineComment(lines[i])
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if pythonLeadingIndent(line) != defIndent || !strings.HasPrefix(trimmed, "@") {
			break
		}
		decorators = append(decorators, trimmed)
	}
	for i, j := 0, len(decorators)-1; i < j; i, j = i+1, j-1 {
		decorators[i], decorators[j] = decorators[j], decorators[i]
	}
	return decorators
}

func pythonDuplicateDefinitionAccessorPair(name string, prevDecorators, currentDecorators []string) bool {
	prevKind := pythonPropertyAccessorKind(name, prevDecorators)
	currentKind := pythonPropertyAccessorKind(name, currentDecorators)
	if currentKind == "setter" || currentKind == "deleter" {
		return prevKind == "property" || prevKind == "setter" || prevKind == "deleter"
	}
	return false
}

func pythonPropertyAccessorKind(name string, decorators []string) string {
	if !isPythonIdentifier(name) {
		return ""
	}
	for _, raw := range decorators {
		dec := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		dec = strings.TrimSpace(strings.TrimSuffix(dec, "()"))
		switch dec {
		case "property":
			return "property"
		case name + ".setter":
			return "setter"
		case name + ".deleter":
			return "deleter"
		}
	}
	return ""
}

func pythonDuplicateDefinitionOverloadPair(prevDecorators, currentDecorators []string) bool {
	return pythonDecoratorsContainTypingOverload(prevDecorators) || pythonDecoratorsContainTypingOverload(currentDecorators)
}

func pythonDecoratorsContainTypingOverload(decorators []string) bool {
	for _, raw := range decorators {
		dec := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		dec = strings.TrimSpace(strings.TrimSuffix(dec, "()"))
		if dec == "overload" || dec == "typing.overload" {
			return true
		}
	}
	return false
}

func pythonDuplicateDefinitionHasInterveningControlFlow(lines []string, firstLine, secondLine, definitionIndent int) bool {
	if firstLine < 1 || secondLine <= firstLine || secondLine > len(lines) {
		return false
	}
	for i := firstLine; i < secondLine-1; i++ {
		line := stripPythonLineComment(lines[i])
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := pythonLeadingIndent(line)
		if indent > definitionIndent {
			continue
		}
		if pythonControlFlowHeader(trimmed) {
			return true
		}
	}
	return false
}

func pythonControlFlowHeader(trimmed string) bool {
	if !strings.HasSuffix(trimmed, ":") {
		return false
	}
	for _, prefix := range []string{
		"if ", "elif ", "else", "try", "except", "finally", "for ", "while ", "with ", "match ", "case ",
	} {
		if trimmed == strings.TrimSuffix(prefix, " ") || strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func pythonDefinitionHeader(trimmed string) (kind string, name string, ok bool) {
	switch {
	case strings.HasPrefix(trimmed, "async def "):
		name = pythonIdentifierAfterPrefix(trimmed, "async def ")
		kind = "function"
	case strings.HasPrefix(trimmed, "def "):
		name = pythonIdentifierAfterPrefix(trimmed, "def ")
		kind = "function"
	case strings.HasPrefix(trimmed, "class "):
		name = pythonIdentifierAfterPrefix(trimmed, "class ")
		kind = "class"
	default:
		return "", "", false
	}
	if !isPythonIdentifier(name) {
		return "", "", false
	}
	return kind, name, true
}

func pythonIdentifierAfterPrefix(line, prefix string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	for i, r := range rest {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return rest[:i]
		}
	}
	return rest
}

func pythonLeadingIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 8
		default:
			return indent
		}
	}
	return indent
}

func pythonScopePath(stack []pythonScopeFrame) string {
	if len(stack) == 0 {
		return ""
	}
	parts := make([]string, 0, len(stack))
	for _, frame := range stack {
		parts = append(parts, frame.Segment)
	}
	return strings.Join(parts, ".")
}

func pythonOpeningTripleQuote(trimmed string) (string, bool) {
	if strings.HasPrefix(trimmed, `"""`) {
		return `"""`, true
	}
	if strings.HasPrefix(trimmed, `'''`) {
		return `'''`, true
	}
	return "", false
}

func pythonTripleQuoteCount(line, delim string) int {
	if delim == "" {
		return 0
	}
	count := 0
	for {
		idx := strings.Index(line, delim)
		if idx < 0 {
			return count
		}
		count++
		line = line[idx+len(delim):]
	}
}

func pythonProductionModuleCandidates(changes []types.FileChange) map[string]struct{} {
	out := map[string]struct{}{}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") || types.LooksLikeTestFilePath(path) {
			continue
		}
		for _, mod := range pythonModuleCandidatesForPath(path) {
			out[mod] = struct{}{}
		}
	}
	return out
}

func pythonModuleCandidatesForPath(relPath string) []string {
	relPath = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(relPath)), "./")
	if !strings.HasSuffix(relPath, ".py") {
		return nil
	}
	stem := strings.TrimSuffix(relPath, ".py")
	if strings.HasSuffix(stem, "/__init__") {
		stem = strings.TrimSuffix(stem, "/__init__")
	}
	stem = strings.Trim(stem, "/")
	if stem == "" {
		return nil
	}
	raw := strings.ReplaceAll(stem, "/", ".")
	candidates := []string{raw}
	for _, root := range []string{"src.", "lib."} {
		if strings.HasPrefix(raw, root) && len(raw) > len(root) {
			candidates = append(candidates, strings.TrimPrefix(raw, root))
		}
	}
	return uniqueNonEmptyStrings(candidates)
}

func pythonImportDeclarations(code string) map[string]struct{} {
	out := map[string]struct{}{}
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		parsePythonImportDeclaration(line, out)
	}
	return out
}

func parsePythonImportDeclaration(line string, out map[string]struct{}) {
	line = strings.TrimSpace(stripPythonLineComment(line))
	if line == "" || strings.HasPrefix(line, ".") {
		return
	}
	if strings.HasPrefix(line, "import ") {
		body := strings.TrimSpace(strings.TrimPrefix(line, "import "))
		for _, part := range strings.Split(body, ",") {
			mod := firstPythonImportToken(part)
			if isPythonModuleName(mod) {
				out[mod] = struct{}{}
			}
		}
		return
	}
	if strings.HasPrefix(line, "from ") {
		body := strings.TrimSpace(strings.TrimPrefix(line, "from "))
		idx := strings.Index(body, " import ")
		if idx <= 0 {
			return
		}
		base := strings.TrimSpace(body[:idx])
		if !isPythonModuleName(base) {
			return
		}
		out[base] = struct{}{}
		imports := strings.TrimSpace(body[idx+len(" import "):])
		imports = strings.Trim(imports, "()")
		for _, part := range strings.Split(imports, ",") {
			name := firstPythonImportToken(part)
			if isPythonIdentifier(name) {
				out[base+"."+name] = struct{}{}
			}
		}
	}
}

func stripPythonLineComment(line string) string {
	var quote rune
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' {
			return line[:i]
		}
	}
	return line
}

func firstPythonImportToken(part string) string {
	fields := strings.Fields(strings.TrimSpace(part))
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(fields[0])
}

func isPythonModuleName(s string) bool {
	if s == "" || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if !isPythonIdentifier(part) {
			return false
		}
	}
	return true
}

func isPythonIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
				return false
			}
			continue
		}
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func pythonImportsCoverAnyTarget(imports, targets map[string]struct{}) bool {
	for target := range targets {
		for imported := range imports {
			if pythonImportCoversTarget(imported, target) {
				return true
			}
		}
	}
	return false
}

func pythonImportCoversTarget(imported, target string) bool {
	imported = strings.TrimSpace(imported)
	target = strings.TrimSpace(target)
	if imported == "" || target == "" {
		return false
	}
	if imported == target || strings.HasPrefix(target, imported+".") {
		return true
	}
	return pythonTopLevelModule(imported) == pythonTopLevelModule(target)
}

func pythonImportsCoverRepoLocalPublicPackage(repoRoot string, changes []types.FileChange, imports map[string]struct{}) bool {
	publicPackages := pythonRepoLocalPublicPackagesForChanges(repoRoot, changes)
	if len(publicPackages) == 0 {
		return false
	}
	for imported := range imports {
		top := pythonTopLevelModule(imported)
		if top == "" {
			continue
		}
		if _, ok := publicPackages[top]; ok {
			return true
		}
	}
	return false
}

func pythonRepoLocalPublicPackagesForChanges(repoRoot string, changes []types.FileChange) map[string]struct{} {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}
	out := map[string]struct{}{}
	seenRoots := map[string]struct{}{}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") || types.LooksLikeTestFilePath(path) {
			continue
		}
		sourceRootRel := pythonSourceRootForPath(path)
		if _, seen := seenRoots[sourceRootRel]; seen {
			continue
		}
		seenRoots[sourceRootRel] = struct{}{}
		sourceRoot := filepath.Join(repoRoot, filepath.FromSlash(sourceRootRel))
		entries, err := os.ReadDir(sourceRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			stem := strings.TrimSuffix(name, ".py")
			if !isPythonIdentifier(stem) || strings.HasPrefix(stem, "_") {
				continue
			}
			full := filepath.Join(sourceRoot, name)
			switch {
			case entry.IsDir():
				if pythonDirectoryLooksImportable(full) {
					out[name] = struct{}{}
				}
			case strings.HasSuffix(name, ".py"):
				out[stem] = struct{}{}
			}
		}
	}
	return out
}

func pythonSourceRootForPath(relPath string) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(relPath)), "./")
	for _, root := range []string{"src", "lib"} {
		if relPath == root || strings.HasPrefix(relPath, root+"/") {
			return root
		}
	}
	return "."
}

func pythonDirectoryLooksImportable(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "__init__.py")); err == nil {
		return true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if strings.HasPrefix(name, "_") {
				continue
			}
			if _, err := os.Stat(filepath.Join(path, name, "__init__.py")); err == nil {
				return true
			}
			continue
		}
		if strings.HasSuffix(name, ".py") && !strings.HasPrefix(name, "_") {
			return true
		}
	}
	return false
}

func pythonTopLevelModule(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, ".") {
		return ""
	}
	if idx := strings.Index(name, "."); idx >= 0 {
		name = name[:idx]
	}
	if !isPythonIdentifier(name) {
		return ""
	}
	return name
}

func formatStringSet(values map[string]struct{}) string {
	if len(values) == 0 {
		return "[]"
	}
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return "[" + strings.Join(items, ", ") + "]"
}

func uniqueNonEmptyStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validatePlanPatchDuplicateInsertions(changes []types.FileChange) string {
	for _, c := range changes {
		if strings.TrimSpace(c.Kind) != "patch" || strings.TrimSpace(c.Patch) == "" {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !duplicateInsertionPathEligible(path) {
			continue
		}
		if block, ok := firstAdjacentDuplicateAddedBlock(c.Patch); ok {
			return fmt.Sprintf(
				"change %q repeats the same added block twice in a row (%d lines, first line %q). "+
					"Adjacent exact duplicate insertion blocks are treated as a planner stutter; remove the duplicate block and re-emit the plan.",
				path, len(block), strings.TrimSpace(block[0]))
		}
	}
	return ""
}

func duplicateInsertionPathEligible(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".kt", ".kts",
		".rs", ".rb", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp",
		".cs", ".swift", ".php", ".scala", ".m", ".mm", ".sh", ".bash",
		".zsh", ".fish":
		return true
	default:
		return false
	}
}

func firstAdjacentDuplicateAddedBlock(patch string) ([]string, bool) {
	var run []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			run = append(run, line[1:])
			continue
		}
		if block, ok := duplicateBlockInAddedRun(run); ok {
			return block, true
		}
		run = run[:0]
	}
	return duplicateBlockInAddedRun(run)
}

func duplicateBlockInAddedRun(run []string) ([]string, bool) {
	if len(run) < 6 {
		return nil, false
	}
	for start := 0; start < len(run); start++ {
		max := (len(run) - start) / 2
		for size := max; size >= 3; size-- {
			left := run[start : start+size]
			right := run[start+size : start+2*size]
			if !sameDuplicateAddedBlockLines(left, right) || !duplicateAddedBlockIsSourceLike(left) {
				continue
			}
			return append([]string(nil), left...), true
		}
	}
	return nil, false
}

func sameDuplicateAddedBlockLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func duplicateAddedBlockIsSourceLike(lines []string) bool {
	nonEmpty := 0
	sourceLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmpty++
		if duplicateInsertedLineIsSourceLike(trimmed) {
			sourceLines++
		}
	}
	return nonEmpty >= 3 && sourceLines >= 2
}

func duplicateInsertedLineIsSourceLike(line string) bool {
	if line == "{" || line == "}" || line == "};" || line == ");" || line == ")," || line == "]" || line == "];" {
		return false
	}
	commentPrefixes := []string{"#", "//", "/*", "*", "--", "<!--"}
	for _, prefix := range commentPrefixes {
		if strings.HasPrefix(line, prefix) {
			return false
		}
	}
	return true
}

func compileStructuredEditPatches(ctx *types.BusContext, changes []types.FileChange) string {
	for i := range changes {
		if len(changes[i].Edits) == 0 {
			continue
		}
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			return fmt.Sprintf("change %q uses edits[] but no repo root is available to compile them", strings.TrimSpace(changes[i].Path))
		}
		patch, err := compileStructuredEditsToPatch(ctx.RepoRoot, &changes[i])
		if err != nil {
			return enrichStructuredEditReplanDiagnostic(ctx, err.Error())
		}
		changes[i].Patch = patch
	}
	return ""
}

func enrichStructuredEditReplanDiagnostic(ctx *types.BusContext, msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return msg
	}
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || ctx.PipelineStage != types.StagePlan {
		return msg
	}
	if ctx.Mutable.VerifyFailureHandoff() == nil {
		return msg
	}
	hasNoOp := strings.Contains(msg, " is a no-op")
	hasOldTextMismatch := strings.Contains(msg, "old_text mismatch")
	if !hasNoOp && !(hasOldTextMismatch && structuredEditReplanProbePassed(ctx)) {
		return msg
	}
	return msg + ". In a verify-failure replan, a no-op edit means the applied worktree may already contain the intended code. Run a typed planner probe with run_tests(dry_run=true) against the scoped failure; if it passes, emit changes: [] to record the no_change_required sentinel. If it fails, re-read the current bytes and emit a real non-no-op edit."
}

func structuredEditReplanProbePassed(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	probes := ctx.Mutable.PlanStageProbeReports()
	for i := len(probes) - 1; i >= 0; i-- {
		report := probes[i]
		if report == nil || report.Channel != types.ChangeReportChannelPlannerProbe {
			continue
		}
		if report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
			return false
		}
		passed, total := report.Score()
		return total > 0 && passed == total
	}
	return false
}

// composePatchRejectionReason mirrors composePatchRejection but
// returns the bare reason string (without the "emit_change_plan
// rejected:" prefix) so the caller can prepend whichever tool name
// applies. Originally composePatchRejection always emitted the
// emit_change_plan prefix; for the structural path we want
// emit_plan_change to surface the rejection under its own name.
func composePatchRejectionReason(repoRoot, path, gitErr, patchPayload string) string {
	full := composePatchRejection(repoRoot, path, gitErr, patchPayload)
	const prefix = "emit_change_plan rejected: "
	if strings.HasPrefix(full, prefix) {
		return full[len(prefix):]
	}
	return full
}

// newChangePlanFromChanges assembles the canonical ChangePlan struct
// from a request, summary, validated changes slice, and acceptance
// tests. ID format and timestamp logic match the legacy inline
// implementation in emit_change_plan.Execute exactly so disk-side
// plan files stay byte-shape compatible.
//
// Reused by emit_plan_change when promoting PartialChangePlan to
// ChangePlan — same factory, same shape.
func newChangePlanFromChanges(request, summary string, changes []types.FileChange, acceptanceTests []string, verificationProbes []types.VerificationProbe) *types.ChangePlan {
	now := time.Now()
	plan := &types.ChangePlan{
		ID:        fmt.Sprintf("plan-%d-%d", now.UnixNano(), os.Getpid()),
		Request:   request,
		Summary:   summary,
		Status:    types.PlanStatusPending,
		CreatedAt: now,
		Changes:   append([]types.FileChange(nil), changes...),
	}
	paths := make(map[string]struct{}, len(changes))
	for _, c := range changes {
		paths[c.Path] = struct{}{}
	}
	for p := range paths {
		plan.TargetPaths = append(plan.TargetPaths, p)
	}
	if len(acceptanceTests) > 0 {
		plan.AcceptanceTests = append([]string(nil), acceptanceTests...)
	}
	if len(verificationProbes) > 0 {
		plan.VerificationProbes = append([]types.VerificationProbe(nil), verificationProbes...)
	}
	return plan
}

func attachWriteBehaviorContracts(ctx *types.BusContext, plan *types.ChangePlan) {
	if ctx == nil || ctx.Mutable == nil || plan == nil {
		return
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil || len(ir.Request.BehaviorContracts) == 0 {
		return
	}
	plan.BehaviorContracts = append([]types.WriteBehaviorContract(nil), ir.Request.BehaviorContracts...)
}

func normalizeVerificationProbes(in []types.VerificationProbe) ([]types.VerificationProbe, string) {
	const (
		maxProbes            = 5
		maxProbeCodeBytes    = 8 * 1024
		maxExpectedFragments = 10
		defaultProbeTimeout  = 10
		maxProbeTimeout      = 30
	)
	if len(in) == 0 {
		return nil, ""
	}
	if len(in) > maxProbes {
		return nil, fmt.Sprintf("verification_probes has %d entries; maximum is %d", len(in), maxProbes)
	}
	out := make([]types.VerificationProbe, 0, len(in))
	seen := map[string]struct{}{}
	for i, probe := range in {
		language := strings.ToLower(strings.TrimSpace(probe.Language))
		if language != "python" {
			return nil, fmt.Sprintf("verification_probes[%d].language=%q is unsupported; supported values: python", i, probe.Language)
		}
		code := strings.TrimSpace(probe.Code)
		if code == "" {
			return nil, fmt.Sprintf("verification_probes[%d].code is required", i)
		}
		if len(code) > maxProbeCodeBytes {
			return nil, fmt.Sprintf("verification_probes[%d].code is %d bytes; maximum is %d", i, len(code), maxProbeCodeBytes)
		}
		id := strings.TrimSpace(probe.ID)
		if id == "" {
			id = fmt.Sprintf("probe-%d", i+1)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Sprintf("verification_probes has duplicate id %q", id)
		}
		seen[id] = struct{}{}
		workingDir := strings.TrimSpace(probe.WorkingDir)
		if workingDir == "" {
			workingDir = "."
		}
		workingDir = filepath.Clean(workingDir)
		if filepath.IsAbs(workingDir) {
			return nil, fmt.Sprintf("verification_probes[%d].working_dir=%q must be repo-relative", i, probe.WorkingDir)
		}
		for _, seg := range strings.Split(filepath.ToSlash(workingDir), "/") {
			if seg == ".." {
				return nil, fmt.Sprintf("verification_probes[%d].working_dir=%q escapes the repository", i, probe.WorkingDir)
			}
		}
		workingDir = filepath.ToSlash(workingDir)
		timeout := probe.TimeoutSeconds
		if timeout <= 0 {
			timeout = defaultProbeTimeout
		}
		if timeout > maxProbeTimeout {
			timeout = maxProbeTimeout
		}
		expected := make([]string, 0, len(probe.ExpectedStdout))
		for _, fragment := range probe.ExpectedStdout {
			fragment = strings.TrimSpace(fragment)
			if fragment != "" {
				expected = append(expected, fragment)
			}
			if len(expected) > maxExpectedFragments {
				return nil, fmt.Sprintf("verification_probes[%d].expected_stdout has more than %d non-empty entries", i, maxExpectedFragments)
			}
		}
		if len(expected) == 0 && !pythonVerificationProbeHasExecutableFailureSignal(code) {
			return nil, fmt.Sprintf("verification_probes[%d].code must include an executable failure signal (assert, raise, sys.exit/exit, pytest.fail, or expected_stdout); printing failure text without a non-zero exit path can falsely pass verification", i)
		}
		contractRefs, rej := normalizeProbeStringRefs(probe.ContractRefs, maxExpectedFragments, fmt.Sprintf("verification_probes[%d].contract_refs", i))
		if rej != "" {
			return nil, rej
		}
		changedSymbolRefs, rej := normalizeProbeStringRefs(probe.ChangedSymbolRefs, maxExpectedFragments, fmt.Sprintf("verification_probes[%d].changed_symbol_refs", i))
		if rej != "" {
			return nil, rej
		}
		out = append(out, types.VerificationProbe{
			ID:                     id,
			Language:               language,
			WorkingDir:             workingDir,
			Code:                   code,
			TimeoutSeconds:         timeout,
			ExpectedStdout:         expected,
			ContractRefs:           contractRefs,
			ChangedSymbolRefs:      changedSymbolRefs,
			ExpectsBaselineFailure: probe.ExpectsBaselineFailure,
		})
	}
	return out, ""
}

func normalizeProbeStringRefs(in []string, max int, field string) ([]string, string) {
	if len(in) == 0 {
		return nil, ""
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, ref := range in {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
		if len(out) > max {
			return nil, fmt.Sprintf("%s has more than %d non-empty entries", field, max)
		}
	}
	return out, ""
}

func validateVerificationProbeContractRefs(ctx *types.BusContext, probes []types.VerificationProbe) string {
	if len(probes) == 0 || ctx == nil || ctx.Mutable == nil {
		return ""
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return ""
	}
	contracts := ir.Request.BehaviorContracts
	if len(contracts) == 0 {
		return ""
	}
	ids := types.WriteBehaviorContractIDs(contracts)
	explicitRequired := types.RequiredWriteBehaviorContractIDs(contracts, false)
	coveredExplicitRequired := false
	for i, probe := range probes {
		for _, ref := range probe.ContractRefs {
			if _, ok := ids[ref]; !ok {
				return fmt.Sprintf("verification_probes[%d].contract_refs contains unknown behavior_contract id %q; use one of %s", i, ref, formatStringSet(ids))
			}
			if _, ok := explicitRequired[ref]; ok {
				coveredExplicitRequired = true
			}
		}
	}
	if len(explicitRequired) > 0 && !coveredExplicitRequired {
		return fmt.Sprintf("verification_probes must reference at least one required explicit behavior_contract via contract_refs; available required ids: %s", formatStringSet(explicitRequired))
	}
	return ""
}

func pythonVerificationProbeHasExecutableFailureSignal(code string) bool {
	stripped := stripPythonProbeStringsAndComments(code)
	for _, ident := range pythonProbeIdentifiers(stripped) {
		switch ident {
		case "assert", "raise":
			return true
		}
	}
	compact := compactPythonProbeSignalSurface(stripped)
	for _, signal := range []string{
		"sys.exit(",
		"exit(",
		"pytest.fail(",
	} {
		if strings.Contains(compact, signal) {
			return true
		}
	}
	return false
}

func stripPythonProbeStringsAndComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	quote := byte(0)
	triple := false
	escaped := false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if inString {
			if ch == '\n' {
				b.WriteByte('\n')
				if !triple {
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
				if triple {
					if i+2 < len(src) && src[i+1] == quote && src[i+2] == quote {
						b.WriteString("  ")
						i += 2
						inString = false
						triple = false
					}
				} else {
					inString = false
				}
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
		if ch == '\'' || ch == '"' {
			inString = true
			quote = ch
			triple = i+2 < len(src) && src[i+1] == ch && src[i+2] == ch
			b.WriteByte(' ')
			if triple {
				b.WriteString("  ")
				i += 2
			}
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func pythonProbeIdentifiers(src string) []string {
	var out []string
	for i := 0; i < len(src); {
		r := rune(src[i])
		if !isPythonIdentifierStart(r) {
			i++
			continue
		}
		start := i
		i++
		for i < len(src) && isPythonIdentifierPart(rune(src[i])) {
			i++
		}
		out = append(out, src[start:i])
	}
	return out
}

func compactPythonProbeSignalSurface(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if isPythonProbeASCIILetter(ch) || isPythonProbeASCIIDigit(ch) || ch == '_' || ch == '.' || ch == '(' {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func isPythonIdentifierStart(r rune) bool {
	return r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func isPythonIdentifierPart(r rune) bool {
	return isPythonIdentifierStart(r) || ('0' <= r && r <= '9')
}

func isPythonProbeASCIILetter(ch byte) bool {
	return ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isPythonProbeASCIIDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
