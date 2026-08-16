# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T18:29:10Z
- sweep_start_ts: 20260816-112909
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-112910 | answer_regex | none | 189s | 26 | read=4,repo_map=6,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 模型读全 Python guard/native/fallback、PyO3 module/wrapper 与 Rust core/helper；终稿文字总体有用。但 Explorer 把 line 46 的 module declaration 作为 citable registration、把 wrapper/core/helper 多行函数体作为 definition/mechanism，未发射 line 47 精确 registered-export tuple 及 wrapper→core→helper 的 typed call rows。因为粗 registration 已可引用，B919 的 exact repair 没启动，B921 也无精确 handoff 可消费；首稿图被正确拒绝后模型删除 optional diagram。终稿对 wrapper/core 调用的引用 caliber 仍停留在 definition-site。另有 requested-dimension false negative：正文已经出现 `_fastlex`，后置检查仍要求补“原生模块名”标题。无畸形 JSON、旧稿恢复或系统代写结论。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260816-112910 | answer_regex,answer_contains | none | 373s | 43 | read=18,repo_map=2,list=0,trace=0,source_lens=1 | midloop=10,inv=4/0,fin_reject=2,unavail=0,prune=0 | partial | 首稿含 17 条大量未证 assignment/call/return，validator 正确拒绝；修补后仅保留 3 条 precedence 与 2 条 exact call，且 endpoint alias 均不同，r578 的自环失真未复现。第二次拒绝仅因 analyze/finalizer 已有 incident edge 却残留 `participant_boundaries`；删除 stale boundary 后通过。终图仍把两个没有 typed bridge 的 component 放在同一 sequenceDiagram 中，视觉上下位置可能暗示未证全局顺序；正文/表格基本有用，关系本身未造假。B922 本轮未被直接触发，不能据此记生产闭环。无 JSON 降级、空答案或 active-stream 固定年龄降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

1. Runner 的 `2/2 PASS` 只证明声明 oracle 命中，不代表跨语言关系图和完整流水线关系已经闭环；人工均为 partial。
2. B921/B922 的确定性实现未回归，但本轮上游没有提供 exact registered-export handoff，B921 未获生产触发；Combo 也没有再次生成 typed endpoint self-collapse，B922 只保留单测正证。
3. 新冻结 B923：一个可引用的粗粒度 registration declaration 不得豁免同一已读源码范围内唯一精确 binding expression 的重发义务。系统只发布 exact typed re-emit debt，不创建 evidence、arrow 或答案。
4. 新冻结 B924：模型已选择的多行 definition/mechanism carrier 可以包含 load-bearing executable relations；若 parser 对已读函数体拥有唯一精确 call relation，当前 context 仍可能只有 definition-site caliber。应由 parser-owned typed relation carrier补足上下文，不能靠扫描终稿词句或系统代画。
5. requested-dimension 的 `_fastlex` false negative 记为 B925 观察项。最优方向是让结构化 answer block/typed dimension receipt 对齐，不以原文关键词扩写硬门。
6. 本批不涉及 Trace。显式时间窗、因果投影、自动补齐、链上-only 主因、实际占用/业务线索与规则计价可消除量双轴均保持；活跃流不得因 4ms 或固定累计年龄降级。
