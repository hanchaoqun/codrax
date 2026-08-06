# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T20:38:01Z
- sweep_start_ts: 20260806-133759
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260806-133801 | answer_regex,answer_contains | none | 178s | 23 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | 12 个生产实现、文件和排除 3 个测试实现均正确；首稿 blocks 字符串被无损恢复。第 1 次 type_relation 方向错误被正确拒绝；第 2 次已改为 implementer→interface，却因 ImplementersOf 权威未进入 evidence handoff 再次被拒，模型第 3 轮只能删除 edge_anchors，最终图失去 typed 关系。这是系统 graph→finalizer 断线，不是缺少 parser 能力。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-133801 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 201s | 41 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、pid、6 次查询、根因板、唤醒链、双轴占用/可消除量、Trace 因果投影与自动补采均在场。模型仍把两个 IO 席跨行相加为 14.859ms、把无绝对标尺的 pressure density/score 定级为“中高/推高水位”，并把 priority_inversion_candidate 叙述成已持锁；typed 禁加/禁定级边界已到 final，按红线不扫描正文硬拒或系统改写，记模型/路由服从 gap。另确认系统算术附注误把同句窗长 114.940ms 与后面的 73.4% 配对，制造一条假不自洽提示。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B192-TYPERELGRAPH1` 不是完整闭环：显式 parser relation 已通，但 Go 等隐式实现的 typed `Graph.ImplementersOf` 只服务搜索/枚举，没有成为可引用关系证据。修复应复用该跨语言图，不解析 repo_map Markdown，不信任模型自报 `implements`。
- Trace 回放是重要不回归证据：本轮源码图关系改动没有触碰显式时间窗、因果投影、自动补齐、根因排序、唤醒链、窗内可消除量或双轴归因。
- `EVAL-B186-RELSYNTH1/PRESSCAL1` 再次复现，但 typed soft boundary 已精准到达，维持模型/路由层观察；不能用正文关键词门、normalizer 或系统代写来追求单次答案。
- 新确认 `EVAL-B193-ARITHPAIR1=P1`：可见算术附注的局部 prose 配对器把窗口长度当作百分比分子。它不改模型正文，但向客户发布错误审计判断，下一批应收窄到同一数值短语/明确局部关系，无法唯一绑定则不复算。
