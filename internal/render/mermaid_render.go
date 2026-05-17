package render

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pgavlin/mermaid-ascii/pkg/diagram"
	"github.com/pgavlin/mermaid-ascii/pkg/render"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// mermaidRenderingEnabled is the master switch for the
// mermaid → ASCII transformation. Default true (REPL / interactive
// terminal mode); set to false by single-shot CLI runners so the
// model's mermaid source ships verbatim to stdout. CLI users
// commonly pipe codrax output to a file, a markdown renderer, a
// mermaid-cli, or paste into a doc — all of those consume mermaid
// source natively and would not benefit from a pre-baked ASCII
// grid. REPL users read the answer immediately in a terminal so
// the ASCII transformation is the right default.
//
// Process-global because there is exactly one render mode per
// invocation; cmd/root.go's runSingleShot toggles it off before
// orchestrator dispatch.
var mermaidRenderingEnabled = true

// SetMermaidRenderingEnabled is the master switch toggle. Pass
// false to disable the mermaid → ASCII transformation; mermaid
// fenced blocks then pass through unchanged.
func SetMermaidRenderingEnabled(enabled bool) {
	mermaidRenderingEnabled = enabled
}

// RenderMermaidBlocks scans `text` for fenced code blocks tagged
// `mermaid` and rewrites each block in place: the mermaid source
// is rendered to deterministically-aligned ASCII via the
// pgavlin/mermaid-ascii library, then the fence is rewritten to
// `text` (so glamour's chroma path skips syntax highlighting and
// preserves monospace alignment). Returns text unchanged when the
// master switch (mermaidRenderingEnabled) is off.
//
// Failure semantics — strictly safe / no regression possible:
//
//   - Library returns an error → block left unchanged (user sees
//     mermaid source verbatim, no worse than before)
//   - Renderer panics on a corner-case DAG → recover() catches,
//     block left unchanged, warning logged
//   - Block is empty / whitespace-only → left unchanged
//   - No `mermaid` blocks present → text returned unchanged
//
// The function is the integration point for the answer pipeline:
// emit_answer_document calls it once on the AnswerDocument.Summary
// after all other normalisation passes have run, so the rendered
// ASCII flows through the rest of the rendering chain untouched.
//
// Cross-platform: pgavlin/mermaid-ascii is pure Go (single import
// dep, MIT licensed), uses runewidth for width calculations, no
// syscalls or shell-out. Output is the same on every platform.
// MermaidBlockDiagnostic describes the outcome of one mermaid block
// processed by TryRenderMermaidBlocks. Used by the renderability
// gate (internal/tool/emit_answer_document.go) to decide whether to
// re-prompt the LLM.
//
// Outcome semantics (load-bearing):
//
//   - OutcomeRendered      — body went through the library and
//     produced an aligned ASCII grid. No re-prompt needed.
//   - OutcomeFallbackRune  — wide rune adapter used narrowRuneFallback
//     for one or more decoration runes (→ … etc). The diagram still
//     rendered; this is INFORMATIONAL — no re-prompt.
//   - OutcomeUnsupportedKind — diagram type is outside pgavlin's
//     subset (classDiagram, stateDiagram, ...). Source preserved
//     under a ```text``` fallback fence. NO re-prompt — this is a
//     library capability gap, not an LLM mistake. The LLM emitted
//     valid Mermaid; the library can't draw it.
//   - OutcomeLibraryRejected — body reached the library and the
//     parse failed. This is the ONLY outcome that re-prompting can
//     potentially fix: the LLM emitted text that pgavlin can't
//     parse AND that the L1+L2 shims can't repair. Surface to the
//     gate so it can ask the LLM to re-emit.
type MermaidBlockOutcome int

const (
	OutcomeRendered MermaidBlockOutcome = iota
	OutcomeFallbackRune
	OutcomeUnsupportedKind
	OutcomeLibraryRejected
)

// MermaidBlockDiagnostic carries per-block result for the
// renderability gate.
type MermaidBlockDiagnostic struct {
	Outcome MermaidBlockOutcome
	// Keyword is the diagram-type token (flowchart / classDiagram /
	// ...) when detectable. Empty if the body lacks one.
	Keyword string
	// Reason is a one-line user-facing explanation. Used to seed
	// the gate's repair Hint. Always non-empty when Outcome !=
	// OutcomeRendered.
	Reason string
	// BodyExcerpt is a small (≤ 200 char) excerpt of the original
	// mermaid body used for repair hints. Truncated to avoid
	// flooding the next prompt.
	BodyExcerpt string
}

