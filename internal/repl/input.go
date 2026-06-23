// Interactive input helpers for the REPL.
//
// The production TTY path uses native_input.go so the terminal's real cursor
// stays at the editing position; this is required for CJK IME candidate windows
// to follow the cursor. The Bubble Tea model in this file remains as a fallback
// for environments where raw native input cannot be started, and as the pure
// logic home for paste folding / history / slash suggestions.
//
// Responsibilities beyond what bubbles/textinput gives us out of the box:
//
//  1. Up/Down history navigation (seeded from memory.Store).
//  2. Reject Enter on whitespace-only / non-printable-only buffers.
//  3. Slash-command suggestion panel when the buffer starts with "/".
//  4. Bracketed-paste folding: a multi-line or long single-line paste
//     (≥ DefaultPasteFoldMinChars runes, configurable) is replaced
//     inline by a `[Pasted text #N +L lines +C chars]` placeholder
//     token, and the textinput widget treats the whole token as one
//     atomic unit (cursor moves past it, Backspace deletes it whole).
//     On submit, tokens expand back to the original pastes.
//
// Cross-platform: bubbletea handles Windows console, Unix termios, and
// CJK widths; no OS-specific imports belong here.
package repl

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// slashCommands is the canonical autocomplete surface. Kept in sync
// with replCommandAliases' target set in internal/types via the
// TestSlashCommandsMatchCanonicalRegistry drift guard. The input
// layer shows top-level command suggestions for bare slash tokens
// and named subcommand suggestions after commands like "/htrace ".
// slashCommand carries autocomplete + /help metadata for one
// /command. Help strings are bilingual: HelpEn shows when
// --lang=en, HelpZh shows for everything else (zh-default
// convention shared with banner / messages.go). Adding a new
// command: fill BOTH variants — leaving HelpZh empty will fall
// back to HelpEn at render time, but that defeats the bilingual
// contract.
//
// Subs is the optional sub-syntax surface — subcommands and named
// flags that the parent command accepts. /help all renders these
// indented under the parent so a user can discover that, e.g.,
// `/plan list` exists, not just `/plan`. Empty Subs means "this
// command takes no subcommands". The default /help intentionally
// stays concise and task-first.
type slashCommand struct {
	Name   string
	HelpEn string
	HelpZh string
	Subs   []slashSubcommand
}

// Help returns the language-appropriate help string. zh-default;
// only `en` flips to English.
func (c slashCommand) Help(lang string) string {
	if isZh(lang) && c.HelpZh != "" {
		return c.HelpZh
	}
	return c.HelpEn
}

// slashSubcommand describes one accepted second-token form of a
// parent slash command — either a positional subcommand
// ("/plan list") or a recognised flag ("/merge --include-failed").
// Bilingual help mirrors slashCommand.
type slashSubcommand struct {
	Name   string
	HelpEn string
	HelpZh string
}

// Help returns the language-appropriate sub help string.
func (s slashSubcommand) Help(lang string) string {
	if isZh(lang) && s.HelpZh != "" {
		return s.HelpZh
	}
	return s.HelpEn
}

type slashSuggestion struct {
	display  string
	insert   string
	help     string
	needsArg bool
	isSub    bool
}

func slashSuggestionsForValue(value, lang string) []slashSuggestion {
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\r\n") {
		return nil
	}
	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		return slashSubSuggestions(value, idx, lang)
	}
	out := make([]slashSuggestion, 0, len(slashCommands))
	for _, c := range slashCommands {
		if strings.HasPrefix(c.Name, value) {
			out = append(out, slashSuggestion{
				display:  c.Name,
				insert:   c.Name,
				help:     c.Help(lang),
				needsArg: needsArg(c.Name),
			})
		}
	}
	return out
}

func slashSubSuggestions(value string, sep int, lang string) []slashSuggestion {
	base := value[:sep]
	rest := strings.TrimLeft(value[sep+1:], " \t")
	if strings.ContainsAny(rest, " \t") {
		return nil
	}
	cmd, ok := slashCommandByName(base)
	if !ok {
		return nil
	}
	subs := cmd.Subs
	if len(subs) == 0 && base == "/atrace" {
		if htrace, ok := slashCommandByName("/htrace"); ok {
			subs = htrace.Subs
		}
	}
	out := make([]slashSuggestion, 0, len(subs))
	for _, sub := range subs {
		syntax := strings.TrimSpace(sub.Name)
		fields := strings.Fields(syntax)
		if len(fields) == 0 {
			continue
		}
		token := fields[0]
		if strings.HasPrefix(token, "<") || !strings.HasPrefix(token, rest) {
			continue
		}
		out = append(out, slashSuggestion{
			display:  base + " " + syntax,
			insert:   base + " " + token,
			help:     sub.Help(lang),
			needsArg: slashSubNeedsArg(syntax),
			isSub:    true,
		})
	}
	return out
}

func slashCommandByName(name string) (slashCommand, bool) {
	for _, c := range slashCommands {
		if c.Name == name {
			return c, true
		}
	}
	return slashCommand{}, false
}

func slashSubNeedsArg(syntax string) bool {
	return strings.Contains(syntax, "<") || strings.Contains(syntax, "[")
}

