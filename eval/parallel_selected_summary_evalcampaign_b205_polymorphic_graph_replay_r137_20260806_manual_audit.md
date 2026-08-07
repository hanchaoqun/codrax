# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T00:21:19Z
- sweep_start_ts: 20260806-172117
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-172119 | answer_regex,answer_contains | none | 77s | 21 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S35c 生效：预读的 `runner.py:17` 成功铸成 `run_in_executor -> plugin.handle` callback handoff，成文零拒绝，装饰器的 import-time 角色也正确。但答案仍把实际 `TimestampMixin.handle -> ValidationMixin.handle -> BasePlugin.handle` cooperative MRO 压成“BasePlugin 提供接口并被继承”，没有回答完整执行路径。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-172119 | answer_regex,answer_contains | none | 149s | 20 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | 证据权威明确列出 `Logger.log -> sink_->write` 与 `make_sink -> SinkRegistry.create`，模型以 C++ 原生 `Logger::log` / `SinkRegistry::create` 表达后却被同一校验器连续拒绝 4 次；模型最终删除 edge anchors 才通过，答案因此停在 `Logger::log`，漏掉 `ConsoleSink::write -> std::fputs(stderr)` 末端，且把 factory return 错称 registration。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- 两个 case 均未触发 malformed JSON：strict decode remap、carrier/element-shape recovery 与 string recovery 计数均为 0。本轮失败仍是 typed 关系组合与校验合同，不是 JSON 解析问题。
- `PREREADAUTH1` 已由生产回放关闭：同一 prompt pre-read 字节现在同时进入 grounding，Python callback 行首轮落地，finalizer reject 2→0。
- `QUALPRES1`（P0 合同矛盾）：hard gate 把同一完整限定身份的语言表面 `Logger.log` 与 `Logger::log`、`SinkRegistry.create` 与 `SinkRegistry::create` 当成不同实体；repair authority 又把 dot 形作为“grounded_edge”回显给模型，形成“明确告诉它已证、又按等价写法拒绝”的自相矛盾重试环。
- `COOPPATH1`（P1）：callback handoff 已到达 finalizer，但 override/super/mixin 的有序 cooperative path 没有 typed obligation，模型自然压缩成基类接口。
- `POLYCOMPOSE1`（P1）：return、assignment、type relation、direct call 各自已有 typed 载体，但缺少低心智、逐边可复制的组合教学；模型把 factory return 重铸为 registration，并在重试中删掉 terminal implementation。

## Batch decision

1. S36a 先统一完整限定身份的跨语言 presentation 等价：仅标准化 `. / :: / # / -> / /` 分隔符，segment 数量和顺序必须完全相同；短名不得借此冒充限定名，不同 owner 仍 fail-closed。
2. S36b 再补 cooperative override/super/trait/mixin 的 typed 有序路径；它是模型可消费的证据与义务，不替代模型结论。
3. S36c 为已落地 typed relation 生成单源、逐边可复制的 `edge_anchor` 提示，schema 仍是 JSON 字段/类型/required 的唯一 authority；禁止让模型从多段 prose 重建 relation enum。
