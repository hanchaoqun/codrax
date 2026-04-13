package normalizer

// zhStopwordRunes is the set of Han characters the normalizer will
// strip from the *edges* of a Han run but not use to reject the run
// entirely. "请解释探索器" → strip 请 → "解释探索器" → further
// extraction may split on translation-table matches if configured.
var zhStopwordRunes = map[rune]bool{
	'请': true, '问': true, '吗': true, '呢': true, '啊': true,
	'的': true, '了': true, '着': true, '吧': true, '嘛': true,
}

func isChineseStopwordRune(r rune) bool { return zhStopwordRunes[r] }

// zhToEn is the bilingual translation table. Keys are maximal Chinese
// phrases the analyzer domain cares about; values are the lowercase
// English canonical surface the alias graph points to.
//
// This table is the normalizer's only domain-specific knowledge
// source. It is kept small on purpose — broader term expansion should
// come from repo-grounded symbol lookups, not from a hand-maintained
// dictionary. Add entries only when they are load-bearing for a real
// regression case.
//
// Review-driven edits (analyzer-v3 B4c):
//   - Removed 解释/停止/验证 — they are concept words that don't help
//     as search keywords and were leaking into explorer hints.
//   - Removed 锁 — a single Han character that extractHanRuns would
//     have filtered anyway (min length 2), so the entry was dead.
//   - Renamed 数据完整 → 数据完整性 so the extractor's maximal-Han
//     run can actually match a naturally-written phrase.
//   - Added探测/证据链/跳数/打分/门控/召回/兜底/线程/幂等 —
//     high-frequency terms that recur throughout this repo's memory
//     index and would otherwise force explorer to re-derive them.
var zhToEn = map[string]string{
	// agent / role
	"探索器": "explorer",
	"分析器": "analyzer",
	"协调器": "orchestrator",
	"终结器": "finalizer",
	"执行器": "executor",
	"规划器": "planner",
	"评审器": "reviewer",
	"实施器": "implementer",
	"验证器": "verifier",

	// process / structure
	"证据":  "evidence",
	"证据链": "evidence_chain",
	"假设":  "hypothesis",
	"任务":  "task",
	"计划":  "plan",
	"风险":  "risk",
	"合同":  "contract",
	"终结":  "finalize",
	"实施":  "implement",
	"链路":  "chain",
	"流水线": "pipeline",
	"阶段":  "stage",
	"场景":  "scenario",
	"模板":  "template",
	"探测":  "probe",
	"门控":  "gate",
	"跳数":  "hop",
	"打分":  "rank",
	"召回":  "recall",
	"兜底":  "fallback",

	// runtime / plumbing
	"配置":  "config",
	"注册":  "register",
	"返回":  "return",
	"调用":  "call",
	"工具":  "tool",
	"记忆":  "memory",
	"会话":  "session",
	"错误":  "error",
	"日志":  "log",
	"重试":  "retry",
	"超时":  "timeout",
	"缓存":  "cache",
	"并发":  "concurrency",
	"线程":  "thread",
	"幂等":  "idempotent",

	// risk dimensions
	"合规":   "compliance",
	"安全":   "security",
	"性能":   "performance",
	"兼容":   "compatibility",
	"数据完整性": "data_integrity",
	"运维":   "ops",
}

// ChineseTranslation returns the English canonical form for a Chinese
// phrase if one is registered in the translation table.
func ChineseTranslation(zh string) (string, bool) {
	en, ok := zhToEn[zh]
	return en, ok
}