// slashCommands is the canonical /help table. Order is significant:
// helpLines (messages.go) inserts a "── Write-mode commands ──"
// header before the first entry that isWriteModeCommand classifies
// as write-mode, then renders every subsequent entry under that
// header. To prevent non-write commands from being mis-grouped, ALL
// non-write commands MUST appear ABOVE the first write entry. The
// 2026-05-08 ordering audit fixed a regression where /branch /cancel
// /env /mermaid /repos /version /exit /quit had drifted below the
// /mode boundary and rendered under the write-mode header.
var slashCommands = []slashCommand{
	// ── Universal core: session / memory / display ───────────────
	{
		Name:   "/help",
		HelpEn: "show concise help; use /help all for every command",
		HelpZh: "显示精简帮助;用 /help all 查看完整命令",
		Subs: []slashSubcommand{
			{"all", "show every command and subcommand", "显示所有命令和子命令"},
		},
	},
	{Name: "/history", HelpEn: "show recent turns", HelpZh: "显示最近会话"},
	{Name: "/compact", HelpEn: "compact memory", HelpZh: "压缩 memory(整理索引,腾出空间)"},
	{Name: "/clear", HelpEn: "wipe conversation memory", HelpZh: "清空会话 memory"},
	// ── Read-mode input / attach / pipeline-bypass ───────────────
	{
		Name:   "/log",
		HelpEn: "attach a runtime log (no arg = paste mode)",
		HelpZh: "附加运行时日志(无参数 = 粘贴模式)",
		Subs: []slashSubcommand{
			{"<path>", "load file as the attached log (replaces any prior)",
				"以指定文件为附加日志(覆盖之前的)"},
			{"append <path>", "append <path> to the existing attached log with a `# codrax-source:` boundary header",
				"在已有附加日志后追加 <path>,自动加 `# codrax-source:` 边界头"},
			{"show", "print the first 20 lines + total bytes of the attached log",
				"打印附加日志前 20 行 + 总字节"},
			{"clear", "drop the attached log (does not touch conversation memory)",
				"清除附加日志(不影响对话记忆)"},
		},
	},
	{
		Name:   "/htrace",
		HelpEn: "attach or convert an ftrace-compatible trace (HiTrace / atrace / systrace / perfetto)",
		HelpZh: "附加或转换 trace(HiTrace / atrace / systrace / perfetto)",
		Subs: []slashSubcommand{
			{"<path>", "load file as the attached trace", "以指定文件为附加 trace"},
			{"append <path>", "append <path> to the existing attached trace", "追加 <path> 到已有附加 trace"},
			{"convert <binary> [out.systrace]", "convert binary Harmony/OpenHarmony HiTrace to text systrace; does not attach by default",
				"把二进制 Harmony/OpenHarmony HiTrace 转成文本 systrace;默认不自动附加"},
			{"tools-status", "inspect trace_streamer, trace engine, and sys parity gate status",
				"查看 trace_streamer、trace engine 和 sys parity gate 状态"},
			{"show", "print the first 800 bytes of the attached trace", "打印附加 trace 前 800 字节"},
			{"clear", "drop the attached trace", "清除附加 trace"},
		},
	},
	{
		Name:   "/atrace",
		HelpEn: "alias of /htrace — Android atrace / systrace spelling (same subcommands)",
		HelpZh: "/htrace 的别名 — Android atrace / systrace 拼法(子命令完全相同)",
	},
	{Name: "/paste", HelpEn: "capture a paste when bracketed paste is stripped (SSH / tmux)",
		HelpZh: "粘贴模式 — 适用于 SSH / tmux 吞掉 bracketed paste 的环境"},
	{
		Name:   "/chat",
		HelpEn: "reply without invoking the analysis pipeline",
		HelpZh: "闲聊回复,不走分析流水线",
		Subs: []slashSubcommand{
			{"<message>", "the message to send (single line; bypasses repo analysis + LLM tool calls)",
				"要发送的消息(单行;绕过仓库分析与 LLM 工具调用)"},
		},
	},
	// ── Universal repo / runtime ops ─────────────────────────────
	{
		Name:   "/repos",
		HelpEn: "show multi-repo topology + active sub-repo state",
		HelpZh: "查看多仓拓扑 + active 子仓状态",
		Subs: []slashSubcommand{
			{"", "list discovered sub-repos (default)", "列出已发现的子仓(默认)"},
			{"focus <slug-or-path>", "pin a sub-repo into the active set across turns (accepts the slug field or the path column from /repos output)",
				"把子仓固定到 active 集合(跨 turn;可以传 slug,也可以传 /repos 列表第一列的相对路径)"},
			{"unfocus [slug-or-path]", "release a focus pin (no arg = release all; accepts slug or path)",
				"取消固定(无参数 = 全部取消;可以传 slug 或路径)"},
			{"refresh", "force re-discover the parent topology",
				"强制重新探测父目录拓扑"},
			{"cap <N>", "session-local override of multi_repo_max_active",
				"会话级覆盖 multi_repo_max_active"},
		},
	},
	{
		Name:   "/branch",
		HelpEn: "show or switch the current git branch in the repo",
		HelpZh: "查看 / 切换当前仓库的 git 分支",
		Subs: []slashSubcommand{
			{"<name>", "git checkout <name> in the main repo (use `/branch -b <name>` to create + switch)",
				"在主仓上 `git checkout <name>`(用 `/branch -b <name>` 创建并切换)"},
		},
	},
	{
		Name:   "/cancel",
		HelpEn: "cancel the in-flight Run. TTY mode: use Ctrl+C (the input box is closed during Run). Pipe / scripted stdin: a `/cancel` line on stdin triggers cancel.",
		HelpZh: "取消正在执行的 Run。TTY 交互模式按 Ctrl+C(运行期输入框关闭,/cancel 输不进去);管道/脚本输入(stdin 重定向)发一行 /cancel 触发。",
	},
	{
		Name:   "/env",
		HelpEn: "show the probed environment + run env_recommend explain on a stderr excerpt",
		HelpZh: "查看探测到的环境画像 / 对 stderr 跑 env_recommend 诊断与推荐",
		Subs: []slashSubcommand{
			{"show", "render the cached EnvFacts (default subcommand)", "渲染当前环境快照(默认子命令)"},
			{"probe", "re-run the environment probe and refresh", "重新探测并刷新缓存"},
			{"explain <stderr>", "run diagnose+recommend on supplied or last-shell stderr", "对参数或最近一次 ! shell 输出跑 diagnose+recommend"},
			{"cache list", "show entries in the disk cache", "列出磁盘缓存条目"},
			{"cache clear", "wipe the disk cache", "清空磁盘缓存"},
			{"stats", "show pipeline counters (calls / stage hits / cache / LLM / docslink)", "查看推荐管线计数器(调用 / 各阶段命中 / 缓存 / LLM / 兜底)"},
			{"stats reset", "zero the pipeline counters", "清零计数器,重新累计窗口"},
		},
	},
	{
		Name:   "/operation",
		HelpEn: "show pending command-operation plan, history, or auto-approval status",
		HelpZh: "查看待处理命令操作计划、历史或自动批准状态",
		Subs: []slashSubcommand{
			{"show", "show the pending operation plan (default)", "查看当前待处理 operation plan(默认)"},
			{"history", "show recent operation plans", "查看最近 operation plan"},
			{"auto", "show low-risk auto-approval status", "查看低风险自动批准状态"},
		},
	},
	{
		Name:   "/mode",
		HelpEn: "show or set the sticky task mode: auto, code, operation, data, or write",
		HelpZh: "显示 / 设置粘滞任务模式:auto、code、operation、data、write",
		Subs: []slashSubcommand{
			{"auto", "use the classifier to choose the route (default)", "使用分类器自动选择路径(默认)"},
			{"code", "force the code/source analysis pipeline", "强制走代码/源码分析路径"},
			{"operation", "force the computer-operation pipeline", "强制走电脑操作路径"},
			{"data", "force the data-processing pipeline", "强制走数据处理路径"},
			{"write", "force write mode (requires write_enabled: true)", "强制走写模式(需要 write_enabled: true)"},
		},
	},
	{Name: "/code", HelpEn: "run one request through code/source analysis", HelpZh: "单次强制走代码/源码分析"},
	{Name: "/op", HelpEn: "run one request through computer operation", HelpZh: "单次强制走电脑操作"},
	{Name: "/data", HelpEn: "run one request through data processing", HelpZh: "单次强制走数据处理"},
	{
		Name:   "/workflow",
		HelpEn: "advanced workflow audit/recovery; write Auto Pilot resumes safe runs automatically",
		HelpZh: "高级 workflow 审计/恢复;写模式 Auto Pilot 会自动续跑安全 run",
		Subs: []slashSubcommand{
			{"show", "inspect current workflow graph, queue, approval, and handoff evidence (default)", "审计当前 workflow 图、队列、审批和 handoff 证据(默认)"},
			{"show <run-id>", "show a saved write workflow run by id", "按 ID 查看已保存的写模式 workflow run"},
			{"list", "list saved write workflow runs", "列出已保存的写模式 workflow run"},
			{"resume [run-id]", "manually resume a saved non-terminal write run for recovery/debug", "为恢复/调试手动恢复指定未终止写模式 run"},
			{"clear [run-id]", "delete an active or saved write workflow run", "删除当前或指定的写模式 workflow run"},
			{"cancel", "cancel the active operation workflow", "取消当前 operation workflow"},
		},
	},
	{
		Name:   "/mermaid",
		HelpEn: "show mermaid render counters (succeeded / fallback / library-rejected / unsupported kinds / gate-blocked)",
		HelpZh: "查看 Mermaid 渲染计数(成功/字符替换/库拒绝/不支持图类/闸门拦截)",
		Subs: []slashSubcommand{
			{"stats", "show counters (default subcommand)", "查看计数(默认子命令)"},
			{"help", "usage line", "显示用法"},
		},
	},
	{Name: "/version", HelpEn: "print build version", HelpZh: "打印构建版本"},
	{Name: "/exit", HelpEn: "leave the REPL", HelpZh: "退出 REPL"},
	{Name: "/quit", HelpEn: "leave the REPL", HelpZh: "退出 REPL"},
	// ── Mode boundary + write-mode workflow ──────────────────────
	// Everything below requires codrax.yaml :: write_enabled: true.
	// helpLines (messages.go) emits a "── Write-mode commands ──"
	// header before the first entry whose name matches
	// isWriteModeCommand — that triggers on /write here.
	{
		Name:   "/write",
		HelpEn: "run one request through write Auto Pilot (requires write_enabled: true)",
		HelpZh: "单次强制走写模式 Auto Pilot(需要 write_enabled: true)",
	},
	{
		Name:   "/plan",
		HelpEn: "inspect / list / clear saved ChangePlans (write-mode)",
		HelpZh: "查看 / 列出 / 清除已保存的 ChangePlan(写模式)",
		Subs: []slashSubcommand{
			{"show", "render the pending plan with per-file unified-diff preview (16 KB total cap)",
				"渲染当前待审 plan,每个文件带 unified-diff 预览(总共 16 KB 上限)"},
			{"show <plan-id>", "render any plan from PlanStore by ID (any status); rebinds pending pointer to it",
				"按 ID 渲染 PlanStore 里任意 plan(任何状态);随后 pending 指针绑到该 plan"},
			{"list", "enumerate every plan in PlanStore with status + size (newest first)",
				"列出 PlanStore 里所有 plan(状态 + 字节,最新的在前)"},
			{"clear", "discard the pending plan without recording a memory turn",
				"丢弃待审 plan,不记入 memory(对比 /reject)"},
			{"clear <plan-id>", "delete a specific plan from PlanStore by ID (also removes the sibling .report.json)",
				"按 ID 从 PlanStore 删除指定 plan(同时删 .report.json 报告)"},
			{"clear --all", "wipe every plan in PlanStore (interactive y/N confirm)",
				"清空 PlanStore 所有 plan(交互式 y/N 二次确认)"},
			{"clear --status=<state>", "wipe every plan with the given status (e.g. rejected, applied_failed, verify_failed; interactive y/N confirm)",
				"清空所有指定状态的 plan(例如 rejected / applied_failed / verify_failed;交互式 y/N 二次确认)"},
		},
	},
	{
		Name:   "/approve",
		HelpEn: "approve the pending write plan or command operation plan",
		HelpZh: "批准待审写模式计划或命令操作计划",
		Subs: []slashSubcommand{
			{"<plan-id>", "write-plan only: target a specific plan by ID instead of the most-recent pending one (positional)",
				"仅写模式 plan:按 ID 指定 plan 而不是默认的最新 pending(位置参数)"},
			{"--plan-id=<id>", "write-plan only: long-flag form of the positional plan-id selector",
				"仅写模式 plan:位置参数 plan-id 的长 flag 形式"},
			{"--merge-to=<branch>", "write-plan only: after a clean apply+verify, immediately /merge into <branch>",
				"仅写模式 plan:apply+verify 通过后立即把改动合到 <branch>(等价 approve + merge 一步)"},
			{"--skip-verify", "write-plan only: apply only, skip the verify stage entirely (no run_tests; plan goes straight to applied) — use when local can't run integration tests",
				"仅写模式 plan:只 apply,完全跳过 verify(不跑 run_tests,plan 直接标 applied)— 本地起不了集成测试时用"},
		},
	},
	{Name: "/reject", HelpEn: "reject the pending write plan or command operation plan (optional free-form reason recorded in memory)",
		HelpZh: "拒绝待审写模式计划或命令操作计划(可选附理由,记入 memory)"},
	{
		Name:   "/verify",
		HelpEn: "re-run verify against an applied plan without re-applying (write mode only)",
		HelpZh: "对已 apply 的 plan 重跑 verify(不再重跑 apply;写模式专属)",
		Subs: []slashSubcommand{
			{"<plan-id>", "target a specific applied plan by ID instead of the pending pointer",
				"按 ID 指定要重跑 verify 的 applied plan(默认用 pending 指针)"},
		},
	},
	{
		Name:   "/worktree",
		HelpEn: "manage preserved worktrees from successful applies (write mode only)",
		HelpZh: "管理成功 apply 时保留的 worktree(写模式专属)",
		Subs: []slashSubcommand{
			{"list", "show every applied plan with a live preserved worktree",
				"列出所有 applied + worktree 仍存活的 plan"},
			{"discard <plan-id>", "remove the named plan's worktree from disk (status stays applied)",
				"删除指定 plan 的 worktree(plan 状态仍为 applied)"},
			{"gc", "manually trigger the ttl + quota reaper (same logic as startup housekeeping)",
				"手动触发 TTL + 配额清理(与启动时一致的逻辑)"},
		},
	},
	{
		Name:   "/merge",
		HelpEn: "fold an applied plan back into the main repo (write mode only)",
		HelpZh: "把已 apply 的 plan 合回主仓(写模式专属)",
		Subs: []slashSubcommand{
			{"--branch=<name>", "create a new branch on main repo + cherry-pick onto it (PR workflow); without this flag, fast-forward into r.branch",
				"在主仓上拉新分支并 cherry-pick(PR 流);不传时 fast-forward 到 r.branch"},
			{"--skip-verify", "also accept unverified plans after operator review; CI is expected to verify later",
				"review 后允许合入未本地验证的 plan;后续交给 CI 验证"},
			{"--include-failed", "also accept verify_failed plans (alias: --force) — for env/CI failures the operator decides to merge anyway",
				"把 verify_failed plan 也纳入候选(别名 --force)— 适合环境/CI 类失败,review 后强合"},
		},
	},
	{
		Name:   "/baseline",
		HelpEn: "inspect the cached pre-apply test snapshots (one per main-repo HEAD)",
		HelpZh: "查看按主仓 HEAD 缓存的应用前测试快照",
		Subs: []slashSubcommand{
			{"list", "enumerate cached entries (default)", "枚举缓存条目(默认)"},
			{"show <sha>", "print the cached test report for the named SHA prefix", "查看指定 SHA 的缓存测试报告"},
			{"clear", "wipe every cached entry (regenerable; no confirm)", "清空所有缓存(可再生,不二次确认)"},
		},
	},
	{
		Name:   "/phase",
		HelpEn: "show write phases: the active workflow run's batches, or a legacy plan group read-only (write mode only)",
		HelpZh: "查看写阶段:活跃 workflow run 的 batch 视图,或只读查看遗留方案组(写模式专属)",
		Subs: []slashSubcommand{
			{"show", "show the active workflow run's batches; falls back to the latest legacy group read-only (default)", "查看活跃 workflow run 的 batch 阶段;无活跃 run 时只读回落最近的遗留方案组(默认)"},
			{"show <group-id>", "show a legacy plan group by id (read-only)", "按 id 只读查看遗留方案组"},
		},
	},
	{
		Name:   "/pitfalls",
		HelpEn: "inspect the per-repo Failure Taxonomy (learned-pitfall cache the planner reads on future plans)",
		HelpZh: "查看本仓库累计的 Failure Taxonomy(planner 在后续 plan 时会读取这些避坑模式)",
		Subs: []slashSubcommand{
			{"list", "enumerate cached patterns (newest first; default subcommand)", "列出已记录的模式(最新在前;默认子命令)"},
			{"show <id>", "show full description + trigger + consequence + scope", "查看指定模式的完整描述 / 触发条件 / 后果 / 适用范围"},
			{"clear", "wipe the cache file (regenerable from future failures)", "清空本地缓存(后续失败时会重新积累)"},
		},
	},
}