// TryRenderMermaidBlocks performs a dry-run render pass and returns
// per-block diagnostics. Used by the renderability gate. The
// returned text is the same string RenderMermaidBlocks would return
// (for the eventual emit) — running both is wasteful, so callers
// can use this once and cache the rewritten text.
//
// Called BEFORE doc.Summary = render.RenderMermaidBlocks(doc.Summary)
// in emit_answer_document.go so the gate can examine the LLM's
// original source. Callers that don't need diagnostics should use
// RenderMermaidBlocks directly.
func TryRenderMermaidBlocks(text string) (rewritten string, diags []MermaidBlockDiagnostic) {
	if !mermaidRenderingEnabled {
		return text, nil
	}
	if text == "" || !strings.Contains(text, "```") || !mayContainMermaid(text) {
		return text, nil
	}
	out := fencedBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		// Snapshot stats before; per-block diff tells us outcome.
		preAttempts := mermaidAttempts.Load()
		preSucceeded := mermaidSucceeded.Load()
		preFallback := mermaidFallbackSubst.Load()
		preLibRej := mermaidLibraryRejected.Load()
		preUnsupported := totalUnsupportedKindCount()

		rewritten := maybeReplaceMermaidFence(match)

		postAttempts := mermaidAttempts.Load()
		postSucceeded := mermaidSucceeded.Load()
		postFallback := mermaidFallbackSubst.Load()
		postLibRej := mermaidLibraryRejected.Load()
		postUnsupported := totalUnsupportedKindCount()

		// Block was non-mermaid (no attempt counted, no unsupported
		// short-circuit) — skip diagnostic.
		if postAttempts == preAttempts && postUnsupported == preUnsupported {
			return rewritten
		}
		// Extract keyword from original match for diagnostic
		// payload (works regardless of outcome).
		kw := keywordFromFenceMatch(match)
		body := bodyFromFenceMatch(match)
		excerpt := body
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "…"
		}

		switch {
		case postUnsupported > preUnsupported:
			diags = append(diags, MermaidBlockDiagnostic{
				Outcome:     OutcomeUnsupportedKind,
				Keyword:     kw,
				Reason:      "diagram kind [" + kw + "] is not in the terminal renderer's supported subset (flowchart / graph / sequenceDiagram); source preserved as-is",
				BodyExcerpt: excerpt,
			})
		case postLibRej > preLibRej:
			diags = append(diags, MermaidBlockDiagnostic{
				Outcome:     OutcomeLibraryRejected,
				Keyword:     kw,
				Reason:      "terminal Mermaid renderer rejected the body; check Mermaid-spec syntax (matched brackets, quoted special-char labels, valid edge syntax)",
				BodyExcerpt: excerpt,
			})
		case postFallback > preFallback:
			diags = append(diags, MermaidBlockDiagnostic{
				Outcome:     OutcomeFallbackRune,
				Keyword:     kw,
				Reason:      "decoration runes substituted with ASCII equivalents",
				BodyExcerpt: excerpt,
			})
		case postSucceeded > preSucceeded:
			// Pure success — no diagnostic.
		}
		return rewritten
	})
	return out, diags
}

// totalUnsupportedKindCount sums every key in mermaidUnsupportedKind
// — used to detect an unsupported short-circuit increment in
// TryRenderMermaidBlocks.
func totalUnsupportedKindCount() int64 {
	mermaidUnsupportedKindLock.Lock()
	var total int64
	for _, v := range mermaidUnsupportedKind {
		total += v
	}
	mermaidUnsupportedKindLock.Unlock()
	return total
}

// keywordFromFenceMatch peels the first mermaid keyword out of a
// fenced-block match. Looks at info-string then body's first
// non-empty line.
func keywordFromFenceMatch(match string) string {
	nl := strings.Index(match, "\n")
	if nl < 0 {
		return ""
	}
	infoLine := strings.TrimSpace(match[3:nl])
	if kw := firstMermaidKeywordIn(infoLine); kw != "" {
		return kw
	}
	if strings.HasPrefix(infoLine, "mermaid") {
		bodyEnd := strings.LastIndex(match, "\n```")
		if bodyEnd > nl {
			return firstMermaidKeywordIn(firstNonEmptyTrimmed(match[nl+1 : bodyEnd]))
		}
	}
	return ""
}

func bodyFromFenceMatch(match string) string {
	nl := strings.Index(match, "\n")
	bodyEnd := strings.LastIndex(match, "\n```")
	if nl < 0 || bodyEnd <= nl {
		return ""
	}
	return strings.TrimSpace(match[nl+1 : bodyEnd])
}

func RenderMermaidBlocks(text string) string {
	if !mermaidRenderingEnabled {
		return text
	}
	if text == "" {
		return text
	}
	// Two-stage prefilter so chitchat / fallback prose carrying
	// non-mermaid fenced blocks (```bash```, ```json```, ```go```)
	// short-circuits BEFORE the regex scan:
	//
	//   1. Must contain at least one fenced block ("```").
	//   2. Must contain either an explicit `mermaid` tag OR one of
	//      the body-keyword markers used by looksLikeMermaidBody.
	//
	// Step 2 is a fast strings.Contains on the source — a single
	// pass over the text — instead of running the regex on every
	// fence in long replies that have no diagram. If neither
	// indicator is present the function returns text byte-identical.
	if !strings.Contains(text, "```") {
		return text
	}
	if !mayContainMermaid(text) {
		return text
	}
	return fencedBlockRe.ReplaceAllStringFunc(text, maybeReplaceMermaidFence)
}

