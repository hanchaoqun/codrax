# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T22:04:24Z
- sweep_start_ts: 20260806-150423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260806-150424 | answer_regex,answer_contains | none | 135s | 21 | read=1,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四个 stage 名称、次序和职责均正确，零 Finalizer reject；但 Mermaid 把证据载体函数 `AllMainStages` 画成第五个 pipeline 节点并连向 StageAnalyze，不满足“4 个主 stage”的领域图边界。职责正文与 ordered list 重复，且逐项 citation 选了 enum 名称行而非已在证据池中的 StageBinding 职责行。与 B153 同 case 的正确四节点生产结果对照，先记模型波动/P2 watch，不加硬门。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-150424 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 199s | 45 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | S30 生产闭环：principal axis-B 行显示 exact priority_inversion candidate kind 与局部权限；模型总结明确“未经单独类型化证据确认、仅为候选验证方向”，直接阻塞关系未建立，建议先验证同步对象。未再把 S-sleep 分类为正常帧节奏/非锁。显式窗、两维根因、根因排序、唤醒链、窗内可消除量、Trace 因果投影和自动补齐全部保留。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `EVAL-B195-PIAUTH1` 在 r130 生产闭环；S30 没有接管模型结论，模型基于就地 typed 权限自行给出校准后的诊断和验证方向。
- 登记 `EVAL-B199-DIAGDOMAIN1=P2/model-variance-watch`：图的证据载体与领域节点偶发混层。B153 同 case 已正确输出四节点，当前不以单轮波动增加硬约束；后续用异构 diagram case 观察。
- 登记 `EVAL-B199-DIAGCITE2=P2/model-variance-watch`：职责事实正确但逐项引用选择更弱 enum 行；已有 StageBinding 证据在池，暂不改系统或扫描答案。