// placeholderRE matches a folded-paste token. Group 1 is the id.
var placeholderRE = regexp.MustCompile(`\[Pasted text #(\d+) \+\d+ lines \+\d+ chars\]`)

// DefaultPasteFoldMinChars is the fallback threshold (in Unicode
// runes, not bytes) above which a single-line paste gets folded into
// a placeholder. Multi-line pastes fold unconditionally. Kept as a
// public constant so cmd/root.go can surface the same default in its
// help text, and so tests stay insensitive to yaml plumbing.
//
// Runes, not bytes, because the user-facing setting is in characters
// (pay attention, CJK users: "你好" is 2 chars, 6 bytes). Keeping the
// internal comparison in runes makes the knob's unit match the UI.
const DefaultPasteFoldMinChars = 120

// maxHistoryItems caps how far Up/Down scrolls. Memory.Store.Recent
// is already bounded, but this keeps the model honest even if the
// caller passes a huge slice.
const maxHistoryItems = 100

// inputResult is what the interactive input session returns.
type inputResult struct {
	// expanded is the final text to feed the orchestrator: placeholders
	// are replaced with their original pasted content.
	expanded string
	// display is what the user last saw on screen: placeholders kept
	// as tokens. Used for the "  > …" echo above the answer block.
	display string
	// continues is true when the raw buffer ended with "\" — the
	// outer loop should re-invoke with a continuation prompt.
	continues bool
	// aborted signals Ctrl+C / Ctrl+D / Esc-on-empty.
	aborted bool
}