// mayContainMermaid is the cheap prefilter for RenderMermaidBlocks.
// True iff the text either carries a `mermaid` tag or a known
// diagram-type keyword anywhere — both quick substring checks. A
// false positive here just costs one regex scan; a false negative
// would silently skip rendering, so the keyword set MUST stay in
// sync with mermaidBodyKeywords (the per-block decision used inside
// looksLikeMermaidBody).
func mayContainMermaid(text string) bool {
	if strings.Contains(text, "```mermaid") {
		return true
	}
	// Also fast-pass when the text contains a fence-as-keyword shape
	// (e.g. ```flowchart, ```sequenceDiagram). The substring check
	// is identical; what matters is that the keyword set covers BOTH
	// "fence-tagged ```mermaid + body-keyword" AND "fence-tagged
	// ```keyword directly".
	for _, kw := range mermaidBodyKeywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// fencedBlockRe matches any fenced code block. We dispatch in
// maybeReplaceMermaidFence based on info-string + body shape so a
// model-emitted bare ``` fence whose body starts with a mermaid
// diagram-type keyword still gets rendered. The capture group holds
// the info-string (info[0]) and the body (info[1]).
//
// Pre-2026-04-30 the regex was tag-anchored at ```mermaid only, so
// the user reported that a model-emitted bare ``` fence containing
// `flowchart LR ...` printed as raw text. The model commonly drops
// the tag when the surrounding prose already names the diagram type;
// we cope by detecting mermaid-shaped bodies regardless of tag.
//
// Pattern shape:
//
//	``` <info-string-zero-or-more-chars>
//	<BODY>
//	```
var fencedBlockRe = regexp.MustCompile("(?s)```([^\\n]*)\\n(.*?)\\n```")

// mermaidFenceRe is retained for the original ```mermaid``` shape.
// Used by replaceMermaidFence's body extraction (it lifts the body
// from match using string indexing, not the regex captures, so
// callers downstream remain stable).
var mermaidFenceRe = regexp.MustCompile("(?s)```mermaid[^\\n]*\\n(.*?)\\n```")

// mermaidBodyKeywords lists every Mermaid-spec diagram-type token
// we recognise as a "mermaid block" for routing purposes. The first
// non-empty body line of an untagged fence must begin with one of
// these (case-sensitive — Mermaid spec).
//
// Routing distinction (load-bearing):
//
//   - mermaidSupportedKeywords — the subset pgavlin/mermaid-ascii
//     can actually render (flowchart / graph / sequenceDiagram).
//     These go through the library.
//
//   - mermaidUnsupportedKeywords — everything else in Mermaid spec
//     (classDiagram, stateDiagram, erDiagram, ...). These are valid
//     Mermaid that the LLM is right to emit per Mermaid standard,
//     but pgavlin's subset cannot draw them. Rather than feeding a
//     doomed body to the library and producing a bare fence + raw
//     mermaid tag (which downstream chroma would syntax-highlight,
//     giving the user the misleading "looks rendered" effect), we
//     short-circuit at routing time: rewrite the fence to ```text
//     and inject a "· <kind> not in renderer subset" leader so the
//     user sees an explicit signal. The mermaid SOURCE survives
//     verbatim — useful for copy-paste into a real Mermaid renderer.
//
// mermaidBodyKeywords is the union, used by mayContainMermaid /
// looksLikeMermaidBody / infoLineStartsWithMermaidKeyword for the
// detection step. The supported / unsupported split kicks in inside
// maybeReplaceMermaidFence after detection.
var mermaidBodyKeywords = append(append([]string(nil), mermaidSupportedKeywords...), mermaidUnsupportedKeywords...)

var mermaidSupportedKeywords = []string{
	"flowchart",
	"graph",
	"sequenceDiagram",
}

var mermaidUnsupportedKeywords = []string{
	"classDiagram",
	"stateDiagram",
	"stateDiagram-v2",
	"erDiagram",
	"journey",
	"gantt",
	"pie",
	"mindmap",
	"timeline",
	"gitGraph",
	"requirementDiagram",
	"C4Context",
}

// firstMermaidKeywordIn returns the first known mermaid keyword the
// line begins with (matching word-boundary so "graph" doesn't shadow
// "graphTD"). Returns "" when no keyword matches. Used by the
// supported / unsupported routing split.
func firstMermaidKeywordIn(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	for _, kw := range mermaidBodyKeywords {
		if line == kw || strings.HasPrefix(line, kw+" ") || strings.HasPrefix(line, kw+"\t") {
			return kw
		}
	}
	return ""
}

// firstNonEmptyTrimmed returns the first non-empty trimmed line of a
// body, used to identify the diagram-type keyword.
func firstNonEmptyTrimmed(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// isMermaidSupportedKind reports whether the keyword belongs to the
// pgavlin subset we can actually render.
func isMermaidSupportedKind(kw string) bool {
	for _, s := range mermaidSupportedKeywords {
		if s == kw {
			return true
		}
	}
	return false
}

// infoLineStartsWithMermaidKeyword reports whether the fence's
// info-string IS a mermaid diagram-type keyword (potentially
// followed by a direction modifier or arguments). Models that drop
// the `mermaid` tag often write the keyword directly as the info-
// string: ```flowchart TD, ```sequenceDiagram, ```graph LR, etc.
// The body in that case starts WITHOUT the keyword line, so the
// caller must reassemble the `mermaid`-tagged form by prepending
// the info-line as the body's first line.
func infoLineStartsWithMermaidKeyword(info string) bool {
	info = strings.TrimSpace(info)
	if info == "" {
		return false
	}
	for _, kw := range mermaidBodyKeywords {
		if info == kw || strings.HasPrefix(info, kw+" ") || strings.HasPrefix(info, kw+"\t") {
			return true
		}
	}
	return false
}

// preprocessMermaidBody is the single entry point that runs every
// renderer-compatibility shim before the body is handed to
// pgavlin/mermaid-ascii. Each shim corresponds to one Mermaid-spec
// feature the library subset does not support. The boundary
// (load-bearing): every shim is a SOURCE-LAYER normalisation that
// preserves what the diagram MEANS — the LLM still emits standard
// Mermaid, the library still does the rendering, the system fills
// the capability gap in between.
func preprocessMermaidBody(body string) string {
	body = mermaidcompat.NormalizeSequenceStops(body)
	body = normalizeSequenceDiagramEndpointAliases(body)
	body = flattenMermaidSubgraphs(body)
	body = normalizeMermaidLabels(body)
	return body
}

var mermaidSequenceSafeIDRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// normalizeSequenceDiagramEndpointAliases rewrites sequenceDiagram
// edges whose actor endpoint is a Mermaid label rather than a valid
// actor identifier, for example:
//
//	Caller->>mm/page_alloc.c:5190: __alloc_frozen_pages_noprof
//
// Mermaid itself allows labels through `participant id as "label"`,
// but pgavlin/mermaid-ascii parses edge endpoints as identifiers. The
// source-layer shim below preserves the diagram meaning by introducing
// deterministic actor aliases and moving the original file:line text
// into participant labels before the renderer sees the body.
func normalizeSequenceDiagramEndpointAliases(body string) string {
	lines := strings.Split(body, "\n")
	first := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.TrimSpace(line) != "sequenceDiagram" {
			return body
		}
		first = i
		break
	}
	if first < 0 {
		return body
	}
	type edgeLine struct {
		index int
		from  string
		arrow string
		to    string
		msg   string
	}
	var edges []edgeLine
	aliasByLabel := make(map[string]string)
	declaredAlias := make(map[string]bool)
	declaredLabel := make(map[string]bool)
	generatedAliasFor := func(label string) string {
		label = strings.TrimSpace(label)
		if label == "" {
			return ""
		}
		if alias := aliasByLabel[label]; alias != "" {
			return alias
		}
		alias := fmt.Sprintf("codraxSeq%d", len(aliasByLabel)+1)
		aliasByLabel[label] = alias
		return alias
	}
	aliasFor := func(label string) string {
		label = strings.TrimSpace(label)
		if label == "" {
			return ""
		}
		if mermaidSequenceSafeIDRe.MatchString(label) {
			return label
		}
		return generatedAliasFor(label)
	}
	for i := first + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if alias, label, ok := parseSequenceParticipantLine(trimmed); ok {
			if alias == "" {
				continue
			}
			if mermaidSequenceSafeIDRe.MatchString(alias) {
				declaredAlias[alias] = true
				if label != "" {
					declaredLabel[label] = true
					aliasByLabel[label] = alias
				}
				continue
			}
			generated := generatedAliasFor(alias)
			display := strings.TrimSpace(label)
			if display == "" {
				display = alias
			}
			if display != alias {
				aliasByLabel[display] = generated
			}
			declaredAlias[generated] = true
			if display != "" {
				declaredLabel[display] = true
			}
			indent := leadingWhitespace(lines[i])
			lines[i] = fmt.Sprintf(`%sparticipant %s as "%s"`, indent, generated, escapeMermaidParticipantLabel(display))
			continue
		}
		from, arrow, to, msg, ok := parseSequenceEdgeLine(trimmed)
		if !ok {
			continue
		}
		fromAlias := aliasFor(from)
		toAlias := aliasFor(to)
		if fromAlias == from && toAlias == to {
			continue
		}
		edges = append(edges, edgeLine{index: i, from: fromAlias, arrow: arrow, to: toAlias, msg: msg})
	}
	if len(aliasByLabel) == 0 {
		return body
	}
	for _, edge := range edges {
		indent := leadingWhitespace(lines[edge.index])
		lines[edge.index] = fmt.Sprintf("%s%s%s%s: %s", indent, edge.from, edge.arrow, edge.to, edge.msg)
	}
	var declarations []string
	for label, alias := range aliasByLabel {
		if declaredAlias[alias] || declaredLabel[label] {
			continue
		}
		declarations = append(declarations, fmt.Sprintf(`    participant %s as "%s"`, alias, escapeMermaidParticipantLabel(label)))
	}
	sort.Slice(declarations, func(i, j int) bool { return declarations[i] < declarations[j] })
	if len(declarations) == 0 {
		return strings.Join(lines, "\n")
	}
	out := make([]string, 0, len(lines)+len(declarations))
	out = append(out, lines[:first+1]...)
	out = append(out, declarations...)
	out = append(out, lines[first+1:]...)
	return strings.Join(out, "\n")
}

