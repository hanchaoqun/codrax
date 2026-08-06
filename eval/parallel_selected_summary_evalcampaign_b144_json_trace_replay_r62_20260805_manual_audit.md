# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T23:59:58Z
- sweep_start_ts: 20260805-165957
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260805-165958 | typed_inventory_rowset,answer_contains | none | 96s | 20 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 4 个 @Entry + 2 个 @Builder 完整；一轮 typed lens、一次 completion、Finalizer 0 reject。Analyzer 仍把 production prescan 未命中叙述成无 ArkTS，但没有取得 evidence 权限，Explorer 首轮 lens 即纠正；P2 继续观察。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260805-165958 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 112s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 模型正文保留 2.000–2.020s、threadpool→network→cookie→app、IO 11ms + runnable 1ms 两修向与两轴结论；系统因果投影在。Analyzer fact_families/non-bounded 一次 typed reject；系统主要占用表重复列同一 iowait 11ms，见 P1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- `EVAL-B143-SILENSREFINE1` / `EVAL-B143-EVIDJSONMIND1`：production replay pass on the happy route。ArkTS 从 r61 的
  139s、repo_map=3、completion=4/2、Explorer 11 轮，收敛为 96s、repo_map=1、completion=1/0、Explorer 5 轮；没有 string carrier、
  item-local unknown field 或 Finalizer reject。模型本轮没有再次走 file-role refusal，因此 typed same-tool refinement 与 redundant-support-ref
  lossless repair 仍保留 targeted pin，不谎报具体分支已被真实触发。
- `EVAL-B144-TRACEACCTDUP1`（P1）：模型正文正确只把 threadpool-400 iowait 11ms 计一次；系统生成的“主要时间占用 / 关键路径候选”表却
  同时列 root-rank state span 2.003..2.014 与 wakeup-composition observation 2.002..2.016，两行主体/type/value/count 都是
  `threadpool-400 / iowait / 11ms / 1`。底层是同一状态账的两个 typed 投影，不是两个可相加事件；当前页面没有 row-local same-account
  标记，读者可能误加成 22ms。根修必须使用 engine-owned account/segment identity 或明确 absorption lineage，不能凭同名/同值/prose 合并，
  以免吞掉同线程两个真实独立事件。
- `EVAL-B138-TRACEFACTFAMILYVAR1` 获得第二个异构 witness：`scope=causal_diagnosis` 仍携带 fact_families，被 runtime precise gate 一次拒绝后
  正确移除。description/skill 教学本身同向，无矛盾；可用 JSON Schema `if/then/else` 在字段生成面表达“bounded 必带、其他必省略”，减少模型
  心智。不得放宽 runtime gate，也不得扫描用户/模型 prose。
- Trace 主答案同时覆盖实际时间占用/新探索方向与现规则可消除量，明确两轴不可相加；`Trace 因果投影`、系统补采、排序、唤醒链均在，
  系统没有替换模型正文。
