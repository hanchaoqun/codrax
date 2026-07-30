package repl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
)

// PIB-4 (pi borrow — pi's issue-analysis gist export + /ir import,
// ledger docs/design/pi_borrow_analysis_20260729.md §7.8): a
// diagnostic turn (request + answer + attached log/trace) exports to a
// transparent flat-directory bundle a colleague can /import to restore
// the attachments and conversation context in their own REPL — the
// missing half-loop of the customer-revisit workflow.

const exportBundleSchemaVersion = 1

type exportBundleManifest struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	RepoRoot      string    `json:"repo_root,omitempty"`
	Branch        string    `json:"branch,omitempty"`
	Language      string    `json:"language,omitempty"`
	TurnID        string    `json:"turn_id,omitempty"`
	// Attachment integrity: sha256 of the payload files as written.
	LogSHA256    string `json:"log_sha256,omitempty"`
	TraceSHA256  string `json:"trace_sha256,omitempty"`
	TraceSource  string `json:"trace_source,omitempty"`
	HasRequest   bool   `json:"has_request,omitempty"`
	HasAnswer    bool   `json:"has_answer,omitempty"`
	CodraxOrigin string `json:"codrax_origin,omitempty"`
}

// handleExportCmd bundles a turn + attachments into
// <runtimeAnchor>/exports/<ts>-bundle/. Bare /export takes the most
// recent turn plus the CURRENT sticky attachments; /export <turn-id>
// reaches any persisted turn by id — attachments are deliberately
// omitted on the by-id form (the sticky lanes belong to the live
// session, not to a historical turn) and the confirmation says so.
func (r *REPL) handleExportCmd(line string) {
	if strings.TrimSpace(r.runtimeAnchor) == "" {
		r.info(exportNoAnchorMsg(r.language))
		return
	}
	turnID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "/export"))
	if turnID != "" {
		r.exportTurnByID(turnID)
		return
	}
	var lastTurn *memory.Turn
	if r.store != nil {
		if recent := r.store.Recent(); len(recent) > 0 {
			turn := recent[len(recent)-1]
			lastTurn = &turn
		}
	}
	if lastTurn == nil && strings.TrimSpace(r.attachedLog) == "" && strings.TrimSpace(r.attachedHitrace) == "" {
		r.info(exportNothingMsg(r.language))
		return
	}
	dir := filepath.Join(r.runtimeAnchor, "exports", time.Now().Format("20060102-150405")+"-bundle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.errorf("export: %v\n", err)
		return
	}
	manifest := exportBundleManifest{
		SchemaVersion: exportBundleSchemaVersion,
		CreatedAt:     time.Now(),
		RepoRoot:      r.repoRoot,
		Branch:        r.branch,
		Language:      r.language,
		CodraxOrigin:  "codrax /export",
	}
	write := func(name, content string) bool {
		if content == "" {
			return true
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			r.errorf("export %s: %v\n", name, err)
			return false
		}
		return true
	}
	if lastTurn != nil {
		manifest.TurnID = lastTurn.ID
		manifest.HasRequest = strings.TrimSpace(lastTurn.Request) != ""
		manifest.HasAnswer = strings.TrimSpace(lastTurn.Response) != ""
		if !write("request.txt", lastTurn.Request) || !write("answer.md", lastTurn.Response) {
			return
		}
	}
	if strings.TrimSpace(r.attachedLog) != "" {
		manifest.LogSHA256 = sha256Hex(r.attachedLog)
		if !write("log.txt", r.attachedLog) {
			return
		}
	}
	if strings.TrimSpace(r.attachedHitrace) != "" {
		manifest.TraceSHA256 = sha256Hex(r.attachedHitrace)
		manifest.TraceSource = r.attachedHitraceSource
		if !write("trace.txt", r.attachedHitrace) {
			return
		}
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		r.errorf("export manifest: %v\n", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), payload, 0o600); err != nil {
		r.errorf("export manifest: %v\n", err)
		return
	}
	r.success(exportDoneMsg(r.language, dir))
}

