package logtriage

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ValidateInput is the raw LLM-emitted bundle before system validation.
// Fields mirror types.LogBundle's Layer 1-3; Layer 4 is NOT present in
// this type so there is no channel for the LLM to try to fill it. The
// log_triager agent constructs this from the emit_log_triage tool call
// and hands it to ValidateBundle.
type ValidateInput struct {
	Meta    types.LogMeta
	Errors  []types.LogError
	Residue types.LogResidue

	// RawLogBytes is the byte length of the original log the LLM saw.
	// Used by Coverage derivation as the denominator for
	// 1 - len(Residue)/len(raw). Zero means "unknown" and Coverage
	// collapses to 1.0 (treat as fully covered — caller's responsibility
	// to pass the real count when they want two-step escalation to
	// fire on low-coverage).
	RawLogBytes int
}

// ValidateBundle runs the 5-pass validator over a raw LLM emission and
// returns a populated LogBundle with Layer 4 filled. The result is
// safe to place on MutableState.SetLogTriage directly.
//
// Pass order:
//
//  1. Truncate Cause recursion beyond LogBundleCaps.MaxCauseDepth.
//  2. Normalise paths: StripBuildPathPrefix → runtime-internal filter →
//     os.Stat or Java basename resolver. Frames whose File cannot be
//     verified keep their Func/Pkg (Entities stay populated) but have
//     File cleared (they stay out of ResolvedFiles).
//  3. Sanity clamp: Confidence into [0.0, 1.0]; drop frames whose Raw
//     is empty (the emit schema requires Raw but we defence-in-depth).
//  4. Dedup + sort Signals (stable enum order).
//  5. Derive Layer 4: ResolvedFiles, Entities, IntentHint, Coverage.
//
// repoRoot is the absolute or relative path to the user repository;
// "" disables file resolution (every frame.File is cleared — used by
// unit tests that don't want to hit the filesystem).
func ValidateBundle(in ValidateInput, repoRoot string) *types.LogBundle {
	if len(in.Errors) == 0 && len(in.Residue.UnknownChunks) == 0 &&
		in.Meta.Lang == "" && len(in.Meta.Signals) == 0 {
		// Nothing to validate.
		return nil
	}

	out := &types.LogBundle{
		Meta:    in.Meta,
		Errors:  cloneErrors(in.Errors),
		Residue: in.Residue,
	}

	// Pass 1: recursion-depth truncate.
	for i := range out.Errors {
		out.Errors[i] = truncateCauseDepth(out.Errors[i], types.LogBundleCaps.MaxCauseDepth)
	}

	// Pass 2 + 3: resolve paths + sanity clamp. Walks the full Cause
	// tree so nested causes benefit from the same validation.
	if repoRoot != "" {
		repoBase := filepath.Base(filepath.Clean(repoRoot))
		walkErrors(out.Errors, func(e *types.LogError) {
			for j := range e.Frames {
				validateFrame(&e.Frames[j], repoRoot, repoBase)
			}
		})
	} else {
		walkErrors(out.Errors, func(e *types.LogError) {
			for j := range e.Frames {
				clampConfidence(&e.Frames[j])
				// No filesystem → no resolved path.
				e.Frames[j].File = ""
			}
		})
	}

	// Pass 4: dedup + reorder Signals to canonical enum order.
	out.Meta.Signals = normaliseSignals(in.Meta.Signals)

	// Pass 5: derive Layer 4.
	out.ResolvedFiles = deriveResolvedFiles(out.Errors)
	out.Entities = deriveEntities(out)
	out.IntentHint = deriveIntentHint(out)
	out.Coverage = deriveCoverage(in.RawLogBytes, in.Residue)

	return out
}

