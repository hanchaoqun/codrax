package sourceowner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type Anchor struct {
	Path         string
	Line         int
	OwnerSymbol  string
	AnchorSymbol string
}

var (
	pythonClassRE     = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	pythonFuncRE      = regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goMethodRE        = regexp.MustCompile(`^\s*func\s*\(\s*(?:[A-Za-z_][A-Za-z0-9_]*\s+)?\*?([A-Za-z_][A-Za-z0-9_]*)\s*\)\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goFuncRE          = regexp.MustCompile(`^\s*func\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceClassRE      = regexp.MustCompile(`^\s*(?:export\s+)?(?:abstract\s+|final\s+|sealed\s+|open\s+|public\s+|private\s+|protected\s+|internal\s+|static\s+)*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	braceFunctionRE   = regexp.MustCompile(`^\s*(?:export\s+|public\s+|private\s+|protected\s+|internal\s+|static\s+|async\s+|final\s+|open\s+|override\s+|suspend\s+)*fun\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceFunctionRE2  = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceMethodRE     = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|async\s+|final\s+|override\s+)*([A-Za-z_][A-Za-z0-9_]*)\s*\([^;{}]*\)\s*(?:[:A-Za-z0-9_<>,.? \t]*)?\{?\s*$`)
	braceMethodHeadRE = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|async\s+|final\s+|override\s+|open\s+|sealed\s+|internal\s+|abstract\s+)*([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceReturnFuncRE = regexp.MustCompile(`^\s*(?:template\s*<[^>]*>\s*)?(?:[A-Za-z_][A-Za-z0-9_:<>~*&,\[\]]*[\s*&]+)+([~A-Za-z_][A-Za-z0-9_:~]*|operator\s*[^\s(]+)\s*\(`)
	rubyClassRE       = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_:]*)\b`)
	rubyDefRE         = regexp.MustCompile(`^\s*def\s+(?:self\.)?([A-Za-z_][A-Za-z0-9_?!]*)\b`)
)

// FindEnclosingOwner returns the nearest structural owner around a 1-based
// source line. It consumes only source bytes, path extension, and line number;
// callers decide how much authority to assign to the result.
func FindEnclosingOwner(path string, content []byte, line int) (Anchor, bool) {
	if line <= 0 || len(content) == 0 {
		return Anchor{}, false
	}
	lines := strings.Split(string(content), "\n")
	if line > len(lines) {
		line = len(lines)
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py", ".pyi":
		return findPythonOwner(path, lines, line)
	case ".go":
		return findGoOwner(path, lines, line)
	case ".rb":
		return findRubyOwner(path, lines, line)
	default:
		return findBraceOwner(path, lines, line)
	}
}

func EnrichSourceLocalizationReview(repoRoot string, review types.SourceLocalizationReview, sourceStage string) types.SourceLocalizationReview {
	review = types.NormalizeSourceLocalizationReview(review)
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" || len(review.EvidenceRefs) == 0 {
		return review
	}
	sourceStage = strings.TrimSpace(sourceStage)
	if sourceStage == "" {
		sourceStage = "structural_owner"
	}
	var enriched types.SourceLocalizationReview
	enriched.Source = sourceStage
	for _, ref := range review.EvidenceRefs {
		rel := filepath.ToSlash(strings.TrimSpace(ref.Source))
		if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") || ref.LineStart <= 0 {
			continue
		}
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil || len(content) == 0 {
			continue
		}
		owner, ok := FindEnclosingOwner(rel, content, ref.LineStart)
		if !ok || owner.OwnerSymbol == "" {
			continue
		}
		ref.OwnerSymbol = owner.OwnerSymbol
		if ref.AnchorSymbol == "" {
			ref.AnchorSymbol = owner.AnchorSymbol
		}
		if ref.Subject == "" {
			ref.Subject = rel + ":" + owner.OwnerSymbol
		}
		enriched.SourcePaths = append(enriched.SourcePaths, rel)
		refCopy := ref
		enriched.EvidenceRefs = append(enriched.EvidenceRefs, refCopy)
		enriched.Anchors = append(enriched.Anchors, types.SourceLocalizationAnchor{
			Path:         rel,
			Role:         types.ClassifySourcePathRole(rel),
			SourceStage:  sourceStage,
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			EvidenceRef:  &refCopy,
			Subject:      refCopy.Subject,
			OwnerSymbol:  owner.OwnerSymbol,
			AnchorSymbol: owner.AnchorSymbol,
			ReasonCode:   "line_structural_owner",
		})
	}
	if len(enriched.Anchors) == 0 {
		return review
	}
	enriched.Status = types.SourceLocalizationSupported
	enriched.ReasonCodes = append(enriched.ReasonCodes, "line_structural_owner_observed")
	return *types.MergeSourceLocalizationReviews(&review, &enriched)
}

func findPythonOwner(path string, lines []string, line int) (Anchor, bool) {
	type frame struct {
		indent int
		name   string
		kind   string
		line   int
	}
	var stack []frame
	pendingHeaderBalance := 0
	pendingHeader := false
	for i := 0; i < line && i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingWhitespaceWidth(raw)
		if pendingHeader {
			pendingHeaderBalance += pythonBracketDelta(raw)
			if pendingHeaderBalance <= 0 && strings.HasSuffix(trimmed, ":") {
				pendingHeader = false
				pendingHeaderBalance = 0
			}
			continue
		}
		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		if m := pythonClassRE.FindStringSubmatch(raw); len(m) == 2 {
			stack = append(stack, frame{indent: indent, name: m[1], kind: "class", line: i + 1})
			if delta := pythonBracketDelta(raw); delta > 0 || !strings.HasSuffix(trimmed, ":") {
				pendingHeader = true
				pendingHeaderBalance = delta
			}
			continue
		}
		if m := pythonFuncRE.FindStringSubmatch(raw); len(m) == 2 {
			stack = append(stack, frame{indent: indent, name: m[1], kind: "function", line: i + 1})
			if delta := pythonBracketDelta(raw); delta > 0 || !strings.HasSuffix(trimmed, ":") {
				pendingHeader = true
				pendingHeaderBalance = delta
			}
		}
	}
	if len(stack) == 0 {
		return Anchor{}, false
	}
	parts := make([]string, 0, len(stack))
	var anchor string
	for _, f := range stack {
		parts = append(parts, f.name)
		anchor = f.name
	}
	return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: strings.Join(parts, "."), AnchorSymbol: anchor}, true
}

