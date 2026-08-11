# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T08:22:45Z
- sweep_start_ts: 20260811-012244
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260811-012245 | typed_inventory_rowset,dimension_substring,answer_contains | none | 335s | 29 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=4,inv=2/1,fin_reject=6,unavail=0,prune=0 | fail | 恢复稿的 2 个 extend、2 个 foreign func、8 个 public class 及路径/package 与 typed inventory 一致，说明 Cangjie 提取没有丢成员；但六次 full/patch 均被同一 row-id hard gate 拒绝后降级。模型合法使用 cells-only table，`cells[0]` 为成员名并逐行复制正确 `source_inventory_row_id`，校验器却只读空的 `label`，形成 renderer/schema 接受、typed identity 拒绝的载体合同自冲突。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-012245 | answer_regex,answer_contains | none | 798s | 43 | read=41,repo_map=3,list=1,trace=0,source_lens=0 | midloop=12,inv=6/0,fin_reject=6,unavail=0,prune=8 | fail | B510-E 获生产正证：四阶段表格含四行且五列完整。图仍不合格：只保留 Run/Loop/dispatchStage 等六条实现关系，Analyze→Explore→Extract→Finalize 的 stage sequence 与 AnalysisIR/EvidenceItems/AnswerDocument 交接缺失；`codrax/analyze/finalizer/Mermaid` 四个断开 participant 连续四轮 boundary reject，最终仍以内部函数名为主。runner 的浅表 PASS 不能覆盖该关系缺口。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B510-E-EMPTYTABLE1` production-closed：新二进制不再接受 header-only 空表，模型补出了四条可见 stage row。
- `B514-SICELL1/P1-high` confirmed：source-inventory exact row gate 与通用 structured-table contract 对 `items[].cells[0]` 的成员身份解释不一致。修复应让 `label` 与 cells-only 首列在同一 typed identity helper 上等价，后续 cell/text 不能成为身份；不扫描 request/title/prose，不改模型值。
- `B510-G-STAGEFLOW1/P1-high` confirmed：本样例不是模型单次波动。Analyzer 重试后丢失 typed requested dimensions/业务 participant slate，checkout-verified stage precedence 又因显示 alias 污染 exact endpoint 而撤权，最终关系池只剩实现调用。应在 typed workflow intent 上保住 stage/flow carrier，并将业务显示 label 与 exact relation endpoint 分层；不能把任意 flow 图硬套 read-mode stage skeleton。
- `B510-F-BOUNDARYREPAIR1/P2` 再次命中：同四个 disconnected participant 的“可见 node + unproven row”在四轮 patch 中反复失败。repair 应按 participant 给单一结构化 recipe，仍由模型 patch。
- `B510-D-BUSINESSLAYER/P1` 仍开放：图应以“请求分析/证据探索/证据提炼/答案组织”等业务动作作显示层，源码函数/字段只作组内 exact evidence endpoint；显示 alias 永不铸造关系权威。
- `B515-MERSEQ1/P1` confirmed：`013600` 的 sequence participant display aliases（如 `Orchestrator.Run`、`o.busCtx.AnalysisIR`）未加引号，统一 Mermaid source normalizer 未覆盖该 sequence 语法，生产计数也显示 `mermaid_source_repair_applied=0`。跨 renderer 自愈只应 quote 右侧 display label，participant id、edge/message 与 typed endpoint 必须保持不变；不可修图继续显式 raw-source fallback。
