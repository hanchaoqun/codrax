# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T18:36:19Z
- sweep_start_ts: 20260831-113618
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260831-113619 | answer_regex | none | 170s | 28 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 主调用链和 walker 角色正确，但把 `fixed=false`/`fixed=true` 对应的 LiteralMatcher/RegexLikeMatcher 写反。源码 `src/main.rs:15-18` 是 `if fixed -> LiteralMatcher`、`else -> RegexLikeMatcher`；finalizer 的 typed handoff 只有两条构造调用/绑定，没有 guard/else 到 effect 的极性。repomap 与 dataflow 已能跨语言铸造 parser-owned `branch_effect`，但 emit_evidence 的 selected-definition 自动补齐只投影 body call，未投影同 callable 的 branch effect，导致模型从两条调用自行猜极性。记 B1506。一次成文拒绝为 diagram visible_label 与 body message 不一致及未证聚合构造边，局部修补后图可用。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260831-113619 | answer_regex,answer_contains | none | 510s | 59 | read=23,repo_map=2,list=0,trace=0,source_lens=0 | midloop=15,inv=3/0,fin_reject=5,unavail=0,prune=2 | fail | 表格和正文基本完整，但要求的 analyze→finalizer 时序图在五次 patch 后被侵蚀为断链图：只剩 `Phase1 -> Dispatch` 和另一个互不相连的 Analyzer→Explorer→Extractor→Finalizer 概念链，缺失 Dispatch→Analyzer、阶段调度与状态载体关系。首稿原有 Run/Phase1/Phase2/BusContext/AnalysisIR/AnswerDocument 及较完整调用/数据流；关系门对未证边给出 remove，模型批量删除后，allowed additions 的三条抽象 precedence 关系替代了执行主干，又触发孤立参与者 roster 往返。runner PASS 仅证明格式合同通过，不能证明图完成用户所问机制。记 B1505。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B1505-DIAGRAMREPAIRAUTOEROSION1/P1`: typed 关系硬门本身应保留，但局部修补协议允许模型用“删除全部失败边 + 添加少量不等价候选”换取通过，且孤立参与者必须逐项决策造成多轮振荡。修复应保护模型已选的关系主干：优先提供同载体 relabel/replace/attach；只有没有同语义 typed 替代时才允许删除，并要求模型明确声明关系覆盖降级，不能把概念 precedence 候选当成执行 call/data-flow 的替代闭包。系统不代画边、不代选关系。
- `B1506-BRANCHPOLARITYHANDOFF1/P1`: parser、cache、dataflow lowerer 已有跨 Go/Python/JS/TS/ArkTS/Cangjie/Kotlin/Ruby/Swift/Lua/Java/Rust/C/C++ 的 `branch_effect` carrier；缺口位于 selected-definition 自动补齐只投影 body call。下一批把同一已选 callable 内、已读行覆盖、parser-owned 的 condition/alternative→effect 一并投影为独立 typed evidence，避免模型靠相邻行或自然语言猜分支极性。
- 两项都不读取用户原文、模型 reasoning、最终 prose 或 Mermaid 可见标签来铸造事实；Trace 显式窗、因果投影、自动补齐、链上根因和活动流无固定时长降级合同不受影响。
