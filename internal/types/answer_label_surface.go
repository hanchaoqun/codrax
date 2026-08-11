package types

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	for _, sep := range []string{" @ ", " | ", "\t", ": "} {
		if strings.Contains(raw, sep) {
			return AnswerSourceLocationSurface{}, false
		}
	}
	if answerSourceLocationLooksLikeCompactSupportRef(raw) {
		return AnswerSourceLocationSurface{}, false
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
	if !HasCodeOrConfigPathSuffix(file) {
		return AnswerSourceLocationSurface{}, false
	}
	return AnswerSourceLocationSurface{
		File:      displayAnswerLocationFile(file),
		LineStart: lineStart,
		LineEnd:   lineEnd,
	}, true
}

func answerSourceLocationLooksLikeCompactSupportRef(raw string) bool {
	idx := strings.LastIndex(raw, "@")
	if idx <= 0 || idx == len(raw)-1 {
		return false
	}
	label := strings.TrimSpace(raw[:idx])
	location := strings.TrimSpace(raw[idx+1:])
	if label == "" || location == "" {
		return false
	}
	if strings.ContainsAny(label, `/\`) || strings.Contains(label, ":") {
		return false
	}
	location = strings.ReplaceAll(location, `\`, `/`)
	colon := strings.LastIndex(location, ":")
	if colon <= 0 || colon == len(location)-1 {
		return false
	}
	file := strings.TrimSpace(location[:colon])
	linePart := strings.TrimSpace(location[colon+1:])
	if file == "" || linePart == "" {
		return false
	}
	if !HasCodeOrConfigPathSuffix(file) {
		return false
	}
	_, _, ok := parseAnswerLineSurface(linePart)
	return ok
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

// ParseAnswerSupportRefMemberLocation parses composite member surfaces where a
// user-visible label is paired with a citable source/config location, such as
// "Foo @ src/foo.go:10", "Foo | src/foo.go:10",
// "Foo — src/foo.go:10, package=demo.foo", or "Foo (src/foo.go:10)".
// This is display grammar for model-authored typed handoff data; it never
// infers a fact from prose. Trailing comma/semicolon content is accepted only
// when it parses as a typed source-inventory attribute.
func ParseAnswerSupportRefMemberLocation(raw string) (label string, location AnswerSourceLocationSurface, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", AnswerSourceLocationSurface{}, false
	}
	for _, sep := range []string{" @ ", "\t", " | ", ": ", " — ", " – "} {
		idx := strings.Index(raw, sep)
		if idx <= 0 {
			continue
		}
		if surface, parsed := parseAnswerSupportLocationSurface(strings.TrimSpace(raw[idx+len(sep):])); parsed {
			return strings.TrimSpace(raw[:idx]), surface, true
		}
	}
	if strings.HasSuffix(raw, ")") {
		if idx := strings.LastIndex(raw, "("); idx > 0 {
			label := strings.TrimSpace(raw[:idx])
			inner := strings.TrimSpace(raw[idx+1 : len(raw)-1])
			if strings.HasPrefix(strings.ToLower(inner), "via ") {
				if _, surface, parsed := ParseAnswerSupportRefMemberLocation(strings.TrimSpace(inner[4:])); parsed {
					return label, surface, true
				}
			}
			if surface, parsed := ParseAnswerSourceLocationSurface(inner); parsed {
				return strings.TrimSpace(raw[:idx]), surface, true
			}
			if locationPart, parsed := answerDecoratedLocationAttributeLocationPart(inner); parsed {
				if surface, parsed := ParseAnswerSourceLocationSurface(locationPart); parsed {
					return strings.TrimSpace(raw[:idx]), surface, true
				}
			}
		}
	}
	if idx := strings.LastIndex(raw, "@"); idx > 0 && idx < len(raw)-1 {
		label := strings.TrimSpace(raw[:idx])
		location := strings.TrimSpace(raw[idx+1:])
		if label != "" {
			if surface, parsed := parseAnswerSupportLocationSurface(location); parsed {
				return label, surface, true
			}
		}
	}
	if surface, parsed := ParseAnswerSourceLocationSurface(raw); parsed {
		return "", surface, true
	}
	return "", AnswerSourceLocationSurface{}, false
}

func answerDecoratedLocationAttributeLocationPart(inner string) (string, bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return "", false
	}
	for _, sep := range []string{",", "，", ";", "；"} {
		if idx := strings.Index(inner, sep); idx > 0 {
			location := strings.TrimSpace(inner[:idx])
			attribute := strings.TrimSpace(inner[idx+len(sep):])
			if location != "" && attribute != "" {
				if _, _, ok := aggregateMemberAttributeQualifierFromDecorator(attribute); !ok {
					continue
				}
				return location, true
			}
		}
	}
	return "", false
}

func parseAnswerSupportLocationSurface(raw string) (AnswerSourceLocationSurface, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AnswerSourceLocationSurface{}, false
	}
	if surface, parsed := ParseAnswerSourceLocationSurface(raw); parsed {
		return surface, true
	}
	if locationPart, parsed := answerDecoratedLocationAttributeLocationPart(raw); parsed {
		if surface, parsed := ParseAnswerSourceLocationSurface(locationPart); parsed {
			return surface, true
		}
	}
	for _, idx := range []int{
		strings.Index(raw, " ("),
		strings.Index(raw, " ["),
		strings.Index(raw, " — "),
		strings.Index(raw, " – "),
	} {
		if idx <= 0 {
			continue
		}
		if surface, parsed := ParseAnswerSourceLocationSurface(strings.TrimSpace(raw[:idx])); parsed {
			return surface, true
		}
	}
	return AnswerSourceLocationSurface{}, false
}

// AnswerSupportRefLabelIsGeneric reports whether the label part of
// "label @ file:line" is the schema placeholder rather than the answer member
// itself. The support ref still has to be validated against the member and the
// grounded location by callers; this only normalizes the carrier grammar.
func AnswerSupportRefLabelIsGeneric(label string) bool {
	label = strings.ToLower(strings.Trim(strings.TrimSpace(label), "`'\" "))
	switch label {
	case "member", "members", "item", "principal_member":
		return true
	default:
		return false
	}
}

// AnswerCodeSurfaceAppearsInText checks whether a code/config surface appears
// as a complete identity token in text. It is intentionally structural and
// language-neutral: callers provide the already model-authored surface and
// already-grounded text, and this helper only prevents substring accidents such
// as "go" matching "goroutine".
func AnswerCodeSurfaceAppearsInText(text string, surface string) bool {
	text = strings.TrimSpace(text)
	surface = strings.TrimSpace(surface)
	if text == "" || surface == "" || !IsCodeIdentitySurface(surface) {
		return false
	}
	searchFrom := 0
	for searchFrom <= len(text) {
		idx := strings.Index(text[searchFrom:], surface)
		if idx < 0 {
			return false
		}
		start := searchFrom + idx
		end := start + len(surface)
		if answerCodeSurfaceBoundaryOK(text, start, end) {
			return true
		}
		if end <= searchFrom {
			return false
		}
		searchFrom = end
	}
	return false
}

// AnswerCodeIdentitySurfacesCompatible compares two already-typed code
// identity surfaces without substring guessing. Qualified spellings may use
// the language-native separators `.`, `::`, `#`, `->`, or `/`; a short symbol
// may stand for a qualified surface's final segment, but two qualified
// surfaces must agree on their complete segment sequence. This keeps
// `Foo::run` equivalent to `Foo.run` while rejecting `Other.run` and
// prefix siblings such as `runWith`.
func AnswerCodeIdentitySurfacesCompatible(left, right string) bool {
	leftSegments := answerCodeIdentitySegments(left)
	rightSegments := answerCodeIdentitySegments(right)
	if len(leftSegments) == 0 || len(rightSegments) == 0 {
		return false
	}
	if answerCodeIdentitySegmentSlicesEqual(leftSegments, rightSegments) {
		return true
	}
	if len(leftSegments) == 1 {
		return leftSegments[0] == rightSegments[len(rightSegments)-1]
	}
	if len(rightSegments) == 1 {
		return rightSegments[0] == leftSegments[len(leftSegments)-1]
	}
	return false
}

// AnswerCodeIdentityOwnsEndpoint reports whether an already-typed participant
// identity is the receiver/owner of an exact operation endpoint. It is a
// presentation/coverage bridge only: the endpoint's existing EvidenceItem
// remains the sole relation authority.
//
// A participant must match the complete suffix of the endpoint owner. Thus
// Mutable owns ctx.Mutable.Reset(), Logger owns pkg.Logger.log(), and
// pkg.Logger owns pkg.Logger.log(); Reset and pkg.Other do not. Matching a
// suffix instead of any interior token keeps outer namespaces/containers from
// claiming an operation owned by a deeper, different receiver.
func AnswerCodeIdentityOwnsEndpoint(participant, endpoint string) bool {
	participantSegments := answerCodeIdentitySegments(participant)
	endpointSegments := answerCodeIdentitySegments(endpoint)
	if len(participantSegments) == 0 || len(endpointSegments) < 2 {
		return false
	}
	ownerSegments := endpointSegments[:len(endpointSegments)-1]
	if len(participantSegments) > len(ownerSegments) {
		return false
	}
	offset := len(ownerSegments) - len(participantSegments)
	for i := range participantSegments {
		if participantSegments[i] != ownerSegments[offset+i] {
			return false
		}
	}
	return true
}

// AnswerCodeIdentitySurfacesEquivalent reports presentation-only identity
// equivalence: both surfaces must have the same complete segment sequence, but
// may use different language-native separators (`.`, `::`, `#`, `->`, `/`).
// Unlike AnswerCodeIdentitySurfacesCompatible it never lets a short tail stand
// for a qualified owner, so hard relation gates can normalize syntax without
// collapsing same-named methods from different types.
func AnswerCodeIdentitySurfacesEquivalent(left, right string) bool {
	return answerCodeIdentitySegmentSlicesEqual(
		answerCodeIdentitySegments(left),
		answerCodeIdentitySegments(right),
	)
}

// AnswerCodeIdentityContainsExactSegment reports whether an exact typed code
// identity contains one complete normalized segment. It does not perform
// substring, plural, or naming-convention inference.
func AnswerCodeIdentityContainsExactSegment(identity, segment string) bool {
	segments := answerCodeIdentitySegments(identity)
	want := answerCodeIdentitySegments(segment)
	if len(segments) == 0 || len(want) != 1 {
		return false
	}
	return answerCodeIdentityContainsSegment(segments, want[0])
}

func answerCodeIdentitySegmentSlicesEqual(left, right []string) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func answerCodeIdentitySegments(raw string) []string {
	raw = strings.Trim(strings.TrimSpace(raw), "`'\"")
	raw = strings.TrimSuffix(raw, "()")
	if raw == "" || strings.ContainsAny(raw, "\n\r\t ") {
		return nil
	}
	raw = strings.NewReplacer(
		"::", ".",
		"->", ".",
		"#", ".",
		"/", ".",
		`\`, ".",
	).Replace(raw)
	var out []string
	for _, segment := range strings.Split(raw, ".") {
		segment = strings.ToLower(strings.Trim(strings.TrimSpace(segment), "*&( )"))
		if segment == "" || !IsCodeIdentitySurface(segment) {
			return nil
		}
		out = append(out, segment)
	}
	return out
}

func answerCodeSurfaceBoundaryOK(text string, start int, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if answerCodeSurfaceRune(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if answerCodeSurfaceRune(r) {
			return false
		}
	}
	return true
}

func answerCodeSurfaceRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		r == '_' || r == '.' || r == '-' || r == '/' || r == ':' || r == '@'
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
	if !HasCodeOrConfigPathSuffix(raw) {
		return "", false
	}
	return displayAnswerLocationFile(raw), true
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
	if strings.ContainsAny(raw, ",，、") {
		return parseAnswerLineListSurface(raw)
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

func parseAnswerLineListSurface(raw string) (int, int, bool) {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '、'
	})
	if len(parts) == 0 {
		return 0, 0, false
	}
	lines := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || strings.Contains(part, "-") {
			return 0, 0, false
		}
		line, err := strconv.Atoi(part)
		if err != nil || line <= 0 {
			return 0, 0, false
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return 0, 0, false
	}
	start := lines[0]
	end := start
	contiguous := true
	for i := 1; i < len(lines); i++ {
		if lines[i] != lines[i-1]+1 {
			contiguous = false
			break
		}
		end = lines[i]
	}
	if !contiguous {
		end = start
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

func displayAnswerLocationFile(file string) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	for strings.HasPrefix(file, "./") {
		file = strings.TrimPrefix(file, "./")
	}
	return file
}

func answerLocationFileMatches(labelFile, citationFile string) bool {
	labelFile = normalizeAnswerLocationFile(labelFile)
	citationFile = normalizeAnswerLocationFile(citationFile)
	if labelFile == "" || citationFile == "" {
		return false
	}
	if labelFile == citationFile {
		return true
	}
	return strings.HasSuffix(citationFile, "/"+labelFile)
}