func parseSequenceParticipantLine(line string) (alias, label string, ok bool) {
	const prefix = "participant "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if rest == "" {
		return "", "", true
	}
	parts := strings.SplitN(rest, " as ", 2)
	alias = strings.TrimSpace(parts[0])
	label = alias
	if len(parts) == 2 {
		label = strings.Trim(strings.TrimSpace(parts[1]), `"`)
	}
	return alias, label, true
}

func parseSequenceEdgeLine(line string) (from, arrow, to, msg string, ok bool) {
	arrowStart, arrow := findSequenceArrow(line)
	if arrowStart < 0 {
		return "", "", "", "", false
	}
	from = strings.TrimSpace(line[:arrowStart])
	rest := strings.TrimSpace(line[arrowStart+len(arrow):])
	to, msg, ok = splitSequenceTargetAndMessage(rest)
	if !ok || from == "" || to == "" {
		return "", "", "", "", false
	}
	return from, arrow, to, msg, true
}

func findSequenceArrow(line string) (int, string) {
	arrows := []string{
		"-->>+", "-->>-", "->>+", "->>-",
		"--)+", "--)-", "-)+", "-)-",
		"--x+", "--x-", "-x+", "-x-",
		"-->+", "-->-", "->+", "->-",
		"-->>", "->>", "-->", "->", "--x", "-x", "--)", "-)",
	}
	best := -1
	bestArrow := ""
	for _, arrow := range arrows {
		idx := strings.Index(line, arrow)
		if idx < 0 {
			continue
		}
		if best < 0 || idx < best || (idx == best && len(arrow) > len(bestArrow)) {
			best = idx
			bestArrow = arrow
		}
	}
	return best, bestArrow
}

