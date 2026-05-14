package types

import (
	"fmt"
	"strings"
)

// compileObservedArtifactSupportLane builds the typed observed-
// artifact lane consumed by every family compiler. The lane carries
// frames / spans / signals extracted from an attached runtime
// artifact (log / perf trace / external observation) and explicitly
// preserves the boundary between "observation fact" and "current code
// mechanism": entries here may explain what was observed but do not
// prove caller-side provenance unless a separately-cited current-code
// line establishes that mapping. AllowedBlocks widens to ordered_list
// / bullet_list / diagram only when the dispatch is in a runtime-
// observation-only mode (no current repo intersection) so the LLM can
// list observed frames as the principal answer.
func compileObservedArtifactSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	allowedBlocks := []string{"summary", "caveat"}
	if runtimeObservationOnly(plan) {
		allowedBlocks = []string{"summary", "ordered_list", "bullet_list", "caveat"}
		if diagramPreferredByEvidence(plan) {
			allowedBlocks = append(allowedBlocks, "diagram")
		}
	}
	lane := AnswerSupportLane{
		Kind:          SupportLaneObservedArtifact,
		Title:         "Observed artifact facts",
		AllowedBlocks: allowedBlocks,
		Guidance: "Use this lane only for facts that came from the attached runtime artifact " +
			"(log / perf trace / external observation). The system populates this lane " +
			"from items the typed evidence projector tagged Origin=log or Origin=perf, " +
			"so every entry here is structurally an observation, not current-code " +
			"mechanism. These facts can explain what was observed (which frame fired, " +
			"which signal triggered) but they do not prove caller-side provenance, " +
			"source-parameter mapping, or exact downstream branch execution unless " +
			"a separately-cited current-code line establishes that mapping.",
	}
	if runtimeObservationOnly(plan) {
		lane.Guidance += " For an external-only runtime artifact with no current-repo intersection, this lane is allowed to carry the principal answer list itself: each item should be an observed frame / event / span from the artifact with citation_ref=-1. Do not substitute current-repo analysis helpers, resolver functions, or nearby implementation details for the artifact facts the user asked about."
		if externalObservationSeedsContainKind(plan.ExternalObservationSeeds, "error_chain") {
			lane.Guidance += " When an entry says `runtime cause chain`, preserve its direction: the upstream/caused-by error happened first and the top-level error is the wrapper or re-raise."
		}
		if len(principalObservationTargets(rm)) > 0 {
			lane.Guidance += " This dispatch has analyzer-resolved principal artifact targets; only frame entries listed in this lane are principal-safe. Other frames from the full log are supporting context unless the user explicitly asked for the full stack."
		}
	}
	for _, seed := range selectSupportExternalObservationSeeds(rm, plan.ExternalObservationSeeds, ExternalObservationPromptSeedLimit) {
		text, location := renderExternalObservationSupportEntry(seed)
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Detail:   externalObservationSupportDetail(seed, text),
			Location: location,
		})
	}
	return lane
}

func externalObservationSeedsContainKind(seeds []ExternalObservationSeed, kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	for _, seed := range seeds {
		if strings.TrimSpace(seed.Kind) == kind {
			return true
		}
	}
	return false
}

func selectSupportExternalObservationSeeds(rm RequestModel, seeds []ExternalObservationSeed, limit int) []ExternalObservationSeed {
	if limit <= 0 || len(seeds) == 0 {
		return nil
	}
	targets := principalObservationTargets(rm)
	if len(targets) == 0 {
		return SelectExternalObservationSeedsForPrompt(seeds, limit)
	}
	out := make([]ExternalObservationSeed, 0, limit)
	seen := make(map[string]bool, limit)
	add := func(seed ExternalObservationSeed) bool {
		if len(out) >= limit {
			return false
		}
		key := externalObservationSeedKey(seed)
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
		out = append(out, seed)
		return true
	}

	for _, seed := range SelectExternalObservationSeedsForPrompt(seeds, limit) {
		if externalObservationSeedIsFrame(seed) {
			continue
		}
		add(seed)
		if len(out) >= 2 {
			break
		}
	}
	addRepresentativeFrames := func(headOnly bool) int {
		added := 0
		for _, seed := range seeds {
			if !externalObservationSeedIsFrame(seed) {
				continue
			}
			if headOnly && !externalObservationSeedIsErrorHeadFrame(seed) {
				continue
			}
			if add(seed) {
				added++
			}
			if len(out) >= limit {
				return added
			}
		}
		return added
	}
	addTargetFrames := func(headOnly bool) int {
		added := 0
		for _, target := range targets {
			for _, seed := range seeds {
				if !externalObservationSeedIsFrame(seed) {
					continue
				}
				if headOnly && !externalObservationSeedIsErrorHeadFrame(seed) {
					continue
				}
				if externalObservationSeedMatchesTarget(seed, target) && add(seed) {
					added++
					break
				}
			}
			if len(out) >= limit {
				return added
			}
		}
		return added
	}
	targetFrameCount := addTargetFrames(true)
	if targetFrameCount > 0 {
		return out
	}
	targetFrameCount += addTargetFrames(false)
	if targetFrameCount > 0 {
		return out
	}
	// If the analyzer resolved principal targets to error types or
	// messages rather than frame functions/files, no frame can match the
	// target surface directly. Keep the typed error facts, but also carry
	// representative frames so downstream answers can explain the runtime
	// artifact without collapsing to message-only prose. This is still
	// typed-field driven: we select frame seeds from LogBundle / PerfBundle,
	// not by scanning raw user text for traceback / panic keywords.
	if addRepresentativeFrames(true) == 0 {
		addRepresentativeFrames(false)
	}
	if len(out) > 0 {
		return out
	}
	return SelectExternalObservationSeedsForPrompt(seeds, limit)
}