// exportTurnByID bundles one persisted turn (request + answer only —
// the sticky attachment lanes belong to the live session, not to a
// historical turn, so the by-id form omits them and says so).
func (r *REPL) exportTurnByID(turnID string) {
	if r.store == nil {
		r.info(exportNothingMsg(r.language))
		return
	}
	turn, err := r.store.Turn(turnID)
	if err != nil {
		r.errorf("export: turn %s: %v\n", turnID, err)
		return
	}
	dir := filepath.Join(r.runtimeAnchor, "exports", time.Now().Format("20060102-150405")+"-"+turnID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.errorf("export: %v\n", err)
		return
	}
	manifest := exportBundleManifest{
		SchemaVersion: exportBundleSchemaVersion,
		CreatedAt:     time.Now(),
		RepoRoot:      r.repoRoot,
		Branch:        r.branch,
		Language:      r.language,
		TurnID:        turn.ID,
		HasRequest:    strings.TrimSpace(turn.Request) != "",
		HasAnswer:     strings.TrimSpace(turn.Response) != "",
		CodraxOrigin:  "codrax /export <turn-id>",
	}
	writeFile := func(name, content string) bool {
		if content == "" {
			return true
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			r.errorf("export %s: %v\n", name, err)
			return false
		}
		return true
	}
	if !writeFile("request.txt", turn.Request) || !writeFile("answer.md", turn.Response) {
		return
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		r.errorf("export manifest: %v\n", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), payload, 0o600); err != nil {
		r.errorf("export manifest: %v\n", err)
		return
	}
	r.success(exportTurnDoneMsg(r.language, turn.ID, dir))
}

func exportTurnDoneMsg(lang, turnID, dir string) string {
	if isZh(lang) {
		return "已导出历史轮 " + turnID + " 的 bundle: " + dir + "(不含附件——sticky 附件属于当前会话)"
	}
	return "Exported turn " + turnID + " bundle: " + dir + " (no attachments — sticky lanes belong to the live session)"
}

