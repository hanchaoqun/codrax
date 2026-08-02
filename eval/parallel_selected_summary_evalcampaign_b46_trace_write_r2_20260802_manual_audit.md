# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T19:19:16Z
- sweep_start_ts: 20260802-121915
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-121916 | log_regex,write_apply,write_patch_oracle | none | 178s | 19 | read=9,repo_map=2,list=2,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 补丁仍正确，真实 `make check` 已由 `declared_coverage_test_surface` 选中并执行，输出 `python text search contract ok`；但 changed-path authority 只按外层 runner=`make` 映射 C/C++，没有消费目标实际执行的 `python3 tests/check_search_client.py`，所以 `memoclaw/client.py` 仍被判 uncovered。说明上一批只修了选路，未贯通 meta-runner 的具体执行语言到覆盖账本。最优解是“成功的精确目标命令 + typed concrete execution family + exact declared changed path”合取；仅有依赖清单或 Python 脚本读取 Rust 文件仍不得确权。 |
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-121916 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 287s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | REMEDY1 已生效：正文明确 58.320ms 是 compute-delivery/供给折算缺口，热控轨上限不证明热节流机制；显式窗、自动补采、两轴、唤醒链、可消量和完整投影均保留，`dma_fence_default_w` 也在正文和系统事实中，runner 仅被过硬固定前缀 oracle 判负。但新发现两项真实 gap：模型把“不同维度”直接推成“独立且不可相加”，而上下文没有逐行 typed relation 权限；系统主要时间占用表又把 `page_cache_churn` 的计数当量 81.616 错写成墙钟 ms，且单次最大 84.300ms 大于累计值。根因是 occupancy path 未复用既有 non-wall-clock caliber guard。重复一遍根因清单属模型表达波动，禁止系统删改。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