func principalObservationTargets(rm RequestModel) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, topic := range rm.SubTopics {
		for _, entity := range topic.Entities {
			add(entity)
		}
	}
	return out
}

func externalObservationSeedMatchesTarget(seed ExternalObservationSeed, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, candidate := range []string{
		seed.Func,
		normalizedSurfaceSymbolTail(seed.Func),
		seed.File,
		fileBaseNoExt(seed.File),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.EqualFold(candidate, target) ||
			strings.EqualFold(normalizedSurfaceSymbolTail(candidate), normalizedSurfaceSymbolTail(target)) {
			return true
		}
	}
	return false
}

func fileBaseNoExt(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	if path == "" {
		return ""
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndex(path, "."); i > 0 {
		path = path[:i]
	}
	return path
}

func renderExternalObservationSupportEntry(seed ExternalObservationSeed) (string, string) {
	raw := strings.TrimSpace(seed.Raw)
	switch strings.TrimSpace(seed.Kind) {
	case "error_chain":
		if raw == "" {
			return "", ""
		}
		return raw, ""
	case "error_type":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("structured runtime error type %q", raw), ""
	case "error_message":
		if raw == "" {
			return "", ""
		}
		return "structured runtime error message: " + raw, ""
	case "signal":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("structured runtime signal %q", raw), ""
	case "log_observation":
		if raw == "" {
			return "", ""
		}
		if subject := strings.TrimSpace(seed.Func); subject != "" {
			return fmt.Sprintf("structured runtime observation about %q: %s", subject, raw), ""
		}
		return fmt.Sprintf("structured runtime observation: %s", raw), ""
	case "perf_jank":
		if subject := strings.TrimSpace(seed.Func); subject != "" {
			return fmt.Sprintf("performance trace observed jank around %q: %s", subject, raw), ""
		}
		return fmt.Sprintf("performance trace observed jank: %s", raw), ""
	case "perf_stall":
		loc := ""
		if file := strings.TrimSpace(strings.ReplaceAll(seed.File, `\`, `/`)); file != "" && seed.Line > 0 {
			loc = fmt.Sprintf("%s:%d", file, seed.Line)
		}
		if subject := strings.TrimSpace(seed.Func); subject != "" {
			return fmt.Sprintf("performance trace observed stall at %q: %s", subject, raw), loc
		}
		return fmt.Sprintf("performance trace observed stall: %s", raw), loc
	case "perf_frame", "perf_startup":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("performance trace observation: %s", raw), ""
	}
	if funcLabel := strings.TrimSpace(seed.Func); funcLabel != "" {
		observedLoc := ""
		if file := strings.TrimSpace(strings.ReplaceAll(seed.File, `\`, `/`)); file != "" && seed.Line > 0 {
			observedLoc = fmt.Sprintf("%s:%d", file, seed.Line)
		}
		frameNoun := externalObservationFrameNoun(seed)
		rolePrefix := "runtime artifact includes " + frameNoun
		switch strings.TrimSpace(seed.Role) {
		case "error_head_frame":
			rolePrefix = "runtime artifact identifies error head " + frameNoun
		case "caller_frame":
			rolePrefix = "runtime artifact includes caller/context " + frameNoun
		}
		if observedLoc != "" {
			return fmt.Sprintf("%s %q at observed %s", rolePrefix, funcLabel, observedLoc), ""
		}
		return fmt.Sprintf("%s %q", rolePrefix, funcLabel), ""
	}
	if raw == "" {
		raw = strings.TrimSpace(seed.Func)
	}
	if raw == "" {
		return "", ""
	}
	location := ""
	switch {
	case strings.TrimSpace(seed.AnchoredFile) != "" && seed.AnchoredLine > 0:
		location = fmt.Sprintf("%s:%d", strings.TrimSpace(seed.AnchoredFile), seed.AnchoredLine)
	case strings.TrimSpace(seed.File) != "" && seed.Line > 0:
		location = fmt.Sprintf("%s:%d", strings.TrimSpace(seed.File), seed.Line)
	}
	if location != "" {
		return fmt.Sprintf("runtime observation %q aligns to %s", raw, location), location
	}
	return fmt.Sprintf("runtime observation %q", raw), ""
}

func externalObservationFrameNoun(seed ExternalObservationSeed) string {
	switch strings.ToLower(strings.TrimSpace(seed.Lang)) {
	case "python", "py":
		return "traceback frame"
	case "arkts", "cangjie", "cj", "java", "kotlin", "swift", "ruby", "javascript", "js", "typescript", "ts", "lua", "rust", "go", "golang", "c", "cpp", "c++", "objc", "objective-c", "cuda":
		return "stack frame"
	default:
		return "stack frame"
	}
}

func externalObservationSupportDetail(seed ExternalObservationSeed, text string) string {
	var parts []string
	for _, raw := range []string{seed.Raw} {
		if detail := answerSupportEntryDetail(raw, text); detail != "" {
			parts = append(parts, detail)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}
