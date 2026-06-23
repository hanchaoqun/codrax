package outputdump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Ext is the suffix used for both the prune glob and new files.
// Hard-coded so the prune sweep never touches unrelated content the
// operator may have placed in the directory.
const Ext = ".md"

// HTMLExt is the sibling browser-ready artifact written next to every
// markdown dump. The markdown file remains the authoritative raw output; the
// HTML is a deterministic system rendering of the same bytes.
const HTMLExt = ".html"

// Args bundles the inputs Write needs. Every caller supplies the raw
// user request text and the raw markdown answer body that reached the
// user's visible answer surface. Terminal chrome, ANSI styling, and
// preview hints are intentionally excluded.
type Args struct {
	Dir              string
	Max              int
	Language         string
	Request          string
	Answer           string
	HasLog           bool
	LogBytes         int
	HasTrace         bool
	TraceBytes       int
	RuntimeArtifacts []RuntimeArtifact
	Now              time.Time
	PID              int
}

type RuntimeArtifact struct {
	Kind   string
	Source string
	Bytes  int
	Detail string
}

var requestPathTokenRE = regexp.MustCompile(`[^\s"'` + "`" + `<>()[\]{}，。；;、]+`)

// Result reports the best-effort artifacts written for one answer dump. A
// markdown path can be present while HTML is empty when the secondary render
// failed; answer delivery must not depend on either artifact.
type Result struct {
	MarkdownPath string
	HTMLPath     string
}

// Write persists the rendered final answer + the user question to
// <dir>/<timestamp>-<pid>.md and prunes oldest files past the retention
// cap. It returns the written path on success. Best-effort: every IO
// error is logged at WARN and swallowed, because transcript dumping is
// a UX affordance and must never alter answer delivery.
func Write(a Args) string {
	return WriteResult(a).MarkdownPath
}

// WriteResult persists the markdown dump and a sibling self-contained HTML
// rendering. Retention is counted by markdown dumps: pruning an old .md removes
// its .html sibling, and orphaned canonical .html dumps are cleaned on the same
// sweep.
func WriteResult(a Args) Result {
	if a.Dir == "" {
		return Result{}
	}
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		logging.Warning("[output_dump] mkdir %s failed: %v", a.Dir, err)
		return Result{}
	}
	PruneDir(a.Dir, a.Max)
	name := FileName(a.Now, a.PID)
	path := filepath.Join(a.Dir, name)
	body := BuildBody(a)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		logging.Warning("[output_dump] write %s failed: %v", path, err)
		return Result{}
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", path, len(body))
	result := Result{MarkdownPath: path}
	htmlPath := HTMLPathForMarkdown(path)
	if htmlPath == "" {
		return result
	}
	htmlBody, err := BuildHTML(filepath.Base(path), body)
	if err != nil {
		logging.Warning("[output_dump] render html for %s failed: %v", path, err)
		return result
	}
	if err := os.WriteFile(htmlPath, []byte(htmlBody), 0o644); err != nil {
		logging.Warning("[output_dump] write %s failed: %v", htmlPath, err)
		return result
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", htmlPath, len(htmlBody))
	result.HTMLPath = htmlPath
	return result
}

// FileName returns the canonical filename, matching tool.SessionDirName's
// `YYYYMMDD-HHMMSS.mmm-<pid>` shape so dump files line up 1:1 with
// .codrax/blob/<id>/ session dirs when the blob layout is enabled.
func FileName(now time.Time, pid int) string {
	if now.IsZero() {
		now = time.Now()
	}
	if pid == 0 {
		pid = os.Getpid()
	}
	return fmt.Sprintf("%s-%d%s",
		now.Format("20060102-150405.000"),
		pid,
		Ext,
	)
}

