// Package perftriage holds validation + merge helpers for the perf
// channel, mirroring internal/analysis/logtriage. Today the validator
// surface is small (the emit_perf_trace tool inlines its derivation
// logic in tool/emit_perf_trace.go::derivePerfLayer4) so this package
// only carries the cross-bundle Merge operation needed by the
// two-step controller — but keeping it as a package leaves room for
// a future ValidateBundle / ResolvePerfFile / SetSourcePrefix
// surface to grow without churning import paths.
package perftriage

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// MergePerfBundles combines N partial PerfBundles produced by the
// two-step segmentation controller into one. Each input has already
// passed through emit_perf_trace.derivePerfLayer4, so the inputs
// carry validated Layer 1-4 data; this function unions Layer 1-3
// then re-derives Layer 4 to stay consistent with the merged content.
//
// Merge strategy (mirrors logtriage.MergeBundles for symmetry):
//
//   - Meta.Source: most common non-empty value wins. Ties → first.
//   - Meta.DurationMs: max of non-zero values (the trace span the
//     LLM saw is the longest segment that overlapped wall-clock).
//   - Meta.AppPID: most common non-zero value wins.
//   - Meta.Signals: set union; canonical alphabetical order so two
//     identical merges yield identical bundles (test stability).
//   - Meta.Summary: non-empty values joined with "; " up to 200 chars.
//   - Frames / Janks / Stalls: concatenated in input order; cross-
//     bundle dedup by (start_ts_ms, duration_ms) — segments may
//     overlap on jank-region boundaries and the LLM might surface
//     the same span from both sides.
//   - Startup: first non-nil wins (a trace covers at most one cold-
//     /warm-/hot-start envelope; if multiple bundles claim startup,
//     trust the one with the largest app_launch_ms).
//   - Residue: concatenate + dedupe.
//   - Layer 4 (ResolvedFiles / Entities / IntentHint / Coverage):
//     re-derived inline (the helpers below stay package-local
//     because they're only sensible for already-validated inputs).
//
// rawTraceBytes is the byte length of the original trace the
// segmenter saw; used as the Coverage denominator. Zero collapses
// Coverage to 1.0 (matches LogBundle behaviour).
func MergePerfBundles(parts []*types.PerfBundle, rawTraceBytes int) *types.PerfBundle {
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts[0]
	}

	merged := &types.PerfBundle{
		Meta: types.PerfMeta{
			Source: pickDominantSource(parts),
			AppPID: pickDominantPID(parts),
		},
	}

	// DurationMs: max of non-zero durations.
	for _, p := range parts {
		if p == nil {
			continue
		}
		if p.Meta.DurationMs > merged.Meta.DurationMs {
			merged.Meta.DurationMs = p.Meta.DurationMs
		}
	}

	// Signals: union, alphabetical.
	sigSet := map[string]bool{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, s := range p.Meta.Signals {
			sigSet[s] = true
		}
	}
	if len(sigSet) > 0 {
		out := make([]string, 0, len(sigSet))
		for s := range sigSet {
			out = append(out, s)
		}
		sort.Strings(out)
		merged.Meta.Signals = out
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

	// Frames / Janks / Stalls: concat + dedup by signature.
	frameSeen := map[string]bool{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, f := range p.Frames {
			key := frameKey(f)
			if frameSeen[key] {
				continue
			}
			frameSeen[key] = true
			merged.Frames = append(merged.Frames, f)
		}
	}
	jankSeen := map[string]bool{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, j := range p.Janks {
			key := jankKey(j)
			if jankSeen[key] {
				continue
			}
			jankSeen[key] = true
			merged.Janks = append(merged.Janks, j)
		}
	}
	stallSeen := map[string]bool{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, s := range p.Stalls {
			key := stallKey(s)
			if stallSeen[key] {
				continue
			}
			stallSeen[key] = true
			merged.Stalls = append(merged.Stalls, s)
		}
	}

	// Startup: take the entry with the largest app_launch_ms; falls
	// back to first non-nil when launches tie at 0.
	var bestStart *types.PerfStartup
	for _, p := range parts {
		if p == nil || p.Startup == nil {
			continue
		}
		if bestStart == nil || p.Startup.AppLaunchMs > bestStart.AppLaunchMs {
			bestStart = p.Startup
		}
	}
	merged.Startup = bestStart

	// Residue: concat + dedupe (full-string equality).
	resSeen := map[string]bool{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, r := range p.Residue {
			if r == "" || resSeen[r] {
				continue
			}
			resSeen[r] = true
			merged.Residue = append(merged.Residue, r)
		}
	}

	// Re-derive Layer 4. The derivations live inline (small enough
	// to keep readable) rather than reaching into emit_perf_trace.go
	// to avoid an analysis→tool import cycle.
	mergedDeriveEntities(merged)
	mergedDeriveResolvedFiles(merged)
	mergedDeriveIntentHint(merged)
	merged.Coverage = mergedDeriveCoverage(rawTraceBytes, merged.Residue)

	return merged
}

// frameKey produces a stable signature for de-dup. Two frames with
// the same (frame_no, ts_ms, duration_ms) are treated as the same
// observation across segments.
func frameKey(f types.PerfFrame) string {
	return joinKey(intStr(f.FrameNo), float3(f.TsMs), float3(f.DurationMs))
}

