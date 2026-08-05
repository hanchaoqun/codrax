# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T18:26:31Z
- sweep_start_ts: 20260805-112630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-112631 | answer_regex,answer_contains | none | 134s | 20 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/1,fin_reject=2,unavail=0,prune=0 | pass | 模型正确还原 `run_pipeline -> resolve -> REGISTRY["json"] -> JsonPlugin instance -> cooperative handle MRO`，并区分运行时类与方法定义 owner。两次成文拒绝都来自可选图把动态绑定硬画成静态 `run_pipeline -> JsonPlugin.handle`，模型最终删除图后通过。新 C2 capsule 生效但只产出 `BasePlugin -> abc.ABC`；复核代码确认 Python decorated class 丢弃 `pyExtractClass` 返回的 relations，故 `JsonPlugin -> TimestampMixin/ValidationMixin/BasePlugin` 没进入图。capsule 还夹带无关 `CsvPlugin.content_type`/`JsonPlugin.content_type` return 行，增加错误联想与 JSON 心智负担。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-112631 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 163s | 39 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | JSON/tool call 正常，模型 principal 正文存在，显式窗、系统 Trace 因果投影和自动补齐均保留；但模型违反同一 prompt 尾部的 typed authority：在 `causal_conclusion=unproven`/`frame_evidence_status=absent` 下写“帧延迟由…造成”，把 34/36 次主导唤醒写成“完全”，把全部 sleep 写成设计性/VSync 等待，把 wakeup+runnable 写成直接阻塞；还将同方向重叠席 23.994+19.041 相加、将不同 IO 口径 10.433+7.386+6.673 相加，并扩写无 typed row 支持的 55ms 代表窗、误写 TID 61841/61839。更深层系统矛盾：本轮 runtime-only root-cause contract 一边硬教 `ordered_list` 和 `current_code_path`，另一边允许 section 形并把缺 list 降为 advisory；这既增加 JSON 心智负担，又用当前源码合同污染纯 Trace 回答。Finalizer prompt 约 229KB/56.9K tokens，重复 Trace 规则掩盖了尾部紧凑决策边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed generic gaps

1. `EVAL-B124-CONTRACTSST1` (P0): Required Answer Facets 从原始 `AnswerSurfacePlan.FacetCoverage` 渲染，而 block contract/validator 使用应用过 runtime-only override 的 `AnswerSemanticView`，导致同一次成文同时收到“current_code_path HARD”和“当前源码不要求”的矛盾合同。
2. `EVAL-B124-TRACEBLOCK1` (P1): runtime-only causal diagnosis 被通用 root-cause stack/cause-chain 模板强制成 `ordered_list`，但客户要求的是分层诊断，模型自然使用 sections；系统随后又把硬要求缺失降 advisory，教学与执行口径不一致。最优解是按 typed `runtime_question_profile.scope=causal_diagnosis` 允许 section/table/list 载体，不扫描原始问题或答案文字。
3. `EVAL-B124-PYDECREL1` (P1): Python `decorated_definition` 分支保留 decorated class symbols/methods，却丢弃同一个 `pyExtractClass` 返回的 inheritance relations；所有带 decorator 的多继承 class 都会缺图层，不是 `JsonPlugin` 特例。
4. `EVAL-B124-CAPSULEFOCUS1` (P1): discover-target relation capsule 无 typed connectivity 过滤，任意 concrete return/assignment 都可抢占预算；应只保留与 static-call/registration typed endpoint 精确连通的 value/factory 行。
5. `EVAL-B124-TRACECOGNITION1` (P1): Trace prompt 已有正确尾部 typed decision boundary，仍被前面数十 KB 重复规则和 16K 行 raw JSON 分页摘要淹没。下一批应去重/压缩 typed context，不能增加新的同义警告，更不能让系统改写模型结论。
6. `EVAL-B124-JSONTEACH2` (P1): 本轮没有 malformed JSON，D1 salvage 未触发；但 Required Blocks、Facet、Support Lane、Submission Checklist 对同一 carrier 给出不一致要求，属于系统教学矛盾。JSON schema 继续作为字段单源，语义教学必须从同一个 compiled view 派生。
