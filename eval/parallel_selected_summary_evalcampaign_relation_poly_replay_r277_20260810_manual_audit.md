# r277 exact-two 人工审计

- 基线：`main@5f3bc07fd`
- 运行：`PARALLEL=2`，两个 case 共用同一不可变二进制快照
- runner：2/2 PASS
- 人工：0/2 PASS

| case | runner | 人工 | 关键结论 |
|---|---|---|---|
| `qf_logic_view_read_pipeline` | PASS | FAIL | 正文描述了阶段顺序，但 Mermaid 仅剩两条内部 helper call；Analyzer/Explorer/Extractor/Finalizer/BusContext/Mutable 全部断开，未回答所求逻辑关系图。 |
| `mr_poly_binding_chain` | PASS | FAIL | native guard、模块名、注册、slow fallback 均在，但 `tokenize_bytes (Rust)` 把 line 40 wrapper 与 line 10 core 合为一项且丢失条目引用，wrapper→core 的 line 42 精确调用未进入最终链。 |

## QF：B477 同源 authority 已真实发布，但缺少可直接消费的 typed recipe

all-log 中存在 `Current Run Stage-Lane Authority`、`canonical_read_main_sequence=analyze -> explore -> extract -> finalize` 及四条已核验 binding，证明 B477 provider 与 prompt 接线真实生效。现有教学只给“权威事实”，没有按 diagram 合同逐条给出 `from`、`to`、`relation_kind=precedence` 的发射形。模型于是反复把阶段顺序表达成 `call`，被证据门正确拒绝，最终删成：

```text
runAnalyzePhase -->|calls| dispatchStage
runReadSchedulerLoop -->|calls| executeStageRequest
```

这不是模型随机波动，也不是应该放宽 call gate；是系统已经持有精确 stage-order authority，却仍让模型猜另一套关系协议。立 `B477b-STAGEAUTHRECIPE1/P0`：由同一 provider 输出恰好三条相邻 precedence recipe，逐条携带 stage/agent 的 from/to 与 relation_kind；明确只证明顺序，不证明 call/data_flow/artifact transfer。系统仍不生成 Mermaid、不补边、不改答案。

本轮同时暴露 `B479-STAGECLOSEBOUND1/P1`：Explorer 70 轮、55 次 read、25 次 mid-loop injection、10 次 completion call。`flow_participant_operation_evidence` 在 stage authority 已充分证明四个 stage participant 后，仍要求为全部六个参与者寻找 operation call/data_flow；BusContext/Mutable 又没有 typed “本次未证”完成载体，导致不断补读。先完成 B477b 并回放，再决定是否需要把共享 authority 纳入 completion satisfaction，或增加一次聚焦后显式边界的 typed 完成车道；不得把 BusContext/Mutable 伪造成已证关系。

## 多态链：事实已收集，结构化 identity/citation 仍未闭合

Explorer 已读到 line 42 `super::tokenize_bytes(&data, &table)`，但发射 evidence 只有 line 40 wrapper definition 与 line 10 core definition，没有发射 wrapper→core call。最终第 4 项把两个不同 callable identity 合并成 `tokenize_bytes (Rust)`，随后 validator 移除其引用，造成“完整链”缺一跳且条目无确定来源。

`B474` 继续为 pending exact wrapper-call evidence；`B478-ITEMCITEID1/P1` 继续审计现有 structured item/citation 校验的覆盖边界：只允许消费 item label、claim form、citation_ref 与 typed source identity，不读取 item free-form text/request/thinking/final prose，也不对 PyO3/Rust 单例硬拟合。

## 状态与不变量

`B477=partial/production-provider-positive`；`B477b=P0-next`；`B479=P1-after-B477b-replay`；
`B478=P1`；`B474=pending`；`B475=production-consumed/soft-only-insufficient`；
`runner=2/2,human=0/2`；`system-diagram/edge/conclusion-authoring=none`；
`raw-request/model-prose-hard-gate=none`；Trace explicit-window、auto-supplement、causal projection、on-chain root-cause 与 non-chain background 分层均未触碰。
