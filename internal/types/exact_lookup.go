package types

import (
	"path/filepath"
	"sort"
	"strings"
)

// BuildExactResolutionContract derives the AnswerContract-level
// contract for exact-target questions. It is intentionally
// conservative: broad enumeration questions return nil so downstream
// target-mention / absence constraints do not over-trigger.
func BuildExactResolutionContract(rm RequestModel) *ExactResolutionContract {
	targets := ExactResolutionTargets(rm)
	if len(targets) == 0 {
		return nil
	}
	label := exactResolutionSubjectLabel(rm)
	if label == "" {
		label = "target"
	}
	policy := exactResolutionContextPolicyForRM(rm)
	contract := &ExactResolutionContract{
		TargetKind:           rm.AnswerSubject.Kind,
		TargetLabel:          label,
		Targets:              targets,
		AllowAbsence:         true,
		RequireTargetMention: true,
		AliasRequiresProof:   true,
		RelatedContextPolicy: policy,
	}
	if hint := exactResolutionScopeHintForPolicy(policy); hint != "" {
		contract.RelatedContextScopeHint = hint
	}
	return contract
}

// ExactResolutionTargets returns the user-named primary targets for
// exact-resolution questions (config keys, file paths, single-symbol
// lookups). It is intentionally conservative: broad enumeration
// questions should return nil so downstream absence contracts do not
// over-trigger.
func ExactResolutionTargets(rm RequestModel) []string {
	if !exactResolutionEnabled(rm) {
		return nil
	}
	candidates := rm.AnalyzerHints.MentionedEntities
	if len(candidates) == 0 {
		candidates = MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.PrimaryEntities)
	}
	if len(candidates) == 0 {
		candidates = MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.Entities)
	}
	if len(candidates) == 0 {
		candidates = rm.AnalyzerHints.PrimaryEntities
		if len(candidates) == 0 {
			candidates = rm.AnalyzerHints.Entities
		}
		if len(candidates) == 0 {
			return nil
		}
	}
	kind := exactResolutionFindingKindForRM(rm)
	label := exactResolutionSubjectLabel(rm)
	seen := make(map[string]bool)
	var out []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(strings.Trim(candidate, "`\"' "))
		if candidate == "" {
			continue
		}
		switch label {
		case "config key":
			if !looksLikeExactConfigToken(candidate) {
				continue
			}
		case "file path":
			if !looksLikeExactPathToken(candidate) {
				continue
			}
		}
		key := normalizeExactResolutionToken(kind, candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func MentionedEntitiesFromRawRequest(raw string, candidates []string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(candidates) == 0 {
		return nil
	}
	lowerRaw := strings.ToLower(strings.ReplaceAll(raw, `\`, `/`))
	normRawSymbol := normalizeExactResolutionToken("symbol", raw)
	normRawPath := normalizeExactResolutionToken("path", raw)
	if lowerRaw == "" && normRawSymbol == "" && normRawPath == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(strings.Trim(candidate, "`\"' "))
		if candidate == "" {
			continue
		}
		key := normalizeExactResolutionToken("symbol", candidate)
		if looksLikeExactPathToken(candidate) {
			key = normalizeExactResolutionToken("path", candidate)
		}
		if key == "" || seen[key] {
			continue
		}
		explicit := strings.Contains(lowerRaw, strings.ToLower(strings.ReplaceAll(candidate, `\`, `/`))) ||
			strings.Contains(normRawSymbol, normalizeExactResolutionToken("symbol", candidate)) ||
			strings.Contains(normRawPath, normalizeExactResolutionToken("path", candidate))
		if !explicit {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func DerivedEntitiesFromMentioned(all, mentioned []string) []string {
	if len(all) == 0 {
		return nil
	}
	seenMentioned := make(map[string]bool, len(mentioned))
	for _, item := range mentioned {
		key := normalizeExactResolutionToken("symbol", item)
		if looksLikeExactPathToken(item) {
			key = normalizeExactResolutionToken("path", item)
		}
		if key != "" {
			seenMentioned[key] = true
		}
	}
	seenOut := make(map[string]bool)
	var out []string
	for _, item := range all {
		key := normalizeExactResolutionToken("symbol", item)
		if looksLikeExactPathToken(item) {
			key = normalizeExactResolutionToken("path", item)
		}
		if key == "" || seenMentioned[key] || seenOut[key] {
			continue
		}
		seenOut[key] = true
		out = append(out, item)
	}
	return out
}

// ExactResolutionPendingTargets returns the subset of contract targets
// whose primary token remains unresolved in prior stage reports.
func ExactResolutionPendingTargets(c *ExactResolutionContract, finds []UnverifiedFinding) []string {
	if c == nil || len(c.Targets) == 0 || len(finds) == 0 {
		return nil
	}
	kind := exactResolutionFindingKind(c)
	seen := make(map[string]bool)
	for _, f := range finds {
		if !exactResolutionFindingKindMatches(kind, f.Kind) {
			continue
		}
		key := normalizeExactResolutionToken(kind, f.Token)
		if key != "" {
			seen[key] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	var out []string
	for _, target := range c.Targets {
		key := normalizeExactResolutionToken(kind, target)
		if key != "" && seen[key] {
			out = append(out, target)
		}
	}
	return out
}

// ExactResolutionTextMentionsTarget reports whether text explicitly
// names the exact target under the same normalization the pending-
// target matcher uses.
func ExactResolutionTextMentionsTarget(c *ExactResolutionContract, text, target string) bool {
	kind := exactResolutionFindingKind(c)
	normTarget := normalizeExactResolutionToken(kind, target)
	if normTarget == "" {
		return false
	}
	return strings.Contains(normalizeExactResolutionToken(kind, text), normTarget)
}

// ExactResolutionTerms returns a deduplicated, sorted token bag built
// from the contract's targets. Used to surface grounded same-family
// context without hardcoding repo-specific names.
func ExactResolutionTerms(c *ExactResolutionContract) []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, target := range c.Targets {
		for _, term := range splitExactResolutionTerms(target) {
			if len(term) < 3 || seen[term] {
				continue
			}
			seen[term] = true
			out = append(out, term)
		}
	}
	sort.Strings(out)
	return out
}

// ExactResolutionScopeTerms returns a small token bag that represents
// the contract's preferred local scope when nearby context is allowed.
func ExactResolutionScopeTerms(c *ExactResolutionContract) []string {
	if c == nil || len(c.Targets) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	add := func(term string) {
		term = strings.TrimSpace(strings.ToLower(term))
		if len(term) < 3 || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	switch c.RelatedContextPolicy {
	case ExactContextSameFamilyGrounded:
		for _, target := range c.Targets {
			for _, term := range splitExactResolutionTerms(target) {
				add(term)
				break
			}
		}
	case ExactContextSameDirectoryGrounded:
		for _, target := range c.Targets {
			dir := filepath.Dir(strings.ReplaceAll(strings.TrimSpace(target), `\`, `/`))
			if dir == "." || dir == "/" || dir == "" {
				continue
			}
			for _, seg := range strings.Split(dir, "/") {
				add(seg)
			}
		}
	}
	sort.Strings(out)
	return out
}

func exactResolutionSubjectLabel(rm RequestModel) string {
	switch rm.AnswerSubject.Kind {
	case SubjectConfigKey:
		return "config key"
	case SubjectFilePath:
		return "file path"
	case SubjectFunctionName, SubjectTypeName, SubjectStructField, SubjectInterface, SubjectEnumValue:
		return "symbol"
	case SubjectHandlerRoute:
		return "route"
	case SubjectStringLiteral:
		return "literal"
	}
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "config_mapping") || rm.Scenario == ScenarioConfigTrace {
		return "config key"
	}
	return "target"
}

func exactResolutionContextPolicyForRM(rm RequestModel) ExactResolutionContextPolicy {
	switch exactResolutionSubjectLabel(rm) {
	case "config key":
		return ExactContextSameFamilyGrounded
	case "file path":
		return ExactContextSameDirectoryGrounded
	default:
		return ExactContextGroundedOnly
	}
}

func exactResolutionScopeHintForPolicy(policy ExactResolutionContextPolicy) string {
	switch policy {
	case ExactContextSameFamilyGrounded:
		return "same namespace / prefix family"
	case ExactContextSameDirectoryGrounded:
		return "same directory / module subtree"
	case ExactContextGroundedOnly:
		return "grounded nearby context only"
	default:
		return ""
	}
}

func exactResolutionEnabled(rm RequestModel) bool {
	switch rm.AnswerSubject.Kind {
	case SubjectConfigKey, SubjectFilePath:
		return true
	case SubjectFunctionName, SubjectTypeName, SubjectStructField, SubjectInterface, SubjectEnumValue, SubjectHandlerRoute, SubjectStringLiteral:
		primary := rm.AnalyzerHints.PrimaryEntities
		if len(primary) == 0 {
			primary = rm.AnalyzerHints.Entities
		}
		return rm.Predicates.IsScalarAnswer || len(primary) <= 1
	}
	return strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "config_mapping") || rm.Scenario == ScenarioConfigTrace
}

func exactResolutionFindingKindForRM(rm RequestModel) string {
	if exactResolutionSubjectLabel(rm) == "file path" {
		return "path"
	}
	return "symbol"
}

func exactResolutionFindingKind(c *ExactResolutionContract) string {
	if c == nil {
		return "symbol"
	}
	if c.TargetKind == SubjectFilePath || strings.EqualFold(strings.TrimSpace(c.TargetLabel), "file path") {
		return "path"
	}
	return "symbol"
}

func exactResolutionFindingKindMatches(expected, got string) bool {
	got = strings.TrimSpace(strings.ToLower(got))
	if got == "" {
		got = "symbol"
	}
	return got == expected
}

func looksLikeExactConfigToken(s string) bool {
	s = strings.TrimSpace(s)
	if looksLikeFileLikeToken(s) {
		return false
	}
	return len(s) >= 3 && (strings.ContainsAny(s, "_.-") || hasExactLookupCamelBoundary(s))
}

func looksLikeExactPathToken(s string) bool {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return false
	}
	return strings.Contains(s, "/") || (filepath.Ext(s) != "" && !strings.Contains(s, " "))
}

func looksLikeFileLikeToken(s string) bool {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" || strings.Contains(s, "/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(s))
	switch ext {
	case ".yaml", ".yml", ".json", ".toml", ".ini", ".conf", ".cfg",
		".xml", ".properties", ".env", ".txt", ".md", ".go", ".py",
		".js", ".ts", ".java", ".kt", ".rs", ".rb", ".swift", ".lua",
		".proto", ".c", ".cc", ".cpp", ".h", ".hpp":
		return true
	}
	return false
}

func hasExactLookupCamelBoundary(s string) bool {
	prevLower := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' && prevLower {
			return true
		}
		prevLower = r >= 'a' && r <= 'z'
	}
	return false
}

func normalizeExactResolutionToken(kind, s string) string {
	s = strings.TrimSpace(strings.Trim(s, "`\"' "))
	switch kind {
	case "path":
		s = strings.ReplaceAll(s, `\`, `/`)
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.TrimPrefix(s, "./")
		return s
	default:
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
}

func splitExactResolutionTerms(s string) []string {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return nil
	}
	var b strings.Builder
	var parts []string
	flush := func() {
		if b.Len() == 0 {
			return
		}
		parts = append(parts, strings.ToLower(b.String()))
		b.Reset()
	}
	prevLower := false
	for _, r := range s {
		switch {
		case (r >= 'A' && r <= 'Z') && prevLower:
			flush()
			b.WriteRune(r + ('a' - 'A'))
			prevLower = false
		case (r >= 'A' && r <= 'Z'):
			b.WriteRune(r + ('a' - 'A'))
			prevLower = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevLower = r >= 'a' && r <= 'z'
		default:
			flush()
			prevLower = false
		}
	}
	flush()
	return parts
}
