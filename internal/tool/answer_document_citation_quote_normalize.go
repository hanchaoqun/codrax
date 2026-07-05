package tool

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

func normalizeCurrentSourceCitationQuotes(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	// Observation-only runs (the user explicitly excluded current-source
	// analysis — typed ExcludesCurrentSource carrier) have no
	// current-source citations to repair by definition: the pass is
	// inert instead of reading files the user asked the run not to
	// touch (customer OOM 2026-07-03 was such a run).
	if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.ExcludesCurrentSource() {
		return 0
	}
	repoRoot := strings.TrimSpace(ctx.RepoRoot)
	if repoRoot == "" && ctx.Mutable != nil {
		repoRoot = strings.TrimSpace(ctx.Mutable.RepoRoot())
	}
	if repoRoot == "" {
		return 0
	}
	artifactPaths := runtimeArtifactCitationPathSet(ctx)
	// Per-document line cache: the same file cited N times is read once.
	// Quote normalization is optional enrichment, so an unreadable or
	// oversized file simply keeps the model's own quote — a nil cache
	// entry remembers the miss and never re-reads.
	lineCache := map[string][]string{}
	fixed := 0
	for i := range doc.Citations {
		cit := &doc.Citations[i]
		if cit.Line <= 0 || cit.LineEnd > cit.Line || strings.TrimSpace(cit.NegativePattern) != "" {
			continue
		}
		// A citation naming the attached/referenced runtime artifact is
		// runtime provenance, not current source — never read it as a
		// source file (typed artifact spelling match, defense in depth
		// on top of the size bound).
		if citationFileIsRuntimeArtifact(artifactPaths, cit.File) {
			continue
		}
		line, ok := currentSourceCitationLine(repoRoot, cit.File, cit.Line, lineCache)
		if !ok {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.TrimSpace(cit.Quote) == line {
			continue
		}
		cit.Quote = line
		fixed++
	}
	return fixed
}

func currentSourceCitationLine(repoRoot, file string, lineNo int, lineCache map[string][]string) (string, bool) {
	if lineNo <= 0 {
		return "", false
	}
	lines, ok := currentSourceCitationLines(repoRoot, file, lineCache)
	if !ok || lineNo > len(lines) {
		return "", false
	}
	return lines[lineNo-1], true
}

func currentSourceCitationLines(repoRoot, file string, lineCache map[string][]string) ([]string, bool) {
	path, ok := currentSourceCitationPath(repoRoot, file)
	if !ok {
		return nil, false
	}
	if lineCache != nil {
		if cached, seen := lineCache[path]; seen {
			return cached, cached != nil
		}
	}
	// Bounded read (customer OOM 2026-07-03): a citation file field can
	// name a multi-GiB runtime artifact inside the repo directory —
	// stat first and skip this optional quote repair instead of slurping
	// the file (the incident slurped a 1104 MiB trace at finalize and
	// lost the whole run to a Windows commit-limit OOM).
	data, err := width.ReadFileBounded(path, 0)
	if err != nil {
		var oversized *width.ErrSourceReadOversized
		if errors.As(err, &oversized) {
			logging.Warning("[emit_answer_document] citation quote normalize skipped %s: %v", file, err)
		}
		if lineCache != nil {
			lineCache[path] = nil
		}
		return nil, false
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if lineCache != nil {
		lineCache[path] = lines
	}
	return lines, true
}

func currentSourceCitationPath(repoRoot, file string) (string, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	file = strings.TrimSpace(file)
	if repoRoot == "" || file == "" {
		return "", false
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", false
	}
	var path string
	if filepath.IsAbs(file) {
		path = filepath.Clean(file)
	} else {
		path = filepath.Join(root, filepath.FromSlash(file))
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", false
	}
	return path, true
}

// answerDocumentHasQuotelessCurrentSourceCitation reports whether any
// citation normalizeCurrentSourceCitationQuotes could plausibly enrich
// (single-line, non-negative-pattern, not a runtime artifact) is still
// missing a quote. Gate for the post-repair quote passes (end of the
// pre-emit chain + pre-persist): repair passes mint citations as bare
// file:line, and the first pass runs before those citations exist.
//
// Honest bound (QCE review 2026-07-05 finding 9): the gate cannot tell a
// freshly minted bare citation from a model-emitted bare citation the first
// pass already failed to fill (missing file, oversized, out-of-repo path) —
// for the latter it re-opens on every later pass at the bounded cost of one
// stat/read attempt per cited file per pass. Runtime-artifact citations are
// excluded here so a GiB-scale trace attachment never keeps the gate open.
func answerDocumentHasQuotelessCurrentSourceCitation(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil {
		return false
	}
	artifactPaths := runtimeArtifactCitationPathSet(ctx)
	for _, cit := range doc.Citations {
		if cit.Line <= 0 || cit.LineEnd > cit.Line || strings.TrimSpace(cit.NegativePattern) != "" {
			continue
		}
		if citationFileIsRuntimeArtifact(artifactPaths, cit.File) {
			continue
		}
		if strings.TrimSpace(cit.Quote) == "" {
			return true
		}
	}
	return false
}

func logCurrentSourceCitationQuoteRepairs(toolName string, fixed int) {
	if fixed <= 0 {
		return
	}
	logging.Warning("[%s] repaired %d citation quote(s) from current source file:line", toolName, fixed)
}

// runtimeArtifactCitationPathSet collects the typed attachment spellings of
// the run's runtime artifacts (trace / log), normalized to slash-form plus
// basename, so citation files naming them are recognized verbatim — never
// by content sniffing or prose matching.
//
// Spellings are stored case-folded (CSP #63 / CPD 核验 P3): the path-shape
// lane (types.RuntimeArtifactPathKind) already lowercases before matching,
// so a model-drifted case variant like `Attached_Trace.txt` was recognized
// by the shape lane but slipped past this typed set on the three faces that
// consult only the set (citation quote normalize ×2 + current-source
// metadata surface terms). Folding both sides realigns the two lanes; real
// source paths are unaffected because they are never in this set.
func runtimeArtifactCitationPathSet(ctx *types.BusContext) map[string]bool {
	if ctx == nil {
		return nil
	}
	out := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw == "-" {
			return
		}
		clean := strings.ToLower(filepath.ToSlash(filepath.Clean(raw)))
		out[clean] = true
		if base := filepath.Base(clean); base != "" && base != "." {
			out[base] = true
		}
	}
	// AttachedHitraceSource is the trace attachment's user spelling —
	// the GiB-scale artifact class this guard exists for. AttachedLog
	// carries pasted excerpt CONTENT, not a path, so it has no spelling
	// to match; log-path citations are already covered by the size
	// bound.
	add(ctx.AttachedHitraceSource)
	// CPD #58 (2026-07-05): the blob MATERIALIZATION spellings are typed
	// too — Codrax itself writes the attachment body to
	// `<WorkDir>/attached_trace.txt` (and companions), and models cite
	// that path verbatim (donghu specimen: 4 citations naming
	// `.codrax/blob/<session>/attached_trace.txt:<line>` rendered as a
	// source bibliography). The basenames are SYSTEM-RESERVED
	// (types.ReservedRuntimeArtifactBlobBasenames, single authority with
	// the blob writer), so a basename match is a reservation match, not
	// content sniffing. This aligns the typed spelling lane with the
	// path-shape lane, which already recognized these names.
	for _, reserved := range types.ReservedRuntimeArtifactBlobBasenames() {
		add(reserved)
	}
	return out
}

// citationFileIsRuntimeArtifact reports whether a citation file field names
// one of the run's runtime artifacts (case-folded cleaned-path or basename
// match — the set side folds too; see runtimeArtifactCitationPathSet).
func citationFileIsRuntimeArtifact(artifactPaths map[string]bool, file string) bool {
	if len(artifactPaths) == 0 {
		return false
	}
	file = strings.TrimSpace(file)
	if file == "" {
		return false
	}
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(file)))
	return artifactPaths[clean] || artifactPaths[filepath.Base(clean)]
}