func jankKey(j types.PerfJank) string {
	return joinKey(float3(j.StartTsMs), float3(j.DurationMs), j.TriggerSpan)
}

func stallKey(s types.PerfStall) string {
	return joinKey(float3(s.StartTsMs), float3(s.DurationMs), s.Symbol)
}

// pickDominantSource returns the most common non-empty Source across
// parts; ties resolve to the first occurrence (stable order).
func pickDominantSource(parts []*types.PerfBundle) string {
	counts := map[string]int{}
	first := map[string]int{}
	for i, p := range parts {
		if p == nil || p.Meta.Source == "" {
			continue
		}
		if _, seen := first[p.Meta.Source]; !seen {
			first[p.Meta.Source] = i
		}
		counts[p.Meta.Source]++
	}
	best := ""
	bestCount := 0
	for s, c := range counts {
		if c > bestCount || (c == bestCount && first[s] < first[best]) {
			best = s
			bestCount = c
		}
	}
	return best
}

// pickDominantPID returns the most common non-zero AppPID; same
// stability rules as pickDominantSource.
func pickDominantPID(parts []*types.PerfBundle) int {
	counts := map[int]int{}
	first := map[int]int{}
	for i, p := range parts {
		if p == nil || p.Meta.AppPID == 0 {
			continue
		}
		if _, seen := first[p.Meta.AppPID]; !seen {
			first[p.Meta.AppPID] = i
		}
		counts[p.Meta.AppPID]++
	}
	best := 0
	bestCount := 0
	for pid, c := range counts {
		if c > bestCount || (c == bestCount && first[pid] < first[best]) {
			best = pid
			bestCount = c
		}
	}
	return best
}

// mergedDeriveEntities collects unique trigger spans + tags + stall
// symbols + startup mode label, capped at 32. Mirrors
// tool/emit_perf_trace.go::derivePerfLayer4 but operates on the
// already-merged bundle so we don't duplicate derivation across
// every part.
func mergedDeriveEntities(b *types.PerfBundle) {
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || len(b.Entities) >= 32 {
			return
		}
		seen[s] = true
		b.Entities = append(b.Entities, s)
	}
	for _, j := range b.Janks {
		add(j.TriggerSpan)
		for _, t := range j.Tags {
			add(t)
		}
	}
	for _, s := range b.Stalls {
		add(s.Symbol)
		if s.Kind != "" {
			add(s.Kind)
		}
	}
	if b.Startup != nil && b.Startup.Mode != "" {
		add(b.Startup.Mode + "-start")
	}
}

// mergedDeriveResolvedFiles unions every Stall.File observation
// across the merged bundle, capped at 10.
func mergedDeriveResolvedFiles(b *types.PerfBundle) {
	seen := map[string]bool{}
	for _, s := range b.Stalls {
		if s.File == "" || seen[s.File] {
			continue
		}
		seen[s.File] = true
		if len(b.ResolvedFiles) >= 10 {
			break
		}
		b.ResolvedFiles = append(b.ResolvedFiles, s.File)
	}
}

// mergedDeriveIntentHint mirrors derivePerfLayer4: any jank or stall
// or slow cold start promotes to "performance".
func mergedDeriveIntentHint(b *types.PerfBundle) {
	if len(b.Janks) > 0 || len(b.Stalls) > 0 ||
		(b.Startup != nil && b.Startup.AppLaunchMs > types.PerfStartupSlowColdMs) {
		b.IntentHint = "performance"
	}
}

// mergedDeriveCoverage approximates how much of the original trace
// the merged bundle structured. The denominator is the original raw
// byte count; the numerator is rawBytes minus the residue bytes.
// Returns clamped [0, 1]. Matches logtriage.deriveCoverage shape.
func mergedDeriveCoverage(rawBytes int, residue []string) float64 {
	if rawBytes <= 0 {
		return 1.0
	}
	resBytes := 0
	for _, r := range residue {
		resBytes += len(r)
	}
	if resBytes >= rawBytes {
		return 0
	}
	return 1.0 - float64(resBytes)/float64(rawBytes)
}

// --- minimal stdlib-only helpers (kept local to this file) ---

func joinKey(parts ...string) string {
	return strings.Join(parts, "|")
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// float3 renders a float64 with 3 decimal places of precision so
// dedup keys are stable across LLM emissions that report the same
// timestamp with slightly different round-off.
func float3(v float64) string {
	return formatFloat3(v)
}

// formatFloat3 is split out so tests can compare without depending
// on strconv.FormatFloat semantics drifting under Go upgrades.
func formatFloat3(v float64) string {
	// Multiply by 1000, round to integer, format. Avoids importing
	// strconv just for this and is good enough for ms timestamps.
	scaled := int64(v*1000.0 + 0.5)
	if v < 0 {
		scaled = int64(v*1000.0 - 0.5)
	}
	whole := scaled / 1000
	frac := scaled - whole*1000
	if frac < 0 {
		frac = -frac
	}
	wholePart := intStr(int(whole))
	fracPart := pad3(int(frac))
	return wholePart + "." + fracPart
}

func pad3(n int) string {
	if n < 10 {
		return "00" + intStr(n)
	}
	if n < 100 {
		return "0" + intStr(n)
	}
	return intStr(n)
}