// inputKeyMap gathers the bindings we intercept before delegating to
// textinput. Keeping them in one place makes cross-platform fiddling
// (e.g. macOS option-arrow quirks) a local edit.
type inputKeyMap struct {
	Submit         key.Binding
	HistoryPrev    key.Binding
	HistoryNext    key.Binding
	Backspace      key.Binding
	DeleteForward  key.Binding
	Tab            key.Binding
	Escape         key.Binding
	Quit           key.Binding
	SuggestionUp   key.Binding
	SuggestionDown key.Binding
	SuggestionTake key.Binding
}

func defaultInputKeys() inputKeyMap {
	return inputKeyMap{
		Submit:         key.NewBinding(key.WithKeys("enter")),
		HistoryPrev:    key.NewBinding(key.WithKeys("up", "ctrl+p")),
		HistoryNext:    key.NewBinding(key.WithKeys("down", "ctrl+n")),
		Backspace:      key.NewBinding(key.WithKeys("backspace", "ctrl+h")),
		DeleteForward:  key.NewBinding(key.WithKeys("delete", "ctrl+d")),
		Tab:            key.NewBinding(key.WithKeys("tab")),
		Escape:         key.NewBinding(key.WithKeys("esc")),
		Quit:           key.NewBinding(key.WithKeys("ctrl+c")),
		SuggestionUp:   key.NewBinding(key.WithKeys("up", "ctrl+p")),
		SuggestionDown: key.NewBinding(key.WithKeys("down", "ctrl+n")),
		SuggestionTake: key.NewBinding(key.WithKeys("tab", "enter")),
	}
}

