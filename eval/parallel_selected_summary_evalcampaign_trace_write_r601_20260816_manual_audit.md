# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T01:42:50Z
- sweep_start_ts: 20260816-184249
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-184250 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 113s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | B951 生产正证：finalizer 收到完整 target-state authority，正文精确显示 running=157.248、runnable=5.604、sleep=70.338、D/IO=0，状态 oracle 已全过。频率结论正确区分 cpu0/cpu4 policy presence 与 target-slice binding unproven；runner 仅因词序写成“频率上限的策略存在性”而未命中固定 `策略上限` 正则，属 oracle 假阴性。模型把 S 状态称“深度休眠”，并写“运行与休眠合计覆盖全窗”（实际还需 runnable 5.604ms），记轻微语义/算术措辞偏差；不以原文扫描硬门或系统改写。Analyzer 一次通过且 source exclusion 正确，但仍把被动有限效果问法归为 observed_value/bounded_fact_set。 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260816-184250 | write_apply,write_patch_oracle | none | 203s | 25 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单文件 production patch 正确：whole-valued float years/months 归一为 int，fractional float 抛 ValueError，测试文件未改。模型 probe 与 `python3 -m unittest discover -v` 等 5 项全部执行通过，changed path caliber=`project_runner/target_behavior`，workflow 以 all_verified 收口，无 replan/假绿。Coder apply 后多生成一轮无工具 prose，但只执行一次 apply_patch，未重复改文件。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- `B951-TYPEDTARGETPARENSPID1`: production-closed. Exact finite state authority reached the model and all four values survived the principal answer; no causal projection was manufactured.
- `B952-RUNTIMEANALYSISJSONSHAPE1`: production-positive but partial. Analyze retries fell from three to zero and the dedicated current-source exclusion quote was correct on the first emit. The model still chose observed_value + bounded_fact_set for a passive finite effect question.
- New `B953-PASSIVETARGETEFFECTSALIENCE1`: soft schema guidance only. It states that passive “target was limited/constrained/affected” wording remains target_effect_verdict even when the condition class is unknown. No validator, request/answer scan, verdict, or projection rule changes.
- The trace question is finite state + finite effect and correctly has `trace_query_final_projection_blocks=0`; absence of a full Trace causal projection is not a gap here. Root-cause/roster/mechanism requests retain the causal path.
- The Trace runner failure is a lexical oracle false negative, not a product-answer failure. Do not make product wording match one regex.
- Write completed with native target behavior evidence; no source-static promotion or unverified acceptance occurred.
- Neither case had an empty answer, malformed JSON recovery, system-authored conclusion/relation/diagram, unavailable tools, or active-stream fixed-age degradation.
