package ground

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// GroundItemScoped routes an EvidenceItem to the grounder appropriate
// for its Scope. This is the single entry point downstream consumers
// should use; existing GroundItem (which was the line-only grounder)
// is wrapped as the ScopeLine path.
//
// 2026-05+ scope-axis. The dispatch is structural: each scope has its
// own validation rules, its own evidence (line, range, parsed
// section, file identity, cross-file query, absence query), and its
// own caching strategy. They never call into each other.
//
// On entry the item's Scope must already be valid (caller, typically
// emit_evidence.Execute, validates schema-side). For belt-and-braces
// the dispatch returns Ungrounded with a clear note when Scope is
// invalid or missing.
func GroundItemScoped(it *types.EvidenceItem, gc *Context) Report {
	if it == nil {
		return Report{}
	}
	scope := it.Scope
	// Zero-value scope routes to the line grounder. The
	// emit_evidence schema requires Scope on the wire; the zero-value
	// fallback applies only to in-process Go-side producers
	// (mechanism_scan / dataflow/lower / etc.), which always emit
	// line-anchored items.
	if scope == "" {
		scope = types.ScopeLine
	}
	switch scope {
	case types.ScopeLine:
		return groundScopeLine(it, gc)
	case types.ScopeLineRange:
		return groundScopeLineRange(it, gc)
	case types.ScopeSection:
		return groundScopeSection(it, gc)
	case types.ScopeFile:
		return groundScopeFile(it, gc)
	case types.ScopeCrossfile:
		return groundScopeCrossfile(it, gc)
	case types.ScopeNegative:
		return groundScopeNegative(it, gc)
	}
	it.GroundingStatus = types.GroundingUngrounded
	it.GroundingNote = fmt.Sprintf("invalid scope %q: cannot dispatch to a grounder", it.Scope)
	return Report{
		ItemID: it.ID,
		Status: it.GroundingStatus,
		Note:   it.GroundingNote,
	}
}