// MergeBundles combines N partial bundles produced by the two-step
// segmentation controller into one. Each input bundle already ran
// through ValidateBundle so its Layer 4 fields are populated. Merge
// strategy:
//
//   - Meta.Lang: most common non-empty value wins.
//   - Meta.Signals: set union, canonical order.
//   - Meta.Summary: non-empty values joined with "; " up to 200 chars.
//   - Errors: concatenated in input order; no cross-bundle dedup
//     (parallel error snapshots from different segments are
//     legitimately distinct).
//   - Residue.UnknownChunks: concatenated + uniqued by prefix.
//   - Layer 4 fields are RE-DERIVED from the merged Layer 1-3 so they
//     remain consistent with the final structured content.
func MergeBundles(parts []*types.LogBundle, rawLogBytes int) *types.LogBundle {
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts[0]
	}

	merged := &types.LogBundle{
		Meta: types.LogMeta{
			Lang: pickDominantLang(parts),
		},
	}

	// Signals: union.
	sigSet := make(map[types.LogSignal]bool)
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, s := range p.Meta.Signals {
			sigSet[s] = true
		}
	}
	for _, s := range types.AllLogSignals() {
		if sigSet[s] {
			merged.Meta.Signals = append(merged.Meta.Signals, s)
		}
	}

	// Summary: join.
	var summaries []string
	for _, p := range parts {
		if p == nil || p.Meta.Summary == "" {
			continue
		}
		summaries = append(summaries, p.Meta.Summary)
	}
	if len(summaries) > 0 {
		joined := strings.Join(summaries, "; ")
		if len(joined) > 200 {
			joined = joined[:200]
		}
		merged.Meta.Summary = joined
	}

	// Errors + Residue: concatenate.
	for _, p := range parts {
		if p == nil {
			continue
		}
		merged.Errors = append(merged.Errors, p.Errors...)
		merged.Residue.UnknownChunks = append(merged.Residue.UnknownChunks,
			p.Residue.UnknownChunks...)
	}

	// Unique UnknownChunks by full-string equality.
	merged.Residue.UnknownChunks = dedupeStrings(merged.Residue.UnknownChunks)

	// Re-derive Layer 4 (already-resolved Files need no re-Stat).
	merged.ResolvedFiles = deriveResolvedFiles(merged.Errors)
	merged.Entities = deriveEntities(merged)
	merged.IntentHint = deriveIntentHint(merged)
	merged.Coverage = deriveCoverage(rawLogBytes, merged.Residue)

	return merged
}

// MergeEntities is the helper the analyzer uses to union an existing
// entity list with the log_triage bundle's Entities slice. First-seen
// order is preserved; duplicates dropped by equality; caller does not
// need to pre-dedup either slice.
func MergeEntities(existing, fromBundle []string) []string {
	if len(fromBundle) == 0 {
		return existing
	}
	seen := make(map[string]bool, len(existing)+len(fromBundle))
	for _, e := range existing {
		seen[e] = true
	}
	out := make([]string, 0, len(existing)+len(fromBundle))
	out = append(out, existing...)
	cap := types.LogBundleCaps.MaxEntities
	for _, e := range fromBundle {
		if e == "" || seen[e] {
			continue
		}
		if len(out) >= cap {
			break
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// MergeResolvedFiles prepends the bundle's ResolvedFiles (validated,
// high-signal) in front of the analyzer's structural ranker output
// and applies LogBundleCaps.MaxResolvedFiles. Dedup preserves bundle
// order for log-derived entries and ranker order for fallbacks.
func MergeResolvedFiles(logFiles, ranked []string) []string {
	cap := types.LogBundleCaps.MaxResolvedFiles
	seen := make(map[string]bool, len(logFiles)+len(ranked))
	out := make([]string, 0, cap)
	for _, p := range logFiles {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) >= cap {
			return out
		}
	}
	for _, p := range ranked {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) >= cap {
			break
		}
	}
	return out
}

// ── internal helpers ─────────────────────────────────────────────

// cloneErrors deep-copies the slice so the validator's frame mutations
// never leak back into the caller's ValidateInput.
func cloneErrors(in []types.LogError) []types.LogError {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.LogError, len(in))
	for i, e := range in {
		out[i] = e
		if len(e.Frames) > 0 {
			out[i].Frames = append([]types.LogFrame(nil), e.Frames...)
		}
		if e.Cause != nil {
			c := cloneErrorPtr(e.Cause)
			out[i].Cause = c
		}
	}
	return out
}

func cloneErrorPtr(e *types.LogError) *types.LogError {
	if e == nil {
		return nil
	}
	out := *e
	if len(e.Frames) > 0 {
		out.Frames = append([]types.LogFrame(nil), e.Frames...)
	}
	if e.Cause != nil {
		out.Cause = cloneErrorPtr(e.Cause)
	}
	return &out
}

