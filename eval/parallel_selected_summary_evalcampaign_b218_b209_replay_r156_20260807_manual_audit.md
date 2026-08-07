# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T09:22:39Z
- sweep_start_ts: 20260807-022238
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-022239 | answer_regex,answer_contains | none | 85s | 21 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | S37i 生产验收通过：soft call-chain set 不再被 relation handoff 称为 required，模型把 selection/dispatch 分成两个阶段并正确解释依赖注入、virtual dispatch、ConsoleSink/stderr；一次成文、零 diagram/JSON 拒绝。列表顺序仍可更清晰地区分 setup 与 log-time，但没有再宣称构造器直接调用 factory。 |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260807-022240 | answer_regex,answer_contains | none | 169s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 最终 JsonPlugin、resolve/REGISTRY、decorator 绑定与 executor callback 均正确，引用覆盖充足。首次自绘 5 条混合语义图没有复制 typed capsule，被拒合理；patch 删除唯一 diagram 却把旧 edge_anchors 搬到 ordered_list，孤儿 metadata 造成第二次无价值拒绝，最终再删 anchor 后通过。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### S37i 生产验收

- C++ 从 r155 的 `finalizer_rejects=2` 降至 0；系统 copy-ready 子图没有再与正文 principal call 完备性冲突。
- 两例 prompt 的 Relation Role Handoff 均明确输出 `No evidence-authorized principal relation member_set is active`，不再出现
  `That required member set is the answer-member carrier`；C++ 最终也不再声称 Logger 构造器直接经过 factory。
- 两例 `strict_decode_*` 全为 0；没有畸形 JSON、自愈字符串、答案消失或 schema 教学冲突。

### EVAL-B250-ORPHANEDGE1 — 删除 optional diagram 后孤儿 edge_anchors 继续触发关系门

Python 第一稿自绘了 `run_pipeline/resolve/REGISTRY/JsonPlugin/executor` 的连续 sequence，包含 assignment/return/callback 被当 call 等未证边；首次拒绝是
正确的证据保护。第二轮 patch 已 `remove_block_ids=[sequence-diagram]`，但又把 `run_pipeline -> resolve` 与 `loop -> handle` anchors 放进 ordered_list。
此时整份文档没有任何 typed diagram body，anchors 已无可绑定的 node aliases，却仍被当成隐形关系主张，因 `loop` 不等价于
`loop.run_in_executor` 再拒一次。

最优泛化方案是 pre-emit 机械清理：只有当文档中**不存在任何 typed diagram block** 时，移除所有孤儿 `edge_anchors`；仍有 diagram 时，兄弟
prose/list block 为另一图携带 anchor 的现有能力保持。该修复只处理 schema metadata，不修改模型 prose、结论、引用或 Mermaid body。

### 仍开放

- Python 没有逐字复制系统 callback capsule，故 `EVAL-B247-SEQUENCECALLBACK1` 仍是单测覆盖、生产未直接验收；本轮不能把模型自绘图失败误记成 capsule 自拒。
- sequence display message 参数不得污染 typed endpoint identity 仍开放；本轮第二拒绝是孤儿 anchor 的缩写端点，不是 message payload 参数改写。
- 所有支持语言 labelled/unlabelled flowchart edge 的 strict relation-anchor 旁路继续开放。
- Trace 显式时间窗、因果投影、系统补齐、根因排序、唤醒链、窗内可消除量和双维根因分析均未进入本批 source-diagram 路径。