// BuildBody composes the markdown dump body. No frontmatter: by user
// contract the file carries H1 request/answer sections. Attachments
// surface as a compact typed table when source metadata is available,
// otherwise as legacy quoted footnote lines under the request.
func BuildBody(a Args) string {
	var b strings.Builder
	labels := dumpLabels(a.Language)
	b.WriteString("# ")
	b.WriteString(labels.Question)
	b.WriteString("\n\n")
	req := strings.TrimRight(a.Request, "\n")
	if req == "" {
		req = labels.Empty
	}
	b.WriteString(req)
	b.WriteString("\n")
	if len(a.RuntimeArtifacts) > 0 {
		b.WriteString("\n## ")
		b.WriteString(labels.RuntimeArtifacts)
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", labels.Kind, labels.Source, labels.Size, labels.Detail)
		b.WriteString("|---|---|---:|---|\n")
		for _, artifact := range a.RuntimeArtifacts {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
				escapeMarkdownTableCell(firstNonEmpty(artifact.Kind, "artifact")),
				escapeMarkdownTableCell(firstNonEmpty(artifact.Source, "(unknown)")),
				HumanBytes(artifact.Bytes),
				escapeMarkdownTableCell(artifact.Detail))
		}
	} else if a.HasLog {
		fmt.Fprintf(&b, "\n> %s: log (%s)\n", labels.Attachment, HumanBytes(a.LogBytes))
	}
	if len(a.RuntimeArtifacts) == 0 && a.HasTrace {
		fmt.Fprintf(&b, "\n> %s: htrace (%s)\n", labels.Attachment, HumanBytes(a.TraceBytes))
	}
	b.WriteString("\n# ")
	b.WriteString(labels.Answer)
	b.WriteString("\n\n")
	ans := strings.TrimRight(a.Answer, "\n")
	if ans == "" {
		ans = labels.Empty
	}
	ans = mermaidcompat.NormalizeMarkdownMermaidFences(ans)
	b.WriteString(ans)
	b.WriteString("\n")
	return b.String()
}

type dumpTextLabels struct {
	Question         string
	Answer           string
	RuntimeArtifacts string
	Kind             string
	Source           string
	Size             string
	Detail           string
	Attachment       string
	Empty            string
}

func dumpLabels(lang string) dumpTextLabels {
	if strings.EqualFold(strings.TrimSpace(lang), "en") {
		return dumpTextLabels{
			Question:         "Question",
			Answer:           "Answer",
			RuntimeArtifacts: "Runtime Artifacts",
			Kind:             "kind",
			Source:           "source",
			Size:             "size",
			Detail:           "detail",
			Attachment:       "attachment",
			Empty:            "(empty)",
		}
	}
	return dumpTextLabels{
		Question:         "问题",
		Answer:           "回答",
		RuntimeArtifacts: "运行时附件",
		Kind:             "类型",
		Source:           "来源",
		Size:             "大小",
		Detail:           "详情",
		Attachment:       "附件",
		Empty:            "(空)",
	}
}

func RuntimeArtifactsFromAttachment(kind, body string) []RuntimeArtifact {
	kind = strings.TrimSpace(kind)
	body = strings.TrimSpace(body)
	if kind == "" || body == "" {
		return nil
	}
	segments := attachmentSegments(body)
	out := make([]RuntimeArtifact, 0, len(segments))
	for _, segment := range segments {
		out = append(out, runtimeArtifactsForSegment(kind, segment.source, segment.body)...)
	}
	return out
}

// RuntimeArtifactsFromRequest reports explicit runtime artifact paths written in
// the user request. This is a transparency surface for terminal/dump UX only:
// it does not classify intent or decide whether current source evidence is
// required. Path-shaped tokens are accepted by canonical suffix/type helpers or
// by content sniffing when the path has an explicit locator and is readable.
func RuntimeArtifactsFromRequest(request string) []RuntimeArtifact {
	request = strings.TrimSpace(request)
	if request == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []RuntimeArtifact
	add := func(raw string) {
		source := normalizeRequestRuntimeArtifactPath(raw)
		if source == "" || seen[source] {
			return
		}
		kind, resolved := runtimeArtifactKindForRequestPath(source)
		if kind == "" {
			return
		}
		seen[source] = true
		out = append(out, runtimeArtifactsForRequestPath(kind, source, resolved)...)
	}
	for _, token := range requestPathTokenRE.FindAllString(request, -1) {
		add(token)
	}
	return out
}

func MergeRuntimeArtifacts(groups ...[]RuntimeArtifact) []RuntimeArtifact {
	seen := map[string]bool{}
	var out []RuntimeArtifact
	for _, group := range groups {
		for _, artifact := range group {
			kind := strings.TrimSpace(artifact.Kind)
			source := strings.TrimSpace(artifact.Source)
			if kind == "" && source == "" {
				continue
			}
			key := strings.ToLower(kind + "\x00" + source)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, artifact)
		}
	}
	return out
}

