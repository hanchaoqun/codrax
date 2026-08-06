# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T03:15:07Z
- sweep_start_ts: 20260805-201505
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260805-201507 | log_regex,answer_regex | none | 46s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 4 条只读命令均退出 0；OS/CPU/内存/GPU 与完整 payload 一致，最终换算 128 GiB 正确，零重试/答案丢失。Planner 把 missing_observations/success_criteria 各发成单字符串，由兼容层无损 singleton-array 修复；summary 的 repair=0 未覆盖 operation compat repair。 |
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-201507 | answer_regex,answer_contains | none | 155s | 27 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 顺序、职责、图和 edge kind 均正确，零成文拒绝；B157 citation normalizer 未再改写模型引用。Explorer 却为 4 members 发 8 个裸 positional support_refs，系统按位置只保留前 4 个 enum identity 行，模型因此自行选择较弱引用；另有“Explore 耗时最长”无 typed 测量支持，记模型越界单 witness。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `EVAL-B157-CITMONO1` 生产回放关闭：本轮唯一 pre-emit mechanical repair 是 diagram edge metadata normalization，模型 citation_ref 未被确定性重绑。
- `EVAL-B157-EVSPAN1` 尚未生产关闭：模型仍以 `scope=line` 携带多行 StageBinding snippet 和跨行职责 summary；需要把相同选择前置到 completion support-ref 决策，保持软教学而非 prose 硬门。
- 新确认 `EVAL-B158-SUPPREFMULTI1=P1 context precision`：4 个成员 + 8 个裸 positional refs 是歧义 JSON；现有容错保留第一批 identity refs，丢掉第二批 behavior refs。最优方案是醒目教学恰好一成员一 ref，并选择能证明 member_note 的 bounded span；不猜哪批“更好”。
- 新确认 `EVAL-B158-OPJSONARRAY1=P2 efficiency`：operation planner 对两个 string-array 字段发 scalar string；兼容层无损修复且无重试。通过短 JSON shape 决策降低发生率，保留兼容层。
- `EVAL-B158-OPJSONMETRIC1=P2 observability`：selected summary 的 repair 字段不统计 operation tool-param compatibility repair，本轮暂记账，不据此误判“零 JSON repair”。