// truncateCauseDepth walks the Cause chain and severs it once depth
// exceeds maxDepth. depth counts the root as 1; maxDepth=5 allows up
// to 5 nested errors (root + 4 Cause hops). A severed chain logs
// nothing here — the caller renders [log_triage] warnings based on
// the returned bundle's depth mismatch with the LLM's emission.
func truncateCauseDepth(e types.LogError, maxDepth int) types.LogError {
	if maxDepth <= 0 {
		e.Cause = nil
		return e
	}
	head := e
	cur := &head
	for i := 1; i < maxDepth && cur.Cause != nil; i++ {
		cur = cur.Cause
	}
	if cur != nil {
		cur.Cause = nil
	}
	return head
}

// walkErrors invokes fn on each LogError in the tree, recursively
// including Cause chains. Order is stable: parallel slice order first,
// then depth-first into each Cause chain.
func walkErrors(errs []types.LogError, fn func(*types.LogError)) {
	for i := range errs {
		walkError(&errs[i], fn)
	}
}

func walkError(e *types.LogError, fn func(*types.LogError)) {
	if e == nil {
		return
	}
	fn(e)
	walkError(e.Cause, fn)
}

// validateFrame runs Pass 2 + 3 on one frame. Clamps Confidence,
// strips build prefix, filters runtime internals, resolves via
// filesystem. Mutates the frame in place.
func validateFrame(f *types.LogFrame, repoRoot, repoBase string) {
	clampConfidence(f)
	if f.Line < 0 {
		f.Line = 0
	}
	if f.File == "" {
		return
	}
	// Runtime internals never resolve inside user repos.
	if IsRuntimeInternalFile(f.File) {
		f.File = ""
		return
	}
	// Java basename: defer to the basename resolver via
	// GlobByBasename + ResolveJavaFile.
	if (f.Lang == "java" || isJavaBasename(f.File)) && !strings.ContainsAny(f.File, "/\\") {
		candidates, err := GlobByBasename(repoRoot, f.File)
		if err != nil || len(candidates) == 0 {
			f.File = ""
			return
		}
		ranked := ResolveJavaFile(f.Pkg, f.File, candidates)
		if len(ranked) == 0 {
			f.File = ""
			return
		}
		f.File = ranked[0]
		return
	}
	stripped := StripBuildPathPrefix(f.File, repoBase, nil)
	resolved := ResolveFrameFile(repoRoot, stripped)
	if resolved == "" {
		f.File = ""
		return
	}
	f.File = resolved
}

func clampConfidence(f *types.LogFrame) {
	if f.Confidence < 0 {
		f.Confidence = 0
	}
	if f.Confidence > 1 {
		f.Confidence = 1
	}
}

// normaliseSignals dedupes signals and returns them in canonical enum
// declaration order. Values outside the enum are dropped silently
// (the emit schema's enum constraint catches them first; this is
// defence-in-depth).
func normaliseSignals(in []types.LogSignal) []types.LogSignal {
	if len(in) == 0 {
		return nil
	}
	set := make(map[types.LogSignal]bool, len(in))
	for _, s := range in {
		if types.IsValidLogSignal(s) {
			set[s] = true
		}
	}
	var out []types.LogSignal
	for _, canonical := range types.AllLogSignals() {
		if set[canonical] {
			out = append(out, canonical)
		}
	}
	return out
}

// deriveResolvedFiles extracts unique file paths from frames whose
// File is non-empty (i.e. path resolved in repo), Line > 0, and
// Confidence >= MinResolvedConfidence. Ordering: Confidence desc,
// then path ascending for determinism.
func deriveResolvedFiles(errs []types.LogError) []string {
	type scored struct {
		path       string
		confidence float64
	}
	var hits []scored
	seen := make(map[string]bool)
	minConf := types.LogBundleCaps.MinResolvedConfidence
	walkErrors(errs, func(e *types.LogError) {
		for _, f := range e.Frames {
			if f.File == "" || f.Line <= 0 {
				continue
			}
			if f.Confidence < minConf {
				continue
			}
			if seen[f.File] {
				continue
			}
			seen[f.File] = true
			hits = append(hits, scored{path: f.File, confidence: f.Confidence})
		}
	})
	if len(hits) == 0 {
		return nil
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].confidence != hits[j].confidence {
			return hits[i].confidence > hits[j].confidence
		}
		return hits[i].path < hits[j].path
	})
	capN := types.LogBundleCaps.MaxResolvedFiles
	if len(hits) > capN {
		hits = hits[:capN]
	}
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.path
	}
	return out
}