func splitSequenceTargetAndMessage(rest string) (target, msg string, ok bool) {
	first := strings.Index(rest, ":")
	if first < 0 {
		return "", "", false
	}
	if next := sourceLocationMessageColon(rest, first); next > first {
		return strings.TrimSpace(rest[:next]), strings.TrimSpace(rest[next+1:]), true
	}
	return strings.TrimSpace(rest[:first]), strings.TrimSpace(rest[first+1:]), true
}

func sourceLocationMessageColon(rest string, firstColon int) int {
	prefix := strings.TrimSpace(rest[:firstColon])
	if prefix == "" || (!strings.Contains(prefix, "/") && !strings.Contains(prefix, ".")) {
		return -1
	}
	i := firstColon + 1
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == firstColon+1 || i >= len(rest) || rest[i] != ':' {
		return -1
	}
	return i
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

func escapeMermaidParticipantLabel(label string) string {
	return strings.ReplaceAll(label, `"`, `\"`)
}

// htmlEntityToText maps the HTML named entities Mermaid spec
// allows inside labels to their plain-text equivalent. We keep the
// set narrow — only the entities that actually appear in real
// Mermaid corpus — to avoid acting like a half-baked HTML parser.
var htmlEntityToText = map[string]string{
	"&nbsp;": " ",
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": "\"",
	"&apos;": "'",
}

// brTagRe matches every <br> / <br/> / <br /> variant
// case-insensitively. Mermaid spec accepts <br/> in labels for
// multi-line text; pgavlin's flowchart parser does not. We
// normalise to a single space so the label still reads as one
// fluid string and the box arithmetic stays predictable.
var brTagRe = regexp.MustCompile(`(?i)<br\s*/?>`)

// normalizeMermaidLabels rewrites Mermaid-spec HTML markup that
// pgavlin/mermaid-ascii's flowchart subset does not handle. Two
// transforms:
//
//  1. <br>, <br/>, <br /> (any case)         → single space
//  2. &nbsp; / &amp; / &lt; / &gt; / &quot; / &apos; → plain text
//
// We deliberately only touch contents inside fenced LABELS would be
// risky to detect statefully; instead we run on the whole body and
// rely on the fact that the source-side characters we replace are
// not load-bearing for Mermaid SYNTAX (parser doesn't use `<br/>`
// or HTML entities anywhere meaningful outside labels).
func normalizeMermaidLabels(body string) string {
	if !strings.Contains(body, "<") && !strings.Contains(body, "&") {
		return body
	}
	body = brTagRe.ReplaceAllString(body, " ")
	for entity, repl := range htmlEntityToText {
		if strings.Contains(body, entity) {
			body = strings.ReplaceAll(body, entity, repl)
		}
	}
	return body
}

// flattenMermaidSubgraphs removes `subgraph <name> [...]` and the
// matching `end` lines from a mermaid body while preserving every
// node declaration and every edge inside the cluster. Pgavlin's
// flowchart parser rejects subgraph syntax; flattening keeps the
// graph structure renderable as a flat flowchart at the cost of
// losing the visual cluster boundary (acceptable in terminal).
//
// Algorithm: scan line-by-line, drop lines whose trimmed content
// starts with "subgraph " or equals "end" at any indentation. A
// stray "end" outside a subgraph is impossible in valid Mermaid
// because end is reserved for subgraph close — the parser uses
// it nowhere else inside flowchart bodies.
func flattenMermaidSubgraphs(body string) string {
	if !strings.Contains(body, "subgraph") {
		return body
	}
	var b strings.Builder
	b.Grow(len(body))
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "subgraph ") || trimmed == "subgraph" {
			continue
		}
		if trimmed == "end" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// looksLikeMermaidBody returns true when body's first non-empty
// trimmed line begins with a known mermaid diagram-type keyword.
// Used to opt untagged fences into mermaid rendering.
func looksLikeMermaidBody(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, kw := range mermaidBodyKeywords {
			if line == kw || strings.HasPrefix(line, kw+" ") || strings.HasPrefix(line, kw+"\t") {
				return true
			}
		}
		return false
	}
	return false
}

