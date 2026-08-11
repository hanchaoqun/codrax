# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T18:35:43Z
- sweep_start_ts: 20260811-113542
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-113543 | answer_regex,answer_contains | none | 156s | 24 | read=4,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B554/B557 prompt 侧生效：copy-ready sequence 同时带 4 条 call、2 条 return Note 和 1 条 guard Note，receipt 为 count=3/kinds=guard,return，无自相矛盾。模型没有选择可选图，这本身合法；但正文明确把三个 typed disconnected component 串成“Logger 初始化调用 make_sink，结果最终交给 sink_”的完整路径，而现有证据没有 Logger→make_sink/返回值→sink_ 桥。runner 的浅层 regex PASS 不能代表关系正确。 |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260811-113543 | answer_regex,answer_contains | none | 207s | 23 | read=3,repo_map=1,list=1,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass-with-caveat | 首稿关系图含 4 条无 authority 边；一次 precise patch 原样复用 typed skeleton，保留 run_pipeline→resolve call、run_in_executor→handle callback 与 register/JsonPlugin binding Note，未删图、未造桥，B555/B556 获生产正证。正文的 JsonPlugin MRO 顺序有 class 声明、两条 cooperative super call 和三项 member-set 支撑；主要残余是 ordered-list 把分离局部事实写成一条连续“解析链”，仍需结构化 component boundary 引导。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 2/2；人工：1/2（1 fail，1 pass-with-caveat）。
- `B554`: production-positive。unary guard 已进入 sequence Note，且不获 edge authority。
- `B555/B556`: production-positive on Python。存在完整 skeleton 时，一次局部 patch 能保住 verified graph；模型仍拥有删除可选图的选择权。
- `B557`: production-positive。annotation receipt 与实际三条 Note 同源，不再出现 count=0 却声称已保留。
- `B558`: no recurrence / close-to-watch。C++ 0 次、Python 1 次 Finalizer reject，未再出现五轮关系修补风暴。
- 新确认 `B559-ORDEREDLISTCOMPONENTBRIDGE1/P1-high`：diagram edge authority 已严格，但 block-level `claim_uses` 不能把每个 ordered-list item 绑定到某条 typed edge/component；模型可在正文/列表中把 disconnected component 叙述成连续路径。现结构不足以做精确 hard gate，禁止用列表标签或原文关键词扫描拟合。优先方案是未来增加 item-level typed relation/component annotations，再由 component topology 校验；在载体到位前保留为 soft guidance + eval witness。
- 新确认 `B560-ACTIVESTREAMTOTALCAP1/P0`：SSE 的 no-visible 误杀已退役，但绝对 `2×request_timeout` 仍会终止持续 reasoning/content/tool progress 的活跃流；当配置为 120s 时即约 4 分钟。需把绝对上限收窄为 transport-liveness-only/无 usable model progress 的逃生臂，真实 reasoning/content/tool progress 后只保留 stall、精确重复退化与外部取消，禁止转成系统代答。
