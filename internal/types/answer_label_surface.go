package types

import (
	"path/filepath"
	"strconv"
	"strings"
)

// AnswerSourceLocationSurface is the structured form of an answer-visible
// file/path label such as "internal/agent/analyzer.go:1903". It is a display
// surface, not a declaration symbol, so validators should align it to the
// cited file:line instead of routing it through symbol-existence gates.
type AnswerSourceLocationSurface struct {
	File      string
	LineStart int
	LineEnd   int
}

// ParseAnswerSourceLocationSurface parses a whole item label when the label
// itself is a source/config location. The grammar is intentionally structural:
// known source/config path suffix + numeric line (or line range). It does not
// depend on language-specific keywords or symbol spellings.
func ParseAnswerSourceLocationSurface(label string) (AnswerSourceLocationSurface, bool) {
	raw := strings.TrimSpace(label)
	if raw == "" || strings.ContainsAny(raw, "\n\r") {
		return AnswerSourceLocationSurface{}, false
	}
	raw = strings.Trim(raw, "`'\" ")
	for _, sep := range []string{" @ ", " | ", "\t"} {
		if strings.Contains(raw, sep) {
			return AnswerSourceLocationSurface{}, false
		}
	}
	raw = strings.ReplaceAll(raw, `\`, `/`)
	colon := strings.LastIndex(raw, ":")
	if colon <= 0 || colon == len(raw)-1 {
		return AnswerSourceLocationSurface{}, false
	}
	file := strings.TrimSpace(raw[:colon])
	linePart := strings.TrimSpace(raw[colon+1:])
	if file == "" || linePart == "" {
		return AnswerSourceLocationSurface{}, false
	}
	lineStart, lineEnd, ok := parseAnswerLineSurface(linePart)
	if !ok {
		return AnswerSourceLocationSurface{}, false
	}
	if !IsCodeOrConfigPathExtension(filepath.Ext(file)) {
		return AnswerSourceLocationSurface{}, false
	}
	return AnswerSourceLocationSurface{
		File:      normalizeAnswerLocationFile(file),
		LineStart: lineStart,
		LineEnd:   lineEnd,
	}, true
}

// AnswerSourceLocationSurfaceMatchesCitation reports whether a parsed
// source-location display label is backed by the exact citation it names.
// A shorter label path may match the suffix of a repo-relative citation path,
// e.g. "analyzer.go:10" against "internal/agent/analyzer.go:10".
func AnswerSourceLocationSurfaceMatchesCitation(surface AnswerSourceLocationSurface, cit Citation) bool {
	file := normalizeAnswerLocationFile(cit.File)
	if surface.File == "" || file == "" || cit.Line <= 0 {
		return false
	}
	if !answerLocationFileMatches(surface.File, file) {
		return false
	}
	end := surface.LineEnd
	if end <= 0 {
		end = surface.LineStart
	}
	return cit.Line >= surface.LineStart && cit.Line <= end
}

// AnswerSourceLocationLabelMatchesCitation combines parsing and alignment for
// answer validators that only have the original item label.
func AnswerSourceLocationLabelMatchesCitation(label string, cit Citation) bool {
	surface, ok := ParseAnswerSourceLocationSurface(label)
	return ok && AnswerSourceLocationSurfaceMatchesCitation(surface, cit)
}

// ParseAnswerFilePathSurface parses a whole item label when the label itself
// is a repo file path without a line suffix. This supports answer surfaces
// where the principal member is the file (for example change-impact
// requested_output=files), while keeping path detection structural through the
// known code/config extension registry.
func ParseAnswerFilePathSurface(label string) (string, bool) {
	raw := strings.TrimSpace(label)
	if raw == "" || strings.ContainsAny(raw, "\n\r") {
		return "", false
	}
	raw = strings.Trim(raw, "`'\" ")
	raw = strings.ReplaceAll(raw, `\`, `/`)
	if strings.Contains(raw, ":") {
		return "", false
	}
	if !IsCodeOrConfigPathExtension(filepath.Ext(raw)) {
		return "", false
	}
	return normalizeAnswerLocationFile(raw), true
}

// AnswerFilePathLabelMatchesCitation reports whether a file-path principal
// label is backed by a citation in that same file. Unlike
// AnswerSourceLocationLabelMatchesCitation it does not require the label to
// carry a line number, because the file is the answer member and the cited
// line is supporting evidence for that file member.
func AnswerFilePathLabelMatchesCitation(label string, cit Citation) bool {
	fileLabel, ok := ParseAnswerFilePathSurface(label)
	if !ok {
		return false
	}
	citationFile := normalizeAnswerLocationFile(cit.File)
	if citationFile == "" {
		return false
	}
	return answerLocationFileMatches(fileLabel, citationFile)
}

// AnswerLocationLabelMatchesCitation accepts either a precise file:line label
// or a file-only label aligned to a citation in that file.
func AnswerLocationLabelMatchesCitation(label string, cit Citation) bool {
	return AnswerSourceLocationLabelMatchesCitation(label, cit) ||
		AnswerFilePathLabelMatchesCitation(label, cit)
}

func parseAnswerLineSurface(raw string) (int, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	startPart := raw
	endPart := ""
	if dash := strings.Index(raw, "-"); dash >= 0 {
		startPart = strings.TrimSpace(raw[:dash])
		endPart = strings.TrimSpace(raw[dash+1:])
	}
	start, err := strconv.Atoi(startPart)
	if err != nil || start <= 0 {
		return 0, 0, false
	}
	end := start
	if endPart != "" {
		parsedEnd, err := strconv.Atoi(endPart)
		if err != nil || parsedEnd < start {
			return 0, 0, false
		}
		end = parsedEnd
	}
	return start, end, true
}

func normalizeAnswerLocationFile(file string) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	for strings.HasPrefix(file, "./") {
		file = strings.TrimPrefix(file, "./")
	}
	return strings.ToLower(file)
}

func answerLocationFileMatches(labelFile, citationFile string) bool {
	if labelFile == citationFile {
		return true
	}
	return strings.HasSuffix(citationFile, "/"+labelFile)
}
