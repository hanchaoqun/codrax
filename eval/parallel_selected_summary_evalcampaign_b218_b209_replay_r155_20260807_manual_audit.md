# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T08:56:47Z
- sweep_start_ts: 20260807-015645
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260807-015647 | answer_regex,answer_contains | none | 108s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 核心解析链、JsonPlugin、decorator 注册时机和 MRO 均正确；一次成文、零 JSON/diagram 拒绝。可选 callback 图未生成，所以 B247 仅有单测、没有本轮生产验收。`@register` 写入 REGISTRY 的说明真实，但最终可见引用落在 lookup 行而非 bind 行，属引用覆盖瑕疵。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-015647 | answer_regex,answer_contains | none | 116s | 20 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | 已正确解释 virtual dispatch，并把 setup/dispatch 分成两个列表，runner 因而转绿；但 summary 仍把 Logger 构造叙成直接经过 make_sink/create，源码只证明构造器接收 unique_ptr。更严重的是模型逐字复制系统单边 copy-ready capsule 后，validator 又以正文另一条 principal call 未入图为由拒绝，形成确定性合同自冲突并迫使删图。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### EVAL-B248-OPTDIAGCOMP1 — copy-ready 子图与“正文调用边必须全部入图”硬门冲突

C++ 第一稿自绘图含未证 vtable bridge 和未配对反向 reply，被拒绝是正确的。第二稿则逐字复制 prompt 提供的唯一可复制图：
`Logger.log -> Sink.write`，并携带完全一致的 `relation_kind=call` anchor。系统仍返回
`principal_call_edge_missing=ConsoleSink.write -> std.fputs`。这证明 validator 把可选图错误当成正文 principal 调用集合的完备复制品；遗漏真实边不是虚假关系，
不应成为 hard reject。最优边界是：所有**可见**图边必须有同向 typed authority；可选图可忠实展示任意已证子集，未画边继续由 sibling prose/list 承载。

### EVAL-B249-RELHANDOFFAUTH1 — Relation Role Handoff 再次提升 soft path

同一 finalizer prompt 已把 C++ 的“完整调用链”标为 `fact_authority=advisory_model_inference`、
`principal_contract=not_authorized`，但后面的 Relation Role Handoff 又称“principal relation member_set 已存在；That required member set is the
answer-member carrier”。这与 component boundary 的 `unproven_between_components` 直接冲突，解释了模型为何仍在摘要里把 setup 与 Logger 构造拼成已证 bridge。
该消费面必须复用共享 `AnswerAggregateFactAuthorizesPrincipalContract`/typed relation marker，不能只按 role/关系形状判断。

### 其他结论

- 两例均无 strict JSON decode、字符串恢复或答案消失；本轮没有暴露 JSON 教学新矛盾。
- Python 一次成文成功，但模型没有选择 optional diagram，因此 sequence callback 修复仍是 `implemented/production-unexercised`。
- C++ 的 `std::fputs`、unknown kind 等若干 item citation 指向定义/相邻行，说明引用精度仍需后续异构回放观察；本批不按具体符号硬拟合。
- Trace runtime family 未进入 source-diagram contract；显式时间窗、因果投影、系统补齐、根因排序、唤醒链、窗内可消除量和双维根因分析均未受影响。
- sequence display message 参数污染 typed endpoint identity、所有语言 labelled/unlabelled flowchart relation-anchor 旁路继续独立开放，不能由本批销账。