// inputModel is the Bubble Tea model that backs one readInput call.
type inputModel struct {
	ti                textinput.Model
	keys              inputKeyMap
	history           []string
	histIdx           int      // -1 = fresh draft; 0..len-1 = browsing (0 = newest)
	draft             string   // saved when first entering history
	pastes            []string // pastes[id] → original content
	isContinue        bool     // set for the "…" continuation prompt
	slashSel          int      // selected index in filtered suggestions
	showSuggest       bool
	doneDisplay       string // captured at Submit for echo
	doneExpanded      string
	submitted         bool
	aborted           bool
	continues         bool
	termWidth         int
	pasteFoldMinChars int    // per-instance threshold, in runes
	lang              string // zh | en — drives slash-command help language
	// echoTag is the same sticky-state marker prepended to the prompt
	// (promptStickyTag output). Echoed back ABOVE the inline viewport
	// after submit so the user has a visual confirmation of which
	// pipeline mode this turn ran under — without it, the [task:write]
	// disappears the moment the prompt is replaced by the agent's
	// streaming output and the user can lose track of which mode
	// produced which result. Empty (read mode + no attachments) means
	// no prefix.
	echoTag string
}

// pasteSeed is a single pre-captured paste the caller injects into a
// freshly constructed inputModel. Used by the /paste fallback: each
// seeded turn gets the pasted content as the model's first
// placeholder (id=0) with the cursor positioned after the token so
// the user can keep typing around it. Nil means "normal start".
type pasteSeed struct {
	content string
}

func newInputModel(prompt string, history []string, isContinue bool, w, foldMinChars int, seed *pasteSeed, lang string) *inputModel {
	ti := textinput.New()
	ti.Prompt = prompt + " "
	ti.PromptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).Bold(true)
	ti.Cursor.Style = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51"))
	// Disable textinput's own suggestion path — we run our own.
	ti.ShowSuggestions = false
	// Remove Up/Down from textinput's keymap so they propagate to us
	// as history nav instead of scrolling its built-in suggestion ring.
	ti.KeyMap.PrevSuggestion = key.NewBinding(key.WithDisabled())
	ti.KeyMap.NextSuggestion = key.NewBinding(key.WithDisabled())
	ti.KeyMap.AcceptSuggestion = key.NewBinding(key.WithDisabled())
	// Let textinput scroll horizontally if the line exceeds width. Width is a
	// terminal cell budget, not a rune count: sticky focus tags can contain CJK
	// or emoji, and under-counting them lets the input row overflow before
	// bubbles/textinput gets a chance to keep the cursor in view.
	promptWidth := inputPromptDisplayWidth(ti.Prompt)
	if w > promptWidth+8 {
		ti.Width = w - promptWidth - 2
	}
	ti.Focus()

	// Freshest turn first → pressing Up once yields the most recent.
	hist := historyReversed(history, maxHistoryItems)

	if foldMinChars <= 0 {
		foldMinChars = DefaultPasteFoldMinChars
	}

	m := &inputModel{
		ti:                ti,
		keys:              defaultInputKeys(),
		history:           hist,
		histIdx:           -1,
		isContinue:        isContinue,
		termWidth:         w,
		pasteFoldMinChars: foldMinChars,
		lang:              lang,
	}
	if seed != nil && seed.content != "" {
		m.injectPlaceholder(seed.content)
	}
	return m
}

// injectPlaceholder folds `content` into placeholder id 0 and seeds
// the textinput with that token followed by a single space so the
// caller lands with the cursor ready for additional prose.
func (m *inputModel) injectPlaceholder(content string) {
	id := len(m.pastes)
	m.pastes = append(m.pastes, content)
	lines := strings.Count(content, "\n") + 1
	chars := utf8.RuneCountInString(content)
	token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]", id, lines, chars)
	val := token + " "
	m.ti.SetValue(val)
	m.ti.SetCursor(utf8.RuneCountInString(val))
}