// maybeReplaceMermaidFence dispatches a fenced-block match to one of
// four outcomes:
//
//  1. Detected as mermaid AND keyword is in the supported subset
//     (flowchart / graph / sequenceDiagram) → render via library;
//     success returns the ASCII grid as ```text``` fence; failure
//     returns the unsupported-kind fallback fence (see step 4).
//
//  2. Detected as mermaid AND keyword is in the unsupported subset
//     (classDiagram / stateDiagram / ...) → short-circuit to the
//     unsupported-kind fallback fence. We do not attempt to render;
//     the library cannot draw those shapes.
//
//  3. Mermaid tag present but no recognisable keyword in body —
//     render attempt; failure → fallback fence.
//
//  4. Anything else (```go / ```bash / ```json / etc.) → leave
//     byte-identical.
//
// Failure fence shape (step 1, 2, 3 fallback): rewrite to ```text```
// and inject a single leader line (`# · <reason>`) above the original
// mermaid source. This guarantees the user always sees an explicit
// signal that the diagram did not render, AND chroma never highlights
// the body as code (because the fence is ```text```), eliminating the
// "looks rendered" visual deception.
func maybeReplaceMermaidFence(match string) string {
	// Find newline that ends the info-string line.
	nl := strings.Index(match, "\n")
	if nl < 0 {
		return match
	}
	infoLine := strings.TrimSpace(match[3:nl]) // skip leading ```
	bodyEnd := strings.LastIndex(match, "\n```")
	if bodyEnd <= nl {
		return match
	}
	body := match[nl+1 : bodyEnd]

	// Case 1: explicit mermaid tag.
	if strings.HasPrefix(infoLine, "mermaid") {
		// Inspect first non-empty body line for keyword.
		first := firstNonEmptyTrimmed(body)
		if kw := firstMermaidKeywordIn(first); kw != "" && !isMermaidSupportedKind(kw) {
			recordMermaidUnsupportedKind(kw)
			return mermaidFallbackFence(body, "Mermaid 子集 ["+kw+"] 不在终端渲染器支持范围（仅支持 flowchart/graph/sequenceDiagram），原始源码已保留供复制到完整 Mermaid 渲染器")
		}
		return replaceMermaidFence(match)
	}
	// Case 1b: info-string IS a mermaid keyword (e.g. ```flowchart TD,
	// ```sequenceDiagram, ```graph LR). Some models drop the
	// `mermaid` tag and put the diagram-type keyword directly as
	// the info-string. The body starts WITHOUT the keyword line, so
	// we synthesise a `mermaid`-tagged form that prepends the
	// info-line as the first body line.
	if kw := firstMermaidKeywordIn(infoLine); kw != "" {
		if !isMermaidSupportedKind(kw) {
			recordMermaidUnsupportedKind(kw)
			full := infoLine + "\n" + body
			return mermaidFallbackFence(full, "Mermaid 子集 ["+kw+"] 不在终端渲染器支持范围（仅支持 flowchart/graph/sequenceDiagram），原始源码已保留供复制到完整 Mermaid 渲染器")
		}
		synth := "```mermaid\n" + infoLine + "\n" + body + "\n```"
		if rendered, ok := renderMermaidFenceBody(synth); ok {
			return rendered
		}
		return mermaidFallbackFence(infoLine+"\n"+body, "终端 Mermaid 渲染器解析失败，原始源码已保留")
	}
	// Case 2: untagged or `text`-tagged fence whose body shape is
	// mermaid. We only match the empty-info-string and `text` cases
	// to keep the contract narrow — a body with `flowchart` keywords
	// inside a ```bash``` fence is unambiguously not a diagram.
	if (infoLine == "" || infoLine == "text") && looksLikeMermaidBody(body) {
		first := firstNonEmptyTrimmed(body)
		if kw := firstMermaidKeywordIn(first); kw != "" && !isMermaidSupportedKind(kw) {
			recordMermaidUnsupportedKind(kw)
			// Untagged input — we already had ```text or bare; preserve
			// as ```text with the warning leader.
			return mermaidFallbackFence(body, "Mermaid 子集 ["+kw+"] 不在终端渲染器支持范围（仅支持 flowchart/graph/sequenceDiagram），原始源码已保留")
		}
		// Synthesize a `mermaid`-tagged match so the renderer's
		// body-extraction logic stays load-bearing on a single
		// shape. We use the explicit (rendered, ok) signal — NOT
		// string-equality against the synthesized form — so the
		// failure path returns the ORIGINAL match (not the synth)
		// regardless of how the failure manifests inside the
		// renderer.
		synth := "```mermaid\n" + body + "\n```"
		if rendered, ok := renderMermaidFenceBody(synth); ok {
			return rendered
		}
		// For untagged input, leave-as-is: there's no ```mermaid
		// label to mislead chroma. We do NOT inject a warning here
		// because the fence was already ```text-equivalent and the
		// body might have been unintentionally mermaid-shaped.
		return match
	}
	return match
}