func pythonBracketDelta(s string) int {
	delta := 0
	for _, r := range s {
		switch r {
		case '(', '[', '{':
			delta++
		case ')', ']', '}':
			delta--
		}
	}
	return delta
}

func findGoOwner(path string, lines []string, line int) (Anchor, bool) {
	for i := minInt(line, len(lines)) - 1; i >= 0; i-- {
		raw := lines[i]
		if m := goMethodRE.FindStringSubmatch(raw); len(m) == 3 {
			return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: m[1] + "." + m[2], AnchorSymbol: m[2]}, true
		}
		if m := goFuncRE.FindStringSubmatch(raw); len(m) == 2 {
			return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: m[1], AnchorSymbol: m[1]}, true
		}
	}
	return Anchor{}, false
}

func findRubyOwner(path string, lines []string, line int) (Anchor, bool) {
	var className string
	var defName string
	for i := 0; i < line && i < len(lines); i++ {
		raw := lines[i]
		if m := rubyClassRE.FindStringSubmatch(raw); len(m) == 2 {
			className = strings.ReplaceAll(m[1], "::", ".")
			continue
		}
		if m := rubyDefRE.FindStringSubmatch(raw); len(m) == 2 {
			defName = m[1]
		}
	}
	switch {
	case className != "" && defName != "":
		return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: className + "." + defName, AnchorSymbol: defName}, true
	case defName != "":
		return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: defName, AnchorSymbol: defName}, true
	case className != "":
		return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: className, AnchorSymbol: className}, true
	default:
		return Anchor{}, false
	}
}

