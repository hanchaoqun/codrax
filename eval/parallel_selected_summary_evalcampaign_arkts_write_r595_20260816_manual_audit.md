# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T23:51:18Z
- sweep_start_ts: 20260816-165116
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260816-165118 | typed_inventory_rowset,answer_contains | none | 121s | 26 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 4 个 `@Entry` 页面类型与 2 个 `@Builder` 函数、逐行路径和引用全部正确，无额外成员。确认 B946：首个修补后每行 `label` 已是可见成员名，requested-dimension evaluator 仍因 block kind=`section` 不算 member-set payload，误发“函数名缺失”，迫使模型把名字重复写入 `cells`；这是 typed block-shape 漂移，不是 ArkTS 抽取或模型 JSON 问题。 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260816-165118 | write_apply,answer_regex | none | 288s | 25 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail / honest-unverified | 只生成并应用一个最终计划，`make check` 的 Python source-shape oracle 35ms 通过；两个 Rust path 精确标为 `capability=source_static`，controller 的 `all_verified` 被确定性降为 `accept_unverified`，终稿诚实。补丁没有 Rust 编译/行为证明，不能判可合并。该轮没有真实 verify failure/replan，故 B943 未获生产命中；planner 的一次 Go probe 因目标语言不匹配被精确拒绝后移除，未形成矛盾合同。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- B946 的根因是同一份 block schema 在 renderer 与 requested-dimension evaluator 中有两套结构枚举：renderer 明确认可 `section` 的逐项 `Items`，后者只数 ordered/bullet/table。已改为共享 `AnswerBlockRendersStructuredItems`；成员名从可见 `label/text/cells` 结构取得，不扫描用户或答案 prose。
- canonical Markdown table 另按真实可见表体计一席，隐藏 citation sidecar 不再冒充 member rows。新增 section-label 正臂与 hidden-sidecar 负臂，完整 `internal/agent` 套件通过。
- Rust 案从 r592 的 672s 收敛到 288s，但本轮没有进入失败验证后的换代车道，因此不能用时长下降宣称 B943 production-closed。保留专项回放义务，不制造失败也不降低 Rust 行为验证口径。
- 两案均有用户可见答案；无 malformed JSON、空答案、固定 4ms/总年龄降级。Read/Trace pipeline 未改，显式窗因果投影、自动补齐、链上-only 主因和背景 support-only 均保持。