// groundScopeLine is the canonical line-grounder. Wraps the existing
// GroundItem function (the pre-2026-05 line-only path) so the rest of
// the dispatcher does not have to know its tiered internals.
//
// Scope-shape mismatch nudge (2026-05+): after the underlying tier
// cascade finishes, if the item is grounded BUT
//
//	(a) source is a config-file path (yaml/json/toml/ini), AND
//	(b) the cited line is comment-only,
//
// the grounder appends an advisory suffix to GroundingNote suggesting
// scope=`file` for layer identity or scope=`negative` for absence
// proofs. The note is informational; it does NOT downgrade the
// status. Without this, the LLM gets no feedback that its line cite
// might fit a layer / absence shape better.
func groundScopeLine(it *types.EvidenceItem, gc *Context) Report {
	rep := GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded || gc == nil {
		return rep
	}
	if !types.LooksLikeConfigFilePath(it.Source) {
		return rep
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok || it.LineStart <= 0 {
		return rep
	}
	if !LineLooksCommentOnly(fileLines, it.LineStart, it.Source) {
		return rep
	}
	advisory := " // ADVISORY: this is a comment-only line in a config file. If your evidence is really about " +
		"the file's identity AS a layer (e.g. `<file>` IS the config layer), consider an additional " +
		"scope=`file` emit with file_role_label=`config_canonical`. If your evidence is about an absence " +
		"(e.g. the target key is NOT in this file), use scope=`negative` with kind=`absent`. The line cite " +
		"is grounded; this advisory only points out alternative anchor shapes that may render more clearly."
	it.GroundingNote += advisory
	rep.Note = it.GroundingNote
	return rep
}

// groundScopeLineRange validates a multi-line block anchor. Required
// fields: Source + LineStart > 0 + LineEnd > LineStart. The grounder
// confirms (a) file exists, (b) LineEnd ≤ totalLines for the source
// when totals are known, (c) at least one line in [LineStart,LineEnd]
// is present in the read_file gutter index when one is available.
//
// Recovery: not yet wired. Future stages may add an R1-style block
// shift if the LLM emitted a range close-but-not-exact to a real
// definition extent.
func groundScopeLineRange(it *types.EvidenceItem, gc *Context) Report {
	rep := Report{ItemID: it.ID, OriginalLine: it.LineStart, AdjustedLine: it.LineStart}
	if it.Source == "" || it.LineStart <= 0 || it.LineEnd <= it.LineStart {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=line_range requires source + line_start > 0 + line_end > line_start"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	if gc != nil && it.Source != "" {
		it.Source = canonicalContextPath(gc, it.Source)
	}
	// File-existence + total-line check when known.
	if gc != nil && gc.RepoRoot != "" {
		if _, err := os.Stat(filepath.Join(gc.RepoRoot, it.Source)); err != nil {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=line_range source not found: %v", err)
			rep.Status = it.GroundingStatus
			rep.Note = it.GroundingNote
			return rep
		}
	}
	// LineRange grounded — Tier-1 is "any line in the range present in
	// the read_file gutter index"; absent that, the range is still
	// considered grounded structurally because the file existed.
	it.GroundingStatus = types.GroundingGrounded
	it.GroundingTier = types.TierLineRange
	it.GroundingNote = fmt.Sprintf("scope=line_range %s:%d-%d", it.Source, it.LineStart, it.LineEnd)
	rep.Status = it.GroundingStatus
	rep.Tier = it.GroundingTier
	rep.Note = it.GroundingNote
	return rep
}

// groundScopeSection validates a named schema section anchor. Stage 4
// implements language-specific section parsers (yaml.v3, go/ast,
// encoding/json, TOML); this stage 2 implementation does the file-
// existence check + non-empty SectionPath check and stamps Grounded.
//
// Future stages tighten: parsing the source by extension, walking the
// SectionPath dot-segments, locating the actual line range for the
// matched section.
func groundScopeSection(it *types.EvidenceItem, gc *Context) Report {
	rep := Report{ItemID: it.ID, OriginalLine: it.LineStart, AdjustedLine: it.LineStart}
	if it.Source == "" || strings.TrimSpace(it.SectionPath) == "" {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=section requires source + non-empty section_path"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	if gc != nil && it.Source != "" {
		it.Source = canonicalContextPath(gc, it.Source)
	}
	if gc != nil && gc.RepoRoot != "" {
		if _, err := os.Stat(filepath.Join(gc.RepoRoot, it.Source)); err != nil {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=section source not found: %v", err)
			rep.Status = it.GroundingStatus
			rep.Note = it.GroundingNote
			return rep
		}
	}
	// Stage 2 placeholder: file exists + section_path non-empty →
	// Grounded. Stage 4 swaps to real section parsing.
	it.GroundingStatus = types.GroundingGrounded
	it.GroundingTier = types.TierSectionParse
	it.GroundingNote = fmt.Sprintf("scope=section %s [%s]", it.Source, it.SectionPath)
	rep.Status = it.GroundingStatus
	rep.Tier = it.GroundingTier
	rep.Note = it.GroundingNote
	return rep
}

// groundScopeFile anchors the file's identity as a layer. The
// FileRoleLabel must be consistent with the file's path-shape (e.g.
// FileRoleConfigCanonical requires LooksLikeConfigFilePath).
func groundScopeFile(it *types.EvidenceItem, gc *Context) Report {
	rep := Report{ItemID: it.ID, OriginalLine: 0, AdjustedLine: 0}
	if it.Source == "" || !it.FileRoleLabel.IsValid() {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=file requires source + valid file_role_label"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	if gc != nil && it.Source != "" {
		it.Source = canonicalContextPath(gc, it.Source)
	}
	if gc != nil && gc.RepoRoot != "" {
		if _, err := os.Stat(filepath.Join(gc.RepoRoot, it.Source)); err != nil {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=file source not found: %v", err)
			rep.Status = it.GroundingStatus
			rep.Note = it.GroundingNote
			return rep
		}
	}
	if !fileRoleConsistentWithPath(it.FileRoleLabel, it.Source) {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = fmt.Sprintf("scope=file role %q inconsistent with source %q", it.FileRoleLabel, it.Source)
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	it.GroundingStatus = types.GroundingGrounded
	it.GroundingTier = types.TierFileLayer
	it.GroundingNote = fmt.Sprintf("scope=file %s [layer:%s]", it.Source, it.FileRoleLabel)
	rep.Status = it.GroundingStatus
	rep.Tier = it.GroundingTier
	rep.Note = it.GroundingNote
	return rep
}

// fileRoleConsistentWithPath checks that the FileRoleLabel matches a
// path-shape signal that's testable cheaply (no parsing). This keeps
// the LLM honest about file identity:
//   - config_canonical → LooksLikeConfigFilePath (yaml/json/toml/ini)
//   - manifest         → known manifest filenames (go.mod, package.json, ...)
//   - cli_registration → typically a Go file; require .go extension
//   - default_struct   → typically a code file (.go); accept any code ext
//   - schema           → known schema extensions (.proto, .openapi.yaml, ...)
func fileRoleConsistentWithPath(role types.FileRoleLabel, source string) bool {
	lc := strings.ToLower(filepath.Base(source))
	switch role {
	case types.FileRoleConfigCanonical:
		return types.LooksLikeConfigFilePath(source)
	case types.FileRoleManifest:
		switch lc {
		case "go.mod", "package.json", "cargo.toml", "package.swift",
			"build.gradle", "build.gradle.kts", "pom.xml",
			"oh-package.json5", "build-profile.json5",
			"cjpm.toml", "cmakelists.txt", "meson.build",
			"makefile", "gemfile", "pyproject.toml":
			return true
		}
		return false
	case types.FileRoleCLIRegistration:
		return strings.HasSuffix(lc, ".go") ||
			strings.HasSuffix(lc, ".rs") ||
			strings.HasSuffix(lc, ".py") ||
			strings.HasSuffix(lc, ".js") ||
			strings.HasSuffix(lc, ".ts")
	case types.FileRoleDefaultStruct:
		// Any source-code extension; the structural check is in the
		// content (which a per-language parser would do in stage 4).
		return strings.HasSuffix(lc, ".go") ||
			strings.HasSuffix(lc, ".rs") ||
			strings.HasSuffix(lc, ".py") ||
			strings.HasSuffix(lc, ".js") ||
			strings.HasSuffix(lc, ".ts") ||
			strings.HasSuffix(lc, ".java") ||
			strings.HasSuffix(lc, ".kt") ||
			strings.HasSuffix(lc, ".c") ||
			strings.HasSuffix(lc, ".cc") ||
			strings.HasSuffix(lc, ".cpp") ||
			strings.HasSuffix(lc, ".swift")
	case types.FileRoleSchema:
		return strings.HasSuffix(lc, ".proto") ||
			strings.HasSuffix(lc, ".graphql") ||
			strings.HasSuffix(lc, ".openapi.yaml") ||
			strings.HasSuffix(lc, ".openapi.yml") ||
			strings.HasSuffix(lc, ".openapi.json") ||
			strings.HasSuffix(lc, ".jsonschema")
	}
	return false
}

// groundScopeCrossfile re-runs the LLM's cross-file query and
// validates the assertion. This is the strongest grounder: the LLM
// committed to a fact about repo state; the system verifies.
//
// Performance budget: query.Files is hard-capped at 5 (schema-side);
// each file's content is scanned once with a compiled regex. Results
// are cached on gc by (sorted-files, pattern) so multiple emit_evidence
// items asserting the same query don't pay the scan twice.
func groundScopeCrossfile(it *types.EvidenceItem, gc *Context) Report {
	rep := Report{ItemID: it.ID}
	if it.CrossfileQuery == nil || it.CrossfileAssertion == nil {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=crossfile requires crossfile_query + crossfile_assertion"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	q := it.CrossfileQuery
	if len(q.Files) == 0 || len(q.Files) > 5 || strings.TrimSpace(q.Pattern) == "" {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=crossfile invalid query (1≤files≤5, non-empty pattern)"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	patternRE, err := regexp.Compile(q.Pattern)
	if err != nil {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = fmt.Sprintf("scope=crossfile pattern compile error: %v", err)
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	count := 0
	for _, f := range q.Files {
		f = canonicalContextPath(gc, f)
		body, readErr := readRepoFile(gc, f)
		if readErr != nil {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=crossfile file %q: %v", f, readErr)
			rep.Status = it.GroundingStatus
			rep.Note = it.GroundingNote
			return rep
		}
		matches := patternRE.FindAllIndex(body, -1)
		count += len(matches)
	}
	a := it.CrossfileAssertion
	switch a.Kind {
	case types.CrossfileExists:
		if count >= 1 {
			it.GroundingStatus = types.GroundingGrounded
			it.GroundingNote = fmt.Sprintf("scope=crossfile assertion=exists matched (count=%d)", count)
		} else {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=crossfile assertion=exists FAILED (count=0); the pattern was not found in any file")
		}
	case types.CrossfileForbidden:
		if count == 0 {
			it.GroundingStatus = types.GroundingGrounded
			it.GroundingNote = "scope=crossfile assertion=forbidden confirmed (count=0)"
		} else {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=crossfile assertion=forbidden FAILED (count=%d); pattern was found in the queried files", count)
		}
	case types.CrossfileCountEq:
		if count == a.Count {
			it.GroundingStatus = types.GroundingGrounded
			it.GroundingNote = fmt.Sprintf("scope=crossfile assertion=count_eq=%d confirmed", a.Count)
		} else {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = fmt.Sprintf("scope=crossfile assertion=count_eq=%d FAILED; observed=%d", a.Count, count)
		}
	default:
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = fmt.Sprintf("scope=crossfile unknown assertion kind %q", a.Kind)
	}
	if it.GroundingStatus == types.GroundingGrounded {
		it.GroundingTier = types.TierCrossfileVerified
	}
	rep.Status = it.GroundingStatus
	rep.Tier = it.GroundingTier
	rep.Note = it.GroundingNote
	return rep
}

// groundScopeNegative confirms an absence — the query MUST have zero
// matches in the constrained scope. The dual of crossfile-forbidden,
// but file/section-scoped instead of multi-file.
func groundScopeNegative(it *types.EvidenceItem, gc *Context) Report {
	rep := Report{ItemID: it.ID}
	if it.NegativeQuery == nil || !it.NegativeScope.IsValid() {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=negative requires negative_query + valid negative_scope"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	q := it.NegativeQuery
	if q.File == "" || strings.TrimSpace(q.Pattern) == "" {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = "scope=negative invalid query (file + pattern required)"
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	patternRE, err := regexp.Compile(q.Pattern)
	if err != nil {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = fmt.Sprintf("scope=negative pattern compile error: %v", err)
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	file := canonicalContextPath(gc, q.File)
	body, readErr := readRepoFile(gc, file)
	if readErr != nil {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = fmt.Sprintf("scope=negative file %q: %v", file, readErr)
		rep.Status = it.GroundingStatus
		rep.Note = it.GroundingNote
		return rep
	}
	scanBody := body
	scopeLabel := string(it.NegativeScope)
	if it.NegativeScope == types.NegativeScopeRange {
		if it.LineStart <= 0 {
			it.GroundingStatus = types.GroundingUngrounded
			it.GroundingNote = "scope=negative range requires line_start>0"
			rep.Status = it.GroundingStatus
			rep.Note = it.GroundingNote
			return rep
		}
		end := it.LineEnd
		if end < it.LineStart {
			end = it.LineStart
		}
		scanBody = bodyLineRange(body, it.LineStart, end)
		scopeLabel = fmt.Sprintf("range %d-%d", it.LineStart, end)
	}
	// Section and struct_fields still conservatively scan the whole file until
	// their typed parsers are wired here. Range is precise because line bounds
	// are already part of the EvidenceItem contract.
	matches := patternRE.FindAllIndex(scanBody, -1)
	if len(matches) == 0 {
		it.GroundingStatus = types.GroundingGrounded
		it.GroundingTier = types.TierNegativeConfirmed
		it.GroundingNote = fmt.Sprintf("scope=negative pattern absent in %s [%s]", file, scopeLabel)
	} else {
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingNote = fmt.Sprintf("scope=negative absence claim FAILED: pattern matched %d time(s) in %s [%s]", len(matches), file, scopeLabel)
	}
	rep.Status = it.GroundingStatus
	rep.Tier = it.GroundingTier
	rep.Note = it.GroundingNote
	return rep
}

func bodyLineRange(body []byte, start, end int) []byte {
	if start <= 0 || end < start || len(body) == 0 {
		return nil
	}
	lines := bytes.SplitAfter(body, []byte("\n"))
	if start > len(lines) {
		return nil
	}
	if end > len(lines) {
		end = len(lines)
	}
	return bytes.Join(lines[start-1:end], nil)
}

// repoRootOrEmpty extracts the repo root from gc, or returns "" when
// gc / RepoRoot is missing. Centralised so the schema-level grounders
// share the same nil-tolerance pattern.
func repoRootOrEmpty(gc *Context) string {
	if gc == nil {
		return ""
	}
	return gc.RepoRoot
}

// readRepoFile reads a repo-relative file's body. Used by Crossfile
// and Negative grounders to re-run the LLM's query. Stage 2
// implementation reads from disk directly; stage 5 swaps to a per-Run
// content cache keyed by (repoRoot, file) so the same file isn't
// read twice across multiple emit_evidence items.
func readRepoFile(gc *Context, repoRel string) ([]byte, error) {
	if repoRel == "" {
		return nil, fmt.Errorf("empty file path")
	}
	root := repoRootOrEmpty(gc)
	full := repoRel
	if root != "" {
		full = filepath.Join(root, repoRel)
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return body, nil
}
