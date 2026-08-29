# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T10:56:29Z
- sweep_start_ts: 20260829-035628
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-035629 | log_attachment,answer_contains | log_triage | 131s | 27 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 核心语义已转正：ArkTS 第一帧为 `NativeBridge.invokeOhSum:33`、调用者为 `HomePage.computeTotal:54`；仓颉第一帧为 `demo.bridge.ohSum:18`、调用者为 `checkout:42`；两栈未再伪造因果，附件内精确路径完整保留。残余是模型 caveat 仍复述 `<unverified-external-source>`，系统又把同一附件路径误报为“尚未由当前证据确认”；log triage 先误铸 cause、再复制非逐字 observation，共 3 轮，analyzer 也在 root-cause 与 bounded-fact 合同间耗 3 轮。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260829-035629 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 234s | 47 | read=1,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统能力完整：精确窗、四跳链、链上排序、两类耗时维度、业务线索、背景隔离、完整因果投影与自动补齐均保持，唯一 read 是 `.codrax/blob/.../trace-query-result-*.json` 运行时工件，不是源码回退。模型正文却把明确重叠的两个优先级席相加为 43.035ms、把 D/IO 重叠席相加为 17.819ms，并在无绝对标尺时称压力“中等”、目标线程“轻载”；系统投影已给出同向取最大且不可相加的正确权威，故记为模型遵循残余，禁止用正文关键词硬门或系统改写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