// historyReversed returns up to cap entries from newest to oldest.
func historyReversed(src []string, cap int) []string {
	out := make([]string, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		s := strings.TrimSpace(src[i])
		if s == "" {
			continue
		}
		out = append(out, src[i])
		if len(out) >= cap {
			break
		}
	}
	return out
}

func (m *inputModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnableBracketedPaste,
		textinput.Blink,
	)
}

// Update dispatches on key events. We intercept submit, history,
// suggestion navigation, and placeholder-atomic editing before
// delegating the rest to the underlying textinput.
func (m *inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		promptWidth := inputPromptDisplayWidth(m.ti.Prompt)
		if msg.Width > promptWidth+8 {
			m.ti.Width = msg.Width - promptWidth - 2
		}
		return m, nil

	case tea.KeyMsg:
		// Bracketed paste is delivered as a KeyRunes msg with Paste=true.
		if msg.Type == tea.KeyRunes && msg.Paste {
			return m, m.handlePaste(string(msg.Runes))
		}

		// When the suggestion panel is visible, it owns Up/Down/Tab/Enter/Esc.
		if m.showSuggest {
			if cmd, handled := m.handleSuggestKey(msg); handled {
				return m, cmd
			}
		}

		// Global keys.
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.aborted = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Escape):
			if m.ti.Value() == "" {
				m.aborted = true
				return m, tea.Quit
			}
			m.ti.SetValue("")
			m.pastes = nil
			m.showSuggest = false
			m.refreshSuggest()
			return m, nil

		case key.Matches(msg, m.keys.Submit):
			return m, m.handleSubmit()

		case key.Matches(msg, m.keys.HistoryPrev):
			if !m.isContinue && len(m.history) > 0 {
				m.historyPrev()
				return m, nil
			}

		case key.Matches(msg, m.keys.HistoryNext):
			if !m.isContinue && len(m.history) > 0 {
				m.historyNext()
				return m, nil
			}

		case key.Matches(msg, m.keys.Backspace):
			if sp, ok := m.spanEndingAt(m.ti.Position()); ok {
				m.deleteSpan(sp)
				m.refreshSuggest()
				return m, nil
			}

		case key.Matches(msg, m.keys.DeleteForward):
			if sp, ok := m.spanStartingAt(m.ti.Position()); ok {
				m.deleteSpan(sp)
				m.refreshSuggest()
				return m, nil
			}

		case key.Matches(msg, m.keys.Tab):
			// Tab outside a visible suggest panel is a no-op (avoid
			// inserting raw \t which trips many terminals).
			return m, nil
		}

		// Delegate to textinput, then repair cursor position if it
		// landed inside a placeholder span.
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		m.snapCursorOutOfSpan(msg)
		m.refreshSuggest()
		return m, cmd
	}

	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func inputPromptDisplayWidth(prompt string) int {
	return lipgloss.Width(prompt)
}

func (m *inputModel) View() string {
	if m.submitted || m.aborted {
		return ""
	}
	out := m.ti.View()
	if m.showSuggest {
		out += "\n" + m.renderSuggestPanel()
	}
	return out
}

// handleSubmit finalises the buffer or silently refuses to submit on
// whitespace/non-printable-only content.
func (m *inputModel) handleSubmit() tea.Cmd {
	raw := m.ti.Value()
	expanded := m.expand(raw)
	if hasUnresolvedPastePlaceholder(expanded) {
		m.ti.SetValue("")
		m.pastes = nil
		m.showSuggest = false
		return nil
	}
	if !hasPrintable(expanded) {
		// Normalise: drop the empty whitespace so the cursor is clean.
		if raw != "" {
			m.ti.SetValue("")
			m.pastes = nil
			m.showSuggest = false
		}
		return nil
	}
	// Detect trailing "\" continuation (on the display buffer, so a
	// backslash inside a folded paste doesn't trigger it).
	display := raw
	continues := false
	if strings.HasSuffix(strings.TrimRight(display, " \t"), "\\") {
		continues = true
		display = strings.TrimRight(display, " \t")
		display = strings.TrimSuffix(display, "\\")
		// Re-expand without the trailing "\" so it doesn't reach dispatch.
		expanded = m.expand(display)
		if hasUnresolvedPastePlaceholder(expanded) {
			m.ti.SetValue("")
			m.pastes = nil
			m.showSuggest = false
			return nil
		}
	}
	m.submitted = true
	m.continues = continues
	m.doneDisplay = display
	m.doneExpanded = expanded
	m.ti.Blur()

	// Persist the echo + divider above the inline viewport, then quit.
	// Skip divider on continuation — the next line's prompt already
	// gives visual separation.
	echoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	divStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	// Echo the EXPANDED form (`[Pasted text #N]` placeholders
	// replaced by their full content) so what the user actually
	// dispatched is what they see in the conversation log. Pre-fix
	// the echo printed the folded placeholder on a single flattened
	// line — handy for editing-time compaction, misleading once
	// dispatched. Multi-line expansion preserves newlines in the
	// echo: line 1 carries the `> ` prompt + (optional) tag, lines
	// 2..N carry an indent so the block reads as one unit.
	echoBody := expanded
	if echoBody == "" {
		echoBody = display
	}
	echoLines := strings.Split(echoBody, "\n")
	cmds := []tea.Cmd{}
	// Print the divider FIRST (above the echo) so it visually marks
	// the boundary between the previous turn's chrome and the new
	// turn's request + output. Pre-2026-04-30 the divider sat
	// BELOW the echo, which produced the inverted-looking layout
	// "echo / divider / output" — the user reported this as
	// breaking visual hierarchy. Skip on continuation: the next
	// line's prompt already gives separation.
	if !continues {
		cmds = append(cmds, tea.Printf("  %s", divStyle.Render("─────────────────────────────────────")))
	}
	for i, ln := range echoLines {
		var rendered string
		if i == 0 {
			rendered = echoStyle.Render("> " + ln)
			if m.echoTag != "" {
				rendered = tagStyle.Render(m.echoTag) + " " + rendered
			}
		} else {
			// Continuation lines inside an unfolded paste — align
			// under the prompt so the block reads visually grouped.
			rendered = echoStyle.Render("  " + ln)
		}
		cmds = append(cmds, tea.Printf("  %s", rendered))
	}
	cmds = append(cmds, tea.Quit)
	return tea.Sequence(cmds...)
}