// deriveEntities constructs the keyword list. Sources (in priority):
//
//   1. Frame.Func last-dotted segment (bare identifier)
//   2. Frame.Pkg (package / namespace hint)
//   3. Error.Type
//   4. Meta.Signals (as string)
//
// Each source is filtered by length sanity (>=2 chars for Func tokens;
// >=3 for Error.Type to suppress 2-char noise). Dedup is case-
// sensitive (stack-trace identifiers carry case semantics).
func deriveEntities(b *types.LogBundle) []string {
	if b == nil {
		return nil
	}
	cap := types.LogBundleCaps.MaxEntities
	seen := make(map[string]bool, cap)
	out := make([]string, 0, cap)
	push := func(tok string) bool {
		tok = strings.TrimSpace(tok)
		if tok == "" || seen[tok] || len(out) >= cap {
			return false
		}
		seen[tok] = true
		out = append(out, tok)
		return true
	}
	walkErrors(b.Errors, func(e *types.LogError) {
		for _, f := range e.Frames {
			if f.Func != "" {
				if idx := strings.LastIndex(f.Func, "."); idx >= 0 && idx < len(f.Func)-1 {
					tok := f.Func[idx+1:]
					if len(tok) >= 2 {
						push(tok)
					}
				} else if len(f.Func) >= 2 {
					push(f.Func)
				}
			}
			if f.Pkg != "" && len(f.Pkg) >= 2 {
				push(f.Pkg)
			}
		}
	})
	walkErrors(b.Errors, func(e *types.LogError) {
		if len(e.Type) >= 3 {
			push(e.Type)
		}
	})
	for _, s := range b.Meta.Signals {
		push(string(s))
	}
	return out
}

// deriveIntentHint returns IntentRootCause when the bundle contains at
// least one frame with File+Line>0 OR when Meta.Signals intersects
// {panic, crash, oom}. Otherwise returns the zero Intent.
func deriveIntentHint(b *types.LogBundle) types.Intent {
	if b == nil {
		return ""
	}
	hasRealFrame := false
	walkErrors(b.Errors, func(e *types.LogError) {
		if hasRealFrame {
			return
		}
		for _, f := range e.Frames {
			if f.File != "" && f.Line > 0 {
				hasRealFrame = true
				return
			}
		}
	})
	if hasRealFrame {
		return types.IntentRootCause
	}
	for _, s := range b.Meta.Signals {
		switch s {
		case types.SignalPanic, types.SignalCrash, types.SignalOOM:
			return types.IntentRootCause
		}
	}
	return ""
}

// deriveCoverage computes 1 - bytes(residue)/bytes(raw). Returns 1.0
// when rawLogBytes is 0 or negative (unknown).
func deriveCoverage(rawLogBytes int, residue types.LogResidue) float64 {
	if rawLogBytes <= 0 {
		return 1.0
	}
	used := 0
	for _, c := range residue.UnknownChunks {
		used += len(c)
	}
	if used >= rawLogBytes {
		return 0.0
	}
	return 1.0 - float64(used)/float64(rawLogBytes)
}

// pickDominantLang chooses the most-common non-empty Lang across
// parts; ties break on first seen. Empty input → "unknown".
func pickDominantLang(parts []*types.LogBundle) string {
	counts := make(map[string]int)
	order := []string{}
	for _, p := range parts {
		if p == nil || p.Meta.Lang == "" {
			continue
		}
		if _, ok := counts[p.Meta.Lang]; !ok {
			order = append(order, p.Meta.Lang)
		}
		counts[p.Meta.Lang]++
	}
	if len(counts) == 0 {
		return "unknown"
	}
	best := order[0]
	for _, l := range order {
		if counts[l] > counts[best] {
			best = l
		}
	}
	return best
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
