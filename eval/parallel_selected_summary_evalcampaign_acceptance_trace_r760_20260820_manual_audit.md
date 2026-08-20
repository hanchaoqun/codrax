# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T07:09:47Z
- sweep_start_ts: 20260820-000946
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-000947 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 270s | 41 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=1,inv=3/1,fin_reject=1,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、目标 20ms sleep、已证 threadpool→network→cookie→app、11ms IO 首席、三条 1ms 调度供给、实际占时/现规则可消双轴和因果投影均完整。模型把 3 条边/4 个 CPU 节点口误成“四跳”，且同段同时写“三席小计 3ms”与“同方向不可相加”；系统板给出的“区间互斥可小计”是准确的，属模型波动。系统校验附注继续泄漏 `typed 事实/typed 席位/typed confidence`，B1219 获生产复证。 |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260820-000947 | log_regex,write_apply,answer_regex,answer_contains | none | 529s | 26 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 仅修改 `fastlex/tokenizer.py`，原五换行测试字节保持不变。实际 probe 覆盖 singleton `[10]`、五换行 `[300]`、普通 `hi->[256]` 与混合内容，补丁全部通过。Controller 精确消费 `required=0/covered=0/planning-only=10`，最终用户面把已验证范围限定到必需 typed 义务，并明确自然语言清单不是逐项执行证明；B1221 获生产正证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit findings

### 1. Write verification authority: production-positive without a prose hard gate

- The active repository paths remain normalized to `fastlex/tokenizer.py`; no multi-repo path rejection recurred. The protected five-newline project test is byte-identical in the applied tree.
- The emitted bounded Python probe directly imports the changed module and asserts four independent behaviors: ordinary BPE, a single newline remaining `[10]`, five newlines collapsing to `[300]`, and mixed-content preservation. The applied loop starts a collapse only after observing an actual identical pair, so it does not repeat r759's singleton regression.
- All analyzer outcomes were correctly stamped `planning_only_ungrounded`; therefore the proof ledger did not falsely promote them to hard contracts. The changed production path nevertheless has executed `target_behavior` coverage from the probe, and the final proof is honestly `adequate`, not `strong`.
- The controller prompt includes `required_typed_contracts=0`, `covered_required_typed_contracts=0`, `planning_only_contracts=10`, and `natural_language_acceptance_items=0`. Its reasoning cites this exact scope before finishing. The user-facing card states that natural-language checklists do not imply independent evidence for every item. B1221 therefore has a direct production witness while preserving the rule that model prose cannot mint a hard gate.

### 2. Explicit-window Trace: causal authority intact, reader-language debt remains

- Twelve windowed trace_query calls cover five dimension families. The final answer correctly reports app-100 as sleep 20.000ms with running/runnable/D/io_wait all zero; the exploratory one-line “20ms runnable wait” slip does not survive into the answer.
- The final causal projection preserves the proved `threadpool-400 -> network-300 -> cookie-200 -> app-100` wake chain. It crowns only the on-chain 11.000ms `fscache_page_wait_on_page_bit` IO wait, then lists three 1.000ms runnable intervals as scheduling-supply items. Adjacent sleep and background IO-pressure rows do not enter root-cause ranking.
- The actual-occupancy table and current-rule eliminable overview remain separate. The projection marks the scheduling intervals mutually exclusive and permits their 3.000ms subtotal while forbidding addition across repair directions. No neighbor/background promotion or system conclusion substitution occurred.
- The model prose has two small inconsistencies despite accurate context: four CPU nodes were called “four hops” although the chain has three edges, and one row says both “subtotal 3.000ms” and “same direction cannot add”. These are model fluctuations; do not add a user/final-prose keyword gate. The deterministic board remains correct.
- The independent generalized presentation GAP is B1219: system-owned cross-check rows still expose implementation terms such as `typed 事实`, `typed 席位`, and `typed confidence`. This is not model authorship and can be fixed at the structured renderer source without touching conclusions or Trace authority.

## Runtime and red-line audit

- The two active streams completed in 270s and 529s. Neither was degraded by 4ms, 4m, first-byte, stall, or accumulated-age thresholds.
- Trace explicit-window projection, automatic supplementation, on-chain-only root-cause ranking, priority/scheduling/compute/D/IO/deterministic-semantic channels, and actual-occupancy versus eliminable-impact accounting remain intact.
- No hard gate reads raw user input, plan prose, generated patch/test prose, model reasoning, final answer text, or Mermaid text. The system did not author or replace the model's conclusion.