// mermaidFallbackFence builds the unified failure-and-unsupported
// fence: rewrite info-string to `text` so chroma does not highlight
// the body as code, and prepend a single `# · <reason>` leader line
// so the user always sees an explicit signal. The ORIGINAL mermaid
// source survives verbatim below the leader, useful for copying
// into a real Mermaid renderer (Mermaid.live / mermaid-cli).
//
// This is the primary fix for the "looks rendered but isn't"
// failure mode: pre-2026-05-02 the failure path returned the
// original ```mermaid``` fence unchanged, which downstream chroma
// then syntax-highlighted as code, giving the user the misleading
// impression rendering had occurred.
func mermaidFallbackFence(body, reason string) string {
	body = strings.TrimRight(body, "\n")
	var b strings.Builder
	b.Grow(len(body) + len(reason) + 32)
	b.WriteString("```text\n")
	b.WriteString("# · ")
	b.WriteString(reason)
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n```")
	return b.String()
}

// replaceMermaidFence is the per-match closure handed to
// ReplaceAllStringFunc. `match` is the full fence (including the
// opening ```mermaid line and closing ```).
//
// On success it returns the rewritten fence (```text``` wrapping the
// rendered ASCII grid). On failure it returns `match` unchanged —
// strict no-regression contract.
//
// Callers that need to distinguish success from "left unchanged on
// purpose" (e.g. maybeReplaceMermaidFence which synthesises a fake
// `mermaid`-tagged match for untagged-but-mermaid-shaped bodies and
// must NOT leak the synthesised form back to the user when render
// fails) should use renderMermaidFenceBody directly — that helper
// returns an explicit (rendered, ok) pair so success vs failure is
// not inferred from string identity.
func replaceMermaidFence(match string) string {
	if rendered, ok := renderMermaidFenceBody(match); ok {
		return rendered
	}
	// Failure path — extract body for the fallback fence so the
	// user sees the warning leader plus the original mermaid
	// source as ```text```. NEVER leave the ```mermaid``` tag in
	// place: chroma would syntax-highlight the body and create
	// the "looks rendered but isn't" deception.
	bodyStart := strings.Index(match, "\n")
	bodyEnd := strings.LastIndex(match, "\n```")
	if bodyStart < 0 || bodyEnd <= bodyStart {
		// Malformed fence — keep input verbatim; nothing useful
		// we can do, AND there is no ```mermaid``` deception to
		// guard against if the fence shape was already broken.
		return match
	}
	body := strings.TrimSpace(match[bodyStart+1 : bodyEnd])
	if body == "" {
		// Empty body has no information for the user and no
		// chroma deception risk — leave the original fence
		// byte-identical (preserves the long-standing "empty
		// mermaid body passes through" contract).
		return match
	}
	return mermaidFallbackFence(match[bodyStart+1:bodyEnd], "终端 Mermaid 渲染器解析失败，原始源码已保留")
}

