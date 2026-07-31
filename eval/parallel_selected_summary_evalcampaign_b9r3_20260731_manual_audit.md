# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T18:18:03Z
- sweep_start_ts: 20260731-111801
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260731-111803 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 120s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 批 W1 生效：系统覆盖块从错误的 edges=8 修正为完整单视图 edges=3，四个 span/三个 temporal_sequence 边及 40ms 总窗正确。但正文仍称“形成完整 flow”“Flow 验证”“每一段切换都有时间戳证据支撑”，与同答案系统块 frame_flow_causality=unproven 直接冲突；pretriage/analysis 也曾把 11ms Choreographer 写成 12ms。已用 typed frame authority 增强 finalizer 软引导，不扫描正文或硬改模型文字；剩余按跨轮模型波动继续观察。 |
| 2 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260731-111803 | trace_attachment,answer_regex | perf_triage+trace_query | 236s | 38 | read=5,repo_map=1,list=0,trace=2,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 86.111ms > 50ms 的主判断、trace/source 角色和源码机制基本正确；批 W2/W3 生效：主观测没有再借源码 citation，也未再出现 per_item_rejection/“逐条拒绝”，且未复现 >100ms 算术错。仍有两类严重权限问题：模型把无 sched/binder/IO/lock 证据写成 heavy-compute/pure computation、1/1 jank=100%；patch 又追加 `attached_trace-44d2a269.txt [absence: H:RenderService:DoFrame]`，而 span 明明存在。后者已由结构化 negative citation 与绑定工件正匹配核验修复；前者按 pretriage navigation-only 权限措辞 gap 立案，不做单 case 关键词硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