func findBraceOwner(path string, lines []string, line int) (Anchor, bool) {
	var className string
	classDepth := -1
	var owner string
	var anchor string
	depth := 0
	for i := 0; i < line && i < len(lines); i++ {
		rawLine := lines[i]
		raw := strings.TrimSpace(rawLine)
		depthBefore := depth
		updateDepth := func() {
			depth += braceDelta(rawLine)
			if depth < 0 {
				depth = 0
			}
		}
		if raw == "" || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "*") {
			updateDepth()
			continue
		}
		if m := braceClassRE.FindStringSubmatch(raw); len(m) == 2 {
			className = m[1]
			classDepth = depthBefore + 1
			owner = className
			anchor = className
			updateDepth()
			continue
		}
		if m := braceFunctionRE.FindStringSubmatch(raw); len(m) == 2 {
			owner, anchor = qualifyOwner(className, m[1]), m[1]
			updateDepth()
			continue
		}
		if m := braceFunctionRE2.FindStringSubmatch(raw); len(m) == 2 {
			owner, anchor = qualifyOwner(className, m[1]), m[1]
			updateDepth()
			continue
		}
		if strings.Contains(raw, "(") && !strings.HasPrefix(raw, "if ") && !strings.HasPrefix(raw, "for ") &&
			!strings.HasPrefix(raw, "while ") && !strings.HasPrefix(raw, "switch ") {
			if m := braceMethodRE.FindStringSubmatch(raw); len(m) == 2 {
				owner, anchor = qualifyOwner(className, m[1]), m[1]
				updateDepth()
				continue
			}
			if member, ok := braceReturnFunctionName(raw); ok {
				owner, anchor = qualifyBraceOwner(className, member)
				updateDepth()
				continue
			}
			if className != "" && depthBefore == classDepth {
				if m := braceMethodHeadRE.FindStringSubmatch(raw); len(m) == 2 {
					owner, anchor = qualifyOwner(className, m[1]), m[1]
				}
			}
		}
		updateDepth()
	}
	if owner == "" {
		return Anchor{}, false
	}
	return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: owner, AnchorSymbol: anchor}, true
}

func braceReturnFunctionName(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || braceLineIsNonOwnerCall(raw) {
		return "", false
	}
	m := braceReturnFuncRE.FindStringSubmatch(raw)
	if len(m) != 2 {
		return "", false
	}
	name := strings.TrimSpace(m[1])
	if name == "" {
		return "", false
	}
	name = strings.Join(strings.Fields(name), " ")
	return name, true
}

func braceLineIsNonOwnerCall(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{
		"if ", "for ", "while ", "switch ", "catch ", "return ", "throw ",
		"case ", "using ", "typedef ", "static_assert", "sizeof", "new ",
		"delete ", "#",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	if strings.HasPrefix(lower, "else if ") {
		return true
	}
	open := strings.Index(trimmed, "(")
	if open <= 0 {
		return true
	}
	head := trimmed[:open]
	if strings.ContainsAny(head, "=.;") {
		return true
	}
	if strings.HasSuffix(trimmed, ";") && !strings.Contains(trimmed, "{") {
		return true
	}
	return false
}

func qualifyBraceOwner(className, member string) (string, string) {
	member = strings.TrimSpace(member)
	member = strings.ReplaceAll(member, " :: ", "::")
	member = strings.ReplaceAll(member, " ::", "::")
	member = strings.ReplaceAll(member, ":: ", "::")
	anchor := member
	if idx := strings.LastIndex(anchor, "::"); idx >= 0 && idx+2 < len(anchor) {
		anchor = anchor[idx+2:]
	}
	if strings.HasPrefix(member, "operator ") {
		anchor = member
	}
	if strings.Contains(member, "::") {
		return member, anchor
	}
	return qualifyOwner(className, member), anchor
}

func qualifyOwner(className, member string) string {
	if className == "" {
		return member
	}
	return className + "." + member
}

func braceDelta(s string) int {
	delta := 0
	for _, r := range s {
		switch r {
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

func leadingWhitespaceWidth(s string) int {
	width := 0
	for _, r := range s {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
