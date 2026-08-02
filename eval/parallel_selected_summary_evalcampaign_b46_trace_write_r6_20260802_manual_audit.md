# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T21:27:52Z
- sweep_start_ts: 20260802-142751
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-142752 | log_regex,write_apply,write_patch_oracle | none | 300s | 20 | read=10,repo_map=4,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 仅改 `memoclaw/client.py`；sync/async 都改为 POST `/v1/search` + JSON body，保留可选 namespace，旧 `/v1/memories/search` 与 `urlencode` 均消失。verification probe 与真实 `make check` 均通过；report=`passed`，changed path=`covered`，caliber=`project_runner`，source=`declared_coverage_test_surface`。API reference 与测试未改，写模式无回归。规划阶段因 contract_refs/old_text/probe 结构校验发生 4 次模型自修，但上下文最终精准，未造成错误改动。 |
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-142752 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 319s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Runner 仍只缺旧固定词形“等待对象 dma_fence_default_w”，typed 调用点存在。REL3 协议已形成生产见证：Explorer 提交并获准 `#4+#13=5.149ms`、`#10=1.648ms` 与 cross-ruler forbidden 三条结构化 claim；Finalizer 第一稿漏带 claims 被拒，第二稿由模型自行复制并改正跨尺相加，系统未改写正文。显式窗、4 次 windowed query、自动补齐、两轴、根因排序、唤醒链与完整投影均保留。但 human 仍 FAIL：首批 authority 只覆盖 two-ruler，模型继续对无 exact carrier 的其它 pair 声称 running 包含 runnable、IO latency 与 burst 重叠、CompThread/JankManager 跨行包含/重叠/独立；其中 running/runnable 实为目标封闭状态分区中的互斥成员，且互斥成员可相加重构窗总量。证明 model-owned 校验框架有效，但 producer coverage 不足；须从 typed partition/fold/interval carriers 通用铸造 authority，不能扫描正文词形。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
