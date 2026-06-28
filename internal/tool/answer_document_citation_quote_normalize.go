package tool

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func normalizeCurrentSourceCitationQuotes(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	repoRoot := strings.TrimSpace(ctx.RepoRoot)
	if repoRoot == "" && ctx.Mutable != nil {
		repoRoot = strings.TrimSpace(ctx.Mutable.RepoRoot())
	}
	if repoRoot == "" {
		return 0
	}
	fixed := 0
	for i := range doc.Citations {
		cit := &doc.Citations[i]
		if cit.Line <= 0 || cit.LineEnd > cit.Line || strings.TrimSpace(cit.NegativePattern) != "" {
			continue
		}
		line, ok := currentSourceCitationLine(repoRoot, cit.File, cit.Line)
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

func currentSourceCitationLine(repoRoot, file string, lineNo int) (string, bool) {
	if lineNo <= 0 {
		return "", false
	}
	lines, ok := currentSourceCitationLines(repoRoot, file)
	if !ok || lineNo > len(lines) {
		return "", false
	}
	return lines[lineNo-1], true
}

func currentSourceCitationLines(repoRoot, file string) ([]string, bool) {
	path, ok := currentSourceCitationPath(repoRoot, file)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), true
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

func logCurrentSourceCitationQuoteRepairs(toolName string, fixed int) {
	if fixed <= 0 {
		return
	}
	logging.Warning("[%s] repaired %d citation quote(s) from current source file:line", toolName, fixed)
}
