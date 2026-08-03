# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T12:34:50Z
- sweep_start_ts: 20260803-053449
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260803-053450 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 28 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=3,inv=1/0,fin_reject=6,unavail=0,prune=0 | pass | B53-B 正证：finalizer 前 `contextual_noncausal_rows` 已含 adjacent 0.200ms 与 background supply_pressure 3.500ms，并明确 non-target/non-additive；模型不再写“无 CPU 压力证据”，模型正文与系统随后投影不再矛盾。显式窗、唤醒链、8.300ms 候选、两轴与自动补采均保留。新系统 GAP：首轮漏带 closure claim 后，模型两次把 `relation_claims` 放到 document root；compat 静默 quarantine `$.relation_claims`，直到第 4 轮才改到 block，白烧 3 次成文。登记 RELCARRIER1。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260803-053450 | trace_attachment,answer_regex | perf_triage+trace_query | 314s | 39 | read=7,repo_map=1,list=0,trace=2,source_lens=1 | fail | runtime/source 证据边界和 current-source citations 基本正确，但有两处实质语义错：payload 是 `B|2000|H:RenderService:DoFrame`，动作是 B，`H:...` 整体是 span name，不能把 name 中的 H 解释成 flow-end；匿名 E 也不重复 span name，不能宣称 B/E 两端需同 name。系统 performance instruction 已明确“B 配同线程匿名 E”，故本次先记 MARKERSEM1 模型错误/观察项，不做关键词 gate。另有系统 MIXLANE1：analyzer 将合法 runtime entities `86.111ms/16.7ms/frame budget` 强制解析为 repo symbol，误发 R1.5 hallucination reject 并重跑。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B53-CTXBG1`: covered by real replay. Context reaches the model before authorship; no prose mutation or new hard gate was used.
- `EVAL-B53-RELCARRIER1/P1`: confirmed. Fix is an exact structured carrier diagnostic: top-level `$.relation_claims` fails immediately with the valid `blocks[i].relation_claims` path; missing-claim validation and handoff repeat the same path. No claim is relocated or authored by the system.
- `EVAL-B53-MIXLANE1/P1`: confirmed. In a typed mixed runtime-artifact + required-current-source question, repo-symbol hit/miss asymmetry is advisory because different subtopics legitimately belong to different evidence universes; downstream origin/evidence gates remain authoritative.
- `EVAL-B53-MARKERSEM1/P2-watch`: one model factual error despite explicit correct context. Do not fit a raw-prose scanner or answer replacement to this instance; replay later with other trace-marker shapes before considering a generic typed endpoint-semantics card.
