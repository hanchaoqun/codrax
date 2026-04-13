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
var zhToEn = map[string]string{
	"探索器":  "explorer",
	"分析器":  "analyzer",
	"协调器":  "orchestrator",
	"终结器":  "finalizer",
	"执行器":  "executor",
	"规划器":  "planner",
	"评审器":  "reviewer",
	"实施器":  "implementer",
	"验证器":  "verifier",
	"证据":   "evidence",
	"假设":   "hypothesis",
	"任务":   "task",
	"计划":   "plan",
	"风险":   "risk",
	"合同":   "contract",
	"终结":   "finalize",
	"实施":   "implement",
	"验证":   "verify",
	"解释":   "explain",
	"停止":   "stop",
	"配置":   "config",
	"注册":   "register",
	"返回":   "return",
	"调用":   "call",
	"链路":   "chain",
	"流水线":  "pipeline",
	"阶段":   "stage",
	"工具":   "tool",
	"记忆":   "memory",
	"会话":   "session",
	"错误":   "error",
	"日志":   "log",
	"重试":   "retry",
	"超时":   "timeout",
	"缓存":   "cache",
	"并发":   "concurrency",
	"锁":    "lock",
	"模板":   "template",
	"场景":   "scenario",
	"合规":   "compliance",
	"安全":   "security",
	"性能":   "performance",
	"兼容":   "compatibility",
	"数据完整": "data_integrity",
	"运维":   "ops",
}

// ChineseTranslation returns the English canonical form for a Chinese
// phrase if one is registered in the translation table.
func ChineseTranslation(zh string) (string, bool) {
	en, ok := zhToEn[zh]
	return en, ok
}
