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
	pythonClassRE    = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	pythonFuncRE     = regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goMethodRE       = regexp.MustCompile(`^\s*func\s*\(\s*(?:[A-Za-z_][A-Za-z0-9_]*\s+)?\*?([A-Za-z_][A-Za-z0-9_]*)\s*\)\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goFuncRE         = regexp.MustCompile(`^\s*func\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceClassRE     = regexp.MustCompile(`^\s*(?:export\s+)?(?:abstract\s+|final\s+|sealed\s+|open\s+|public\s+|private\s+|protected\s+|internal\s+|static\s+)*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	braceFunctionRE  = regexp.MustCompile(`^\s*(?:export\s+|public\s+|private\s+|protected\s+|internal\s+|static\s+|async\s+|final\s+|open\s+|override\s+|suspend\s+)*fun\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceFunctionRE2 = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	braceMethodRE    = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|async\s+|final\s+|override\s+)*([A-Za-z_][A-Za-z0-9_]*)\s*\([^;{}]*\)\s*(?:[:A-Za-z0-9_<>,.? \t]*)?\{?\s*$`)
	rubyClassRE      = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_:]*)\b`)
	rubyDefRE        = regexp.MustCompile(`^\s*def\s+(?:self\.)?([A-Za-z_][A-Za-z0-9_?!]*)\b`)
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
	for i := 0; i < line && i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingWhitespaceWidth(raw)
		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		if m := pythonClassRE.FindStringSubmatch(raw); len(m) == 2 {
			stack = append(stack, frame{indent: indent, name: m[1], kind: "class", line: i + 1})
			continue
		}
		if m := pythonFuncRE.FindStringSubmatch(raw); len(m) == 2 {
			stack = append(stack, frame{indent: indent, name: m[1], kind: "function", line: i + 1})
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
	var owner string
	var anchor string
	for i := 0; i < line && i < len(lines); i++ {
		raw := strings.TrimSpace(lines[i])
		if raw == "" || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "*") {
			continue
		}
		if m := braceClassRE.FindStringSubmatch(raw); len(m) == 2 {
			className = m[1]
			owner = className
			anchor = className
			continue
		}
		if m := braceFunctionRE.FindStringSubmatch(raw); len(m) == 2 {
			owner, anchor = qualifyOwner(className, m[1]), m[1]
			continue
		}
		if m := braceFunctionRE2.FindStringSubmatch(raw); len(m) == 2 {
			owner, anchor = qualifyOwner(className, m[1]), m[1]
			continue
		}
		if strings.Contains(raw, "(") && !strings.HasPrefix(raw, "if ") && !strings.HasPrefix(raw, "for ") &&
			!strings.HasPrefix(raw, "while ") && !strings.HasPrefix(raw, "switch ") {
			if m := braceMethodRE.FindStringSubmatch(raw); len(m) == 2 {
				owner, anchor = qualifyOwner(className, m[1]), m[1]
			}
		}
	}
	if owner == "" {
		return Anchor{}, false
	}
	return Anchor{Path: filepath.ToSlash(path), Line: line, OwnerSymbol: owner, AnchorSymbol: anchor}, true
}

func qualifyOwner(className, member string) string {
	if className == "" {
		return member
	}
	return className + "." + member
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