func normalizeRequestRuntimeArtifactPath(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "`\"'<> \t\r\n，。；;、()[]{}")
	s = strings.TrimSuffix(s, ":")
	s = strings.ReplaceAll(s, "\\", "/")
	return strings.TrimSpace(s)
}

func runtimeArtifactKindForRequestPath(path string) (string, string) {
	path = normalizeRequestRuntimeArtifactPath(path)
	if path == "" {
		return "", ""
	}
	if kind := types.RuntimeArtifactPathKind(path); kind != "" {
		return kind, resolveRequestRuntimeArtifactPath(path)
	}
	if !requestPathHasExplicitLocator(path) {
		return "", ""
	}
	resolved := resolveRequestRuntimeArtifactPath(path)
	if resolved == "" {
		return "", ""
	}
	if requestPathContentLooksLikeRuntimeArtifact(resolved) {
		return "trace", resolved
	}
	return "", ""
}

func requestPathHasExplicitLocator(path string) bool {
	path = normalizeRequestRuntimeArtifactPath(path)
	return filepath.IsAbs(path) ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, "~/") ||
		strings.ContainsAny(path, `/\`)
}

func resolveRequestRuntimeArtifactPath(path string) string {
	path = normalizeRequestRuntimeArtifactPath(path)
	if path == "" {
		return ""
	}
	resolved := path
	if strings.HasPrefix(resolved, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			resolved = filepath.Join(home, strings.TrimPrefix(resolved, "~/"))
		}
	}
	if !filepath.IsAbs(resolved) {
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			resolved = filepath.Join(cwd, resolved)
		}
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return ""
	}
	return resolved
}

func requestPathContentLooksLikeRuntimeArtifact(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	if n <= 0 {
		return false
	}
	raw := string(buf[:n])
	lower := strings.ToLower(raw)
	return strings.HasPrefix(raw, "PERFILE2") ||
		strings.Contains(raw, "perf_sample:") ||
		strings.Contains(raw, "sched_switch:") ||
		strings.Contains(raw, "tracing_mark_write:") ||
		strings.Contains(raw, "# tracer:") ||
		(strings.Contains(lower, `"artifacts"`) && strings.Contains(lower, `"tracebundle"`))
}

func runtimeArtifactsForRequestPath(kind, source, resolved string) []RuntimeArtifact {
	body, bytes := requestRuntimeArtifactBodyAndSize(resolved)
	if strings.EqualFold(kind, "trace") {
		out := runtimeArtifactsForSegment("trace", source, body)
		if len(out) > 0 {
			out[0].Detail = appendDetail(out[0].Detail, "referenced in request")
			if bytes > 0 {
				out[0].Bytes = bytes
			}
			return out
		}
	}
	detail := "referenced in request"
	if strings.EqualFold(kind, "log") {
		detail = appendDetail(detail, "runtime log")
	}
	return []RuntimeArtifact{{
		Kind:   kind,
		Source: source,
		Bytes:  bytes,
		Detail: detail,
	}}
}

func requestRuntimeArtifactBodyAndSize(resolved string) (string, int) {
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return "", 0
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", 0
	}
	bytes := safeInt64ToInt(info.Size())
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", bytes
	}
	return string(data), bytes
}

type attachmentSegment struct {
	source string
	body   string
}

func attachmentSegments(body string) []attachmentSegment {
	var out []attachmentSegment
	current := &attachmentSegment{source: "(inline)"}
	appendCurrent := func() {
		if current == nil {
			return
		}
		if current.source == "(inline)" && strings.TrimSpace(current.body) == "" {
			return
		}
		out = append(out, *current)
	}
	for _, line := range strings.Split(body, "\n") {
		if source, ok := strings.CutPrefix(strings.TrimSpace(line), "# codrax-source: "); ok {
			appendCurrent()
			current = &attachmentSegment{source: strings.TrimSpace(source)}
			continue
		}
		if current != nil {
			if current.body != "" {
				current.body += "\n"
			}
			current.body += line
		}
	}
	appendCurrent()
	return out
}

func runtimeArtifactsForSegment(kind, source, body string) []RuntimeArtifact {
	base := runtimeArtifactForSegment(kind, source, body)
	if !strings.EqualFold(kind, "trace") || base.Kind != "tracebundle" {
		return []RuntimeArtifact{base}
	}
	bundle, ok := parseTraceBundleMetadata(body)
	if !ok {
		return []RuntimeArtifact{base}
	}
	if bundle.Version != "" {
		base.Detail = appendDetail(base.Detail, "version="+bundle.Version)
	}
	if detail := traceBundleProviderDetail(bundle.TraceDecisions); detail != "" {
		base.Detail = appendDetail(base.Detail, detail)
	}
	if detail := traceBundleCoverageDetail("trace_db_coverage", bundle.TraceDBCoverage); detail != "" {
		base.Detail = appendDetail(base.Detail, detail)
	}
	if detail := traceBundleCoverageDetail("trace_coverage", bundle.TraceCoverage); detail != "" {
		base.Detail = appendDetail(base.Detail, detail)
	}
	if len(bundle.Caveats) > 0 {
		base.Detail = appendDetail(base.Detail, "caveats="+joinDetailList(bundle.Caveats, 3))
	}
	out := []RuntimeArtifact{base}
	seen := map[string]bool{}
	addBundleArtifact := func(a traceBundleReportArtifact) {
		path := strings.TrimSpace(a.Path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		detail := ""
		if a.Converter != "" {
			detail = appendDetail(detail, "converter="+a.Converter)
		}
		if a.PluginName != "" {
			detail = appendDetail(detail, "plugin="+a.PluginName)
		}
		if len(a.Caveats) > 0 {
			detail = appendDetail(detail, "caveats="+joinDetailList(a.Caveats, 3))
		}
		out = append(out, RuntimeArtifact{
			Kind:   firstNonEmpty(strings.TrimSpace(a.Type), "artifact"),
			Source: path,
			Bytes:  safeInt64ToInt(a.Bytes),
			Detail: detail,
		})
	}
	if strings.TrimSpace(bundle.Systrace) != "" {
		addBundleArtifact(traceBundleReportArtifact{Type: "systrace", Path: bundle.Systrace})
	}
	for _, artifact := range bundle.Artifacts {
		addBundleArtifact(artifact)
	}
	return out
}

func runtimeArtifactForSegment(kind, source, body string) RuntimeArtifact {
	artifactKind := kind
	detail := ""
	if strings.EqualFold(kind, "trace") {
		artifactKind, detail = traceArtifactKindAndDetail(source, body)
	}
	if strings.EqualFold(kind, "log") {
		detail = "runtime log"
	}
	return RuntimeArtifact{
		Kind:   artifactKind,
		Source: source,
		Bytes:  len([]byte(body)),
		Detail: detail,
	}
}

type traceBundleReportMetadata struct {
	Version         string                           `json:"version"`
	InputPath       string                           `json:"input_path"`
	Systrace        string                           `json:"systrace"`
	Artifacts       []traceBundleReportArtifact      `json:"artifacts"`
	TraceDecisions  []traceBundleReportTraceDecision `json:"trace_provider_decisions"`
	TraceDBCoverage []traceBundleReportTraceCoverage `json:"trace_db_coverage"`
	TraceCoverage   []traceBundleReportTraceCoverage `json:"trace_coverage"`
	Caveats         []string                         `json:"caveats"`
}

type traceBundleReportArtifact struct {
	Type          string   `json:"type"`
	Path          string   `json:"path"`
	Bytes         int64    `json:"bytes"`
	Converter     string   `json:"converter"`
	PluginName    string   `json:"plugin_name"`
	PluginVersion string   `json:"plugin_version"`
	Caveats       []string `json:"caveats"`
}

type traceBundleReportTraceDecision struct {
	ProviderKind    string `json:"provider_kind"`
	ProviderName    string `json:"provider_name"`
	EngineMode      string `json:"engine_mode"`
	Selected        bool   `json:"selected"`
	Attempted       bool   `json:"attempted"`
	Succeeded       bool   `json:"succeeded"`
	Fallback        bool   `json:"fallback"`
	TraceQueryReady bool   `json:"trace_query_ready"`
	Reason          string `json:"reason"`
	Caveat          string `json:"caveat"`
}

type traceBundleReportTraceCoverage struct {
	Family      string `json:"family"`
	Table       string `json:"table"`
	Found       bool   `json:"found"`
	RowsRead    int    `json:"rows_read"`
	RowsEmitted int    `json:"rows_emitted"`
	Skipped     string `json:"skipped"`
	Error       string `json:"error"`
}

func parseTraceBundleMetadata(body string) (traceBundleReportMetadata, bool) {
	var bundle traceBundleReportMetadata
	if err := json.Unmarshal([]byte(body), &bundle); err != nil {
		return traceBundleReportMetadata{}, false
	}
	if strings.TrimSpace(bundle.Systrace) == "" && len(bundle.Artifacts) == 0 {
		return traceBundleReportMetadata{}, false
	}
	return bundle, true
}

func traceBundleProviderDetail(decisions []traceBundleReportTraceDecision) string {
	for _, decision := range decisions {
		if !decision.Selected && !decision.Attempted {
			continue
		}
		provider := firstNonEmpty(decision.ProviderName, decision.ProviderKind)
		if provider == "" {
			continue
		}
		detail := "trace_provider=" + provider
		if decision.EngineMode != "" {
			detail = appendDetail(detail, "engine="+decision.EngineMode)
		}
		detail = appendDetail(detail, fmt.Sprintf("trace_query_ready=%t", decision.TraceQueryReady))
		detail = appendDetail(detail, fmt.Sprintf("succeeded=%t", decision.Succeeded))
		if decision.Fallback {
			detail = appendDetail(detail, "fallback=true")
		}
		if decision.Reason != "" {
			detail = appendDetail(detail, "reason="+compactDetailValue(decision.Reason))
		}
		if decision.Caveat != "" {
			detail = appendDetail(detail, "caveat="+compactDetailValue(decision.Caveat))
		}
		return detail
	}
	return ""
}

func traceBundleCoverageDetail(label string, rows []traceBundleReportTraceCoverage) string {
	if len(rows) == 0 {
		return ""
	}
	const limit = 3
	parts := make([]string, 0, limit+1)
	for _, row := range rows {
		if len(parts) >= limit {
			break
		}
		name := firstNonEmpty(row.Family, row.Table, "unknown")
		if row.Family != "" && row.Table != "" {
			name = row.Family + "/" + row.Table
		}
		item := fmt.Sprintf("%s found=%t", compactDetailValue(name), row.Found)
		if row.RowsRead != 0 {
			item += fmt.Sprintf(" rows=%d", row.RowsRead)
		}
		if row.RowsEmitted != 0 {
			item += fmt.Sprintf(" emitted=%d", row.RowsEmitted)
		}
		if row.Skipped != "" {
			item += " skipped=" + compactDetailValue(row.Skipped)
		}
		if row.Error != "" {
			item += " error=" + compactDetailValue(row.Error)
		}
		parts = append(parts, item)
	}
	if len(rows) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(rows)-limit))
	}
	return label + "=" + strings.Join(parts, "; ")
}

func joinDetailList(items []string, limit int) string {
	var clean []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			clean = append(clean, item)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	if limit <= 0 || len(clean) <= limit {
		return strings.Join(clean, "; ")
	}
	return strings.Join(clean[:limit], "; ") + fmt.Sprintf("; +%d more", len(clean)-limit)
}

func safeInt64ToInt(v int64) int {
	if v <= 0 {
		return 0
	}
	maxInt := int64(^uint(0) >> 1)
	if v > maxInt {
		return int(maxInt)
	}
	return int(v)
}

func traceArtifactKindAndDetail(source, body string) (string, string) {
	p := strings.ToLower(strings.TrimSpace(source))
	detail := "runtime trace"
	kind := "trace"
	switch {
	case strings.HasSuffix(p, ".tracebundle.json"):
		kind = "tracebundle"
		detail = "trace bundle metadata"
	case strings.HasSuffix(p, ".perftrace"):
		kind = "perftrace"
		detail = "perf sample text"
	case strings.HasSuffix(p, ".perf.data") || filepath.Base(p) == "perf.data":
		kind = "perf_data"
		detail = "raw perf.data sidecar"
	}
	if strings.Contains(body, "source=raw_perfdata_fallback") {
		detail = appendDetail(detail, "source=raw_perfdata_fallback")
	}
	if strings.Contains(body, "symbolization_status=unsymbolized") {
		detail = appendDetail(detail, "symbolization_status=unsymbolized")
	}
	if strings.Contains(body, "sample_kind=off_cpu") {
		detail = appendDetail(detail, "sample_kind=off_cpu")
	}
	if strings.Contains(body, "cpu_known=false") {
		detail = appendDetail(detail, "cpu_known=false")
	}
	if strings.Contains(body, "clock_confidence=assumed") {
		detail = appendDetail(detail, "clock_confidence=assumed")
	} else if strings.Contains(body, "clock_confidence=unknown") {
		detail = appendDetail(detail, "clock_confidence=unknown")
	}
	if strings.Contains(body, "perf_sample:") && kind == "trace" {
		kind = "perftrace"
		detail = appendDetail("perf sample text", "inline perf_sample rows")
	}
	return kind, detail
}

func appendDetail(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}
	return base + "; " + extra
}

func compactDetailValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.Join(strings.Fields(value), "_")
}

func escapeMarkdownTableCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return ""
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// BuildHTML renders the already-composed markdown dump body into a
// self-contained HTML page. The source markdown remains the canonical output
// artifact; callers use this only for user-facing browser convenience.
func BuildHTML(title, markdownBody string) (string, error) {
	return preview.RenderStandaloneMarkdownHTML(title, []byte(markdownBody))
}

// HTMLPathForMarkdown returns the sibling HTML path for a markdown dump path.
func HTMLPathForMarkdown(markdownPath string) string {
	markdownPath = strings.TrimSpace(markdownPath)
	if markdownPath == "" {
		return ""
	}
	ext := filepath.Ext(markdownPath)
	if strings.EqualFold(ext, Ext) {
		return strings.TrimSuffix(markdownPath, ext) + HTMLExt
	}
	return markdownPath + HTMLExt
}

// HumanBytes formats a byte count for the attachment footnote.
// Coarse-grained on purpose: the dump line is informational, not a
// precise measurement.
func HumanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// PruneDir keeps the most-recent max-1 *.md files under dir (by mtime),
// reserving one slot for the incoming write. max <= 0 disables pruning.
// Matching .html siblings are removed with their .md files, and orphaned
// canonical .html dumps are cleaned on the same pass so the two artifact types
// keep the same retention shape.
func PruneDir(dir string, max int) {
	if max <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type entry struct {
		path string
		mod  time.Time
	}
	files := make([]entry, 0, len(entries))
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{
			path: filepath.Join(dir, e.Name()),
			mod:  info.ModTime(),
		})
	}
	pruneOrphanHTMLDumps(dir, entries, names)
	keepExisting := max - 1
	if keepExisting < 0 {
		keepExisting = 0
	}
	if len(files) <= keepExisting {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].path < files[j].path
		}
		return files[i].mod.Before(files[j].mod)
	})
	for i := 0; i < len(files)-keepExisting; i++ {
		if err := os.Remove(files[i].path); err != nil {
			logging.Warning("[output_dump] prune %s failed: %v", files[i].path, err)
		}
		htmlPath := HTMLPathForMarkdown(files[i].path)
		if htmlPath != "" {
			if err := os.Remove(htmlPath); err != nil && !os.IsNotExist(err) {
				logging.Warning("[output_dump] prune %s failed: %v", htmlPath, err)
			}
		}
	}
}

func pruneOrphanHTMLDumps(dir string, entries []os.DirEntry, names map[string]bool) {
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, HTMLExt) || !isCanonicalDumpName(name, HTMLExt) {
			continue
		}
		mdName := strings.TrimSuffix(name, HTMLExt) + Ext
		if names[mdName] {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logging.Warning("[output_dump] prune %s failed: %v", path, err)
		}
	}
}

func isCanonicalDumpName(name, ext string) bool {
	if !strings.HasSuffix(name, ext) {
		return false
	}
	stem := strings.TrimSuffix(name, ext)
	if len(stem) < len("20060102-150405.000-1") {
		return false
	}
	if !allDigits(stem[0:8]) || stem[8] != '-' ||
		!allDigits(stem[9:15]) || stem[15] != '.' ||
		!allDigits(stem[16:19]) || stem[19] != '-' {
		return false
	}
	return allDigits(stem[20:])
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