// handlePaste decides verbatim-vs-folded and mutates the buffer.
// Unit for the threshold is Unicode runes (characters), not bytes,
// so a 60-char Chinese paste and a 60-char ASCII paste both fold.
func (m *inputModel) handlePaste(pasted string) tea.Cmd {
	chars := utf8.RuneCountInString(pasted)
	shouldFold := strings.Contains(pasted, "\n") || chars >= m.pasteFoldMinChars
	if !shouldFold {
		return m.insertAtCursor(pasted)
	}
	id := len(m.pastes)
	m.pastes = append(m.pastes, pasted)
	lines := strings.Count(pasted, "\n") + 1
	token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]", id, lines, chars)
	return m.insertAtCursor(token)
}

// insertAtCursor writes text at the current textinput cursor and keeps
// the cursor at the end of the inserted region.
func (m *inputModel) insertAtCursor(text string) tea.Cmd {
	v := []rune(m.ti.Value())
	pos := m.ti.Position()
	if pos > len(v) {
		pos = len(v)
	}
	ins := []rune(text)
	newV := make([]rune, 0, len(v)+len(ins))
	newV = append(newV, v[:pos]...)
	newV = append(newV, ins...)
	newV = append(newV, v[pos:]...)
	m.ti.SetValue(string(newV))
	m.ti.SetCursor(pos + len(ins))
	m.refreshSuggest()
	return nil
}

// expand replaces every placeholder token with its recorded content.
func (m *inputModel) expand(s string) string {
	return placeholderRE.ReplaceAllStringFunc(s, func(tok string) string {
		sub := placeholderRE.FindStringSubmatch(tok)
		if len(sub) < 2 {
			return tok
		}
		id, err := strconv.Atoi(sub[1])
		if err != nil || id < 0 || id >= len(m.pastes) {
			return tok
		}
		return m.pastes[id]
	})
}

func hasUnresolvedPastePlaceholder(s string) bool {
	return placeholderRE.MatchString(s)
}

// span is a rune-indexed range [start, end) for one placeholder token.
type span struct{ start, end int }

func (m *inputModel) spans() []span {
	v := m.ti.Value()
	var out []span
	for _, loc := range placeholderRE.FindAllStringIndex(v, -1) {
		startRune := utf8.RuneCountInString(v[:loc[0]])
		endRune := startRune + utf8.RuneCountInString(v[loc[0]:loc[1]])
		out = append(out, span{startRune, endRune})
	}
	return out
}

func (m *inputModel) spanAt(pos int) (span, bool) {
	for _, sp := range m.spans() {
		if pos > sp.start && pos < sp.end {
			return sp, true
		}
	}
	return span{}, false
}

func (m *inputModel) spanEndingAt(pos int) (span, bool) {
	for _, sp := range m.spans() {
		if pos == sp.end {
			return sp, true
		}
	}
	return span{}, false
}

func (m *inputModel) spanStartingAt(pos int) (span, bool) {
	for _, sp := range m.spans() {
		if pos == sp.start {
			return sp, true
		}
	}
	return span{}, false
}

func (m *inputModel) deleteSpan(sp span) {
	v := []rune(m.ti.Value())
	if sp.end > len(v) {
		return
	}
	newV := make([]rune, 0, len(v)-(sp.end-sp.start))
	newV = append(newV, v[:sp.start]...)
	newV = append(newV, v[sp.end:]...)
	m.ti.SetValue(string(newV))
	m.ti.SetCursor(sp.start)
}

// snapCursorOutOfSpan is called after textinput processes a key: if
// cursor motion parked us strictly inside a placeholder, nudge to the
// side the motion came from.
func (m *inputModel) snapCursorOutOfSpan(k tea.KeyMsg) {
	sp, ok := m.spanAt(m.ti.Position())
	if !ok {
		return
	}
	switch k.Type {
	case tea.KeyLeft, tea.KeyCtrlB, tea.KeyHome, tea.KeyCtrlA:
		m.ti.SetCursor(sp.start)
	default:
		m.ti.SetCursor(sp.end)
	}
}

// historyPrev and historyNext walk the buffer of prior submissions.
// If a recalled entry contains newlines, fold it into one placeholder
// so the single-line textinput doesn't explode.
func (m *inputModel) historyPrev() {
	if m.histIdx == -1 {
		m.draft = m.ti.Value()
	}
	if m.histIdx+1 >= len(m.history) {
		return
	}
	m.histIdx++
	m.loadHistoryEntry(m.history[m.histIdx])
}

func (m *inputModel) historyNext() {
	if m.histIdx == -1 {
		return
	}
	m.histIdx--
	if m.histIdx == -1 {
		m.ti.SetValue(m.draft)
		m.ti.SetCursor(utf8.RuneCountInString(m.draft))
		return
	}
	m.loadHistoryEntry(m.history[m.histIdx])
}

func (m *inputModel) loadHistoryEntry(entry string) {
	// Replace placeholder state with a fresh slot if the entry is
	// multi-line (a past /log paste or trailing-\ composition).
	if strings.Contains(entry, "\n") {
		id := len(m.pastes)
		m.pastes = append(m.pastes, entry)
		lines := strings.Count(entry, "\n") + 1
		token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]",
			id, lines, utf8.RuneCountInString(entry))
		m.ti.SetValue(token)
		m.ti.SetCursor(utf8.RuneCountInString(token))
	} else {
		m.ti.SetValue(entry)
		m.ti.SetCursor(utf8.RuneCountInString(entry))
	}
	m.showSuggest = false
}