// renderMermaidFenceBody is the explicit (rendered, ok) core of
// mermaid → ASCII transformation. ok=false means the caller should
// keep the ORIGINAL source unchanged (whatever shape it was — bare
// fence, text-tagged fence, mermaid-tagged fence). This separates
// "successfully rendered" from "no-op return value" so callers
// don't have to compare strings to detect failure.
//
// All four failure modes (panic, missing body, library reject,
// empty render) collapse to ok=false here.
func renderMermaidFenceBody(match string) (out string, ok bool) {
	recordMermaidAttempt()
	defer func() {
		if r := recover(); r != nil {
			logging.Warning("[render/mermaid] panic during render (fallback fence): %v", r)
			recordMermaidLibraryRejected()
			out = ""
			ok = false
		}
	}()

	// Extract body. The regex guarantees match shape; body lives
	// between the first newline after ```mermaid and the closing
	// ```.
	bodyStart := strings.Index(match, "\n")
	if bodyStart < 0 {
		return "", false
	}
	bodyEnd := strings.LastIndex(match, "\n```")
	if bodyEnd <= bodyStart {
		return "", false
	}
	body := strings.TrimSpace(match[bodyStart+1 : bodyEnd])
	if body == "" {
		return "", false
	}

	// Renderer-compatibility layer: every shim run before the
	// library sees the body normalises one Mermaid-spec feature
	// pgavlin's subset does not handle. The shims preserve diagram
	// MEANING (we never change what the LLM intended), only the
	// byte form fed to a library whose subset is narrower than
	// Mermaid spec.
	body = preprocessMermaidBody(body)

	// CJK + decoration-rune adapter (see cjk_adapter.go). Wide
	// runes (CJK / Hiragana / Katakana / Hangul) substitute to
	// 2-byte ASCII placeholders for byte/cell parity; narrow
	// multi-byte decoration runes (→ … — ✓ ●) substitute to ASCII
	// equivalents from narrowRuneFallback (cell-equivalent in the
	// final grid).
	adapter := newCJKAdapter(body)
	preparedBody, _, substErr := adapter.substitute(body)
	if substErr != nil {
		// Only path that still errors: placeholder pool exhausted
		// (>1296 unique wide runes — almost never reached). Genuine
		// "cannot render", flag at WARN.
		logging.Warning("[render/mermaid] cjk adapter pool exhausted: %v", substErr)
		recordMermaidLibraryRejected()
		return "", false
	}
	if degraded := adapter.DegradedRunes(); len(degraded) > 0 {
		recordMermaidFallbackSubstituted(len(degraded))
		logging.Warning("[render/mermaid] %d narrow-multibyte rune(s) replaced via fallback table; example: %q→%q",
			len(degraded), string(degraded[0].Rune), degraded[0].Replacement)
	}

	cfg := diagram.DefaultConfig()
	// Use the library's Unicode box-drawing default (┌─┐│└┘┬┴├┤┼)
	// instead of forcing ASCII (+-|/=). The Unicode characters render
	// as crisp clean borders in every modern monospace terminal
	// (Alacritty, iTerm2, kitty, Windows Terminal, GNOME Terminal,
	// VS Code embedded), and the CJK adapter handles wide-rune
	// alignment by substituting CJK content to ASCII placeholders
	// BEFORE the library renders + restoring AFTER, so the grid the
	// library produces is built against narrow-only widths — the
	// East-Asian-Width ambiguity of the box-drawing characters
	// themselves is irrelevant to label alignment under that scheme.
	// Pre-2026-05-06 we forced ASCII (+/-) to be defensive about
	// legacy CJK terminal builds; the visual cost (chunky plus-sign
	// borders, jagged corners) outweighs the safety margin given the
	// adapter contract.
	// CLI style (no HTML colour markup); plain text body.
	cfg.StyleType = "cli"

	rendered, err := render.Render(preparedBody, cfg)
	if err != nil {
		logging.Warning("[render/mermaid] library render failed (fallback fence): %v", err)
		recordMermaidLibraryRejected()
		return "", false
	}
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		recordMermaidLibraryRejected()
		return "", false
	}
	// Restore wide runes in the rendered grid. The adapter walks
	// the placeholder map and byte-replaces each occurrence; the
	// pool was chosen to exclude any 2-byte pair already present
	// in the source so we don't risk false-positive replacements.
	rendered = adapter.restore(rendered)

	// Rewrap as `text` fence so chroma doesn't tokenize the box-
	// drawing chars as code (which it would if the fence stayed
	// `mermaid` or had no tag and chroma auto-detected). `text` is
	// an explicit "no syntax highlighting" hint chroma honours.
	//
	// Prepend a light header label ("─── flowchart ───") so a long
	// answer with multiple diagram blocks reads with each diagram
	// clearly demarcated. The keyword comes from the body's first
	// non-empty trimmed line; "diagram" is the safety-net fallback.
	header := mermaidHeaderLine(firstMermaidKeywordIn(firstNonEmptyTrimmed(body)))
	var b strings.Builder
	b.Grow(len(rendered) + len(header) + 24)
	b.WriteString("```text\n")
	if header != "" {
		b.WriteString(header)
		b.WriteString("\n")
	}
	b.WriteString(rendered)
	b.WriteString("\n```")
	recordMermaidSucceeded()
	return b.String(), true
}

// mermaidHeaderLine returns a soft "─── <kind> ───" label rendered
// above the ASCII grid. Empty when the keyword is unknown so an
// untyped fallback diagram does not get a meaningless "─── ───"
// line. Kind is lowercased and length-limited for layout sanity.
func mermaidHeaderLine(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return ""
	}
	if len(kind) > 24 {
		kind = kind[:24]
	}
	return "─── " + kind + " ───"
}