// handleImportCmd restores a bundle: attachments re-attach through the
// same sticky lanes /log and /htrace use (runner setters included via
// the next dispatch's propagation), and the bundled turn is appended
// to memory so the conversation context continues here.
func (r *REPL) handleImportCmd(line string) {
	dir := strings.TrimSpace(strings.TrimPrefix(line, "/import"))
	if dir == "" {
		r.info(importUsageMsg(r.language))
		return
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		r.errorf("import: %v\n", err)
		return
	}
	var manifest exportBundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		r.errorf("import manifest: %v\n", err)
		return
	}
	if manifest.SchemaVersion != exportBundleSchemaVersion {
		r.errorf("import: unsupported bundle schema %d (this codrax reads %d)\n", manifest.SchemaVersion, exportBundleSchemaVersion)
		return
	}
	if manifest.RepoRoot != "" && manifest.RepoRoot != r.repoRoot {
		r.warn("%s\n", importRepoMismatchMsg(r.language, manifest.RepoRoot, r.repoRoot))
	}
	readOpt := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		return string(data)
	}
	// Attachments: REGFIX-D #17 (sweep) — verify EVERY payload BEFORE
	// attaching ANY of them, and treat a missing hash as a verification
	// failure rather than a waiver. The previous order attached log.txt
	// and only then checked trace.txt, so a bundle tampered in the trace
	// lane left the log attached despite the advertised
	// "a tampered bundle fails loud and attaches nothing" contract; and
	// stripping the sha fields from the manifest bypassed checking
	// entirely.
	logPayload := readOpt("log.txt")
	tracePayload := readOpt("trace.txt")
	if logPayload != "" {
		if manifest.LogSHA256 == "" {
			r.errorf("import: log.txt present but the manifest carries no sha256 — refusing to attach unverifiable payload\n")
			return
		}
		if sha256Hex(logPayload) != manifest.LogSHA256 {
			r.errorf("import: log.txt sha256 mismatch against manifest\n")
			return
		}
	}
	if tracePayload != "" {
		if manifest.TraceSHA256 == "" {
			r.errorf("import: trace.txt present but the manifest carries no sha256 — refusing to attach unverifiable payload\n")
			return
		}
		if sha256Hex(tracePayload) != manifest.TraceSHA256 {
			r.errorf("import: trace.txt sha256 mismatch against manifest\n")
			return
		}
	}
	// Every payload verified — only now do the sticky lanes change.
	if logPayload != "" {
		r.attachedLog = logPayload
	}
	if tracePayload != "" {
		r.attachedHitrace = tracePayload
		r.attachedHitraceSource = manifest.TraceSource
	}
	request := readOpt("request.txt")
	answer := readOpt("answer.md")
	if strings.TrimSpace(request) != "" || strings.TrimSpace(answer) != "" {
		// The imported pair enters memory as a plain turn so
		// BuildContext / /history see the prior thread here.
		r.recordTurn(request, request, answer, memory.KindPipeline)
	}
	r.success(importDoneMsg(r.language, dir, strings.TrimSpace(request) != "", r.attachedLog != "", r.attachedHitrace != ""))
	if strings.TrimSpace(request) != "" {
		r.info(importRequestEchoMsg(r.language, request))
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func exportNoAnchorMsg(lang string) string {
	if isZh(lang) {
		return "/export 需要 .codrax 运行目录(runtime anchor)可用"
	}
	return "/export needs the .codrax runtime anchor"
}

func exportNothingMsg(lang string) string {
	if isZh(lang) {
		return "没有可导出的内容: 本会话还没有已完成的轮次,也没有附件"
	}
	return "Nothing to export: no completed turn and no attachments in this session"
}

func exportDoneMsg(lang, dir string) string {
	if isZh(lang) {
		return "已导出诊断 bundle: " + dir + "(对方用 /import " + dir + " 一键还原)"
	}
	return "Exported diagnostic bundle: " + dir + " (restore anywhere with /import " + dir + ")"
}

func importUsageMsg(lang string) string {
	if isZh(lang) {
		return "用法: /import <bundle目录> — 还原 /export 产出的诊断 bundle"
	}
	return "Usage: /import <bundle-dir> — restore a bundle produced by /export"
}

func importRepoMismatchMsg(lang, want, got string) string {
	if isZh(lang) {
		return fmt.Sprintf("bundle 来自其他仓库(%s ≠ %s),问题与附件仍可复现,但代码锚点可能不匹配", want, got)
	}
	return fmt.Sprintf("bundle came from a different repo (%s ≠ %s); the question and attachments still replay, code anchors may not match", want, got)
}

func importDoneMsg(lang, dir string, hasTurn, hasLog, hasTrace bool) string {
	parts := []string{}
	if hasTurn {
		parts = append(parts, map[bool]string{true: "对话轮次", false: "turn"}[isZh(lang)])
	}
	if hasLog {
		parts = append(parts, map[bool]string{true: "日志附件", false: "log attachment"}[isZh(lang)])
	}
	if hasTrace {
		parts = append(parts, map[bool]string{true: "trace 附件", false: "trace attachment"}[isZh(lang)])
	}
	if isZh(lang) {
		return "已导入 bundle(" + strings.Join(parts, "、") + "): " + dir
	}
	return "Imported bundle (" + strings.Join(parts, ", ") + "): " + dir
}

func importRequestEchoMsg(lang, request string) string {
	oneLine := strings.Join(strings.Fields(request), " ")
	if len(oneLine) > 160 {
		oneLine = oneLine[:160] + "…"
	}
	if isZh(lang) {
		return "原始问题: " + oneLine + "(附件已挂载,可直接追问或重跑)"
	}
	return "Original question: " + oneLine + " (attachments live; ask follow-ups or re-run)"
}