// refreshSuggest recomputes the visibility of the slash-command panel.
func (m *inputModel) refreshSuggest() {
	if m.isContinue {
		m.showSuggest = false
		return
	}
	matches := m.filterSuggestions(m.ti.Value())
	if len(matches) == 0 {
		m.showSuggest = false
		return
	}
	m.showSuggest = true
	if m.slashSel >= len(matches) {
		m.slashSel = 0
	}
}

func (m *inputModel) filterSuggestions(value string) []slashSuggestion {
	return slashSuggestionsForValue(value, m.lang)
}

// handleSuggestKey consumes Up/Down/Tab/Enter/Esc while the panel is
// visible. Returns (cmd, true) when it took the event.
func (m *inputModel) handleSuggestKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	matches := m.filterSuggestions(m.ti.Value())
	switch {
	case key.Matches(msg, m.keys.SuggestionUp):
		if len(matches) > 0 {
			m.slashSel = (m.slashSel - 1 + len(matches)) % len(matches)
		}
		return nil, true
	case key.Matches(msg, m.keys.SuggestionDown):
		if len(matches) > 0 {
			m.slashSel = (m.slashSel + 1) % len(matches)
		}
		return nil, true
	case key.Matches(msg, m.keys.SuggestionTake):
		if len(matches) > 0 && m.slashSel < len(matches) {
			chosen := matches[m.slashSel]
			// Tab accepts and keeps editing. Enter submits commands that
			// are complete, but keeps editing when the selected subcommand
			// still needs a positional argument.
			if msg.Type == tea.KeyEnter {
				next := chosen.insert
				if chosen.isSub && chosen.needsArg {
					next += " "
					m.ti.SetValue(next)
					m.ti.SetCursor(utf8.RuneCountInString(next))
					m.showSuggest = false
					return nil, true
				}
				m.ti.SetValue(next)
				m.ti.SetCursor(utf8.RuneCountInString(next))
				m.showSuggest = false
				return m.handleSubmit(), true
			}
			next := chosen.insert
			if chosen.needsArg {
				next += " "
			}
			m.ti.SetValue(next)
			m.ti.SetCursor(utf8.RuneCountInString(next))
			m.showSuggest = false
			return nil, true
		}
	case key.Matches(msg, m.keys.Escape):
		m.showSuggest = false
		return nil, true
	}
	return nil, false
}

// needsArg reports whether Tab-completing this command should leave
// a trailing space so the user can immediately type arguments (vs.
// commands like /exit that take none). The list includes every
// command whose handler reads a non-empty remainder.
func needsArg(cmd string) bool {
	switch cmd {
	case "/log", "/htrace", "/chat", "/mode", "/plan", "/reject", "/verify", "/worktree", "/merge", "/branch", "/env", "/baseline", "/approve", "/phase", "/repos", "/operation", "/workflow":
		return true
	}
	return false
}

// renderSuggestPanel lays the filtered list below the input.
func (m *inputModel) renderSuggestPanel() string {
	matches := m.filterSuggestions(m.ti.Value())
	if len(matches) == 0 {
		return ""
	}
	selStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	var b strings.Builder
	for i, suggestion := range matches {
		var prefix string
		var nm string
		if i == m.slashSel {
			prefix = selStyle.Render("▸ ")
			nm = selStyle.Render(suggestion.display)
		} else {
			prefix = "  "
			nm = nameStyle.Render(suggestion.display)
		}
		b.WriteString("  ")
		b.WriteString(prefix)
		b.WriteString(nm)
		b.WriteString("  ")
		b.WriteString(helpStyle.Render(suggestion.help))
		if i < len(matches)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// readInputBubble runs one Bubble Tea session. The outer caller uses
// its return (expanded, continues) to build the multi-line buffer.
func (r *REPL) readInputBubble(prompt string, isContinue bool) (inputResult, error) {
	w, _, _ := term.GetSize(0)
	if w <= 0 {
		w = 80
	}
	hist := r.historyStrings()
	var seed *pasteSeed
	if !isContinue && r.pendingPaste != "" {
		seed = &pasteSeed{content: r.pendingPaste}
		r.pendingPaste = "" // single-use; consumed on inject
	}
	model := newInputModel(prompt, hist, isContinue, w, r.pasteFoldMinChars, seed, r.language)
	// echoTag is the same sticky-state marker the Loop assembled for the
	// prompt — re-render it ABOVE the inline viewport on submit so the
	// confirmation line carries the same mode/attachment chrome the user
	// typed against.
	model.echoTag = r.echoTag

	p := tea.NewProgram(model,
		tea.WithOutput(r.out),
	)
	finalM, err := p.Run()
	if err != nil {
		return inputResult{aborted: true}, err
	}
	fm, _ := finalM.(*inputModel)
	if fm == nil {
		return inputResult{aborted: true}, io.EOF
	}
	if fm.aborted {
		return inputResult{aborted: true}, io.EOF
	}
	return inputResult{
		expanded:  fm.doneExpanded,
		display:   fm.doneDisplay,
		continues: fm.continues,
	}, nil
}

// historyStrings returns past user Requests, oldest first, drawn from
// the memory store when present. Folded paste turns store a compact
// display Request plus the expanded RequestForSummary; history recall must
// prefer the expanded form so pressing ↑ never re-submits a dead
// "[Pasted text #N]" placeholder after restart.
func (r *REPL) historyStrings() []string {
	if r.store == nil {
		return nil
	}
	turns := r.store.Recent()
	out := make([]string, 0, len(turns))
	for _, t := range turns {
		req := strings.TrimSpace(t.RequestForSummary)
		if req == "" {
			req = t.Request
		}
		if strings.TrimSpace(req) == "" {
			continue
		}
		if hasUnresolvedPastePlaceholder(req) {
			continue
		}
		out = append(out, req)
	}
	return out
}

// hasPrintable returns true iff the string contains at least one rune
// that is not whitespace and not a control/format/zero-width codepoint.
func hasPrintable(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if !unicode.IsPrint(r) {
			continue
		}
		return true
	}
	return false
}
