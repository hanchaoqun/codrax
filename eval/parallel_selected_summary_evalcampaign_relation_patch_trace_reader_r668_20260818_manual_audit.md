# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T07:28:06Z
- sweep_start_ts: 20260818-002804
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260818-002806 | answer_regex | none | 182s | 26 | read=3,repo_map=5,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | Python guard、`_fastlex.tokenize_bytes`、PyO3 wrapper→Rust core→`best_merge`、ImportError 回退与精确定义引用均完整，且关系以八步有序列表稳定可见，Finalizer 0 reject。B1050 本轮没有 patch，故只能记 no-regression/not-triggered，不能伪称 sparse patch 获生产正证。末尾又说 `_HAVE_NATIVE` 初始化逻辑“未被读取文件覆盖”，与已读 tokenizer.py:1-5 和正文 ImportError 说明冲突；本质仍是 B1048 branch role 未触发导致该分支没有进入 typed evidence，暂不靠题面关键词硬推导。 |
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-002806 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 206s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial-system | B1051 读者卡真实进入最后成文缝且窗口、目标五态、worker-200 8.300ms、背景权限均用自然语言；模型正文只剩 caveat 一处括号复制 raw token，未再泄漏整串控制值。但事实卡把 target 自身 rank-0/effective-0 的 10ms sleep causal-impact 行因处于 OnChainCauses 误写为“可参与主因”，模型随即写“两者均计入链上主因推理”，而系统投影明细正确写成症状。确认 B1052：目标自状态症状必须按 typed target identity+零 rank/effective 与语义工作分离。显式窗、投影、补采、worker 根因 #1、跨 CPU、双轴和背景不晋升均守住，无 4ms 降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized findings and disposition

- `B1050-SPARSERELATIONPATCHCONTENTLOSS1`：实现保留；r668 0 patch/0 reject，只证明普通首稿无回归，未触发 sparse metadata repair，继续等待生产正证。
- `B1051-TRACETYPEDREADERLANGUAGE1`：last-seam 读者卡已生产接线；绝大多数内部值退出模型正文，但一次 raw 括号回显仍存在。禁止以输出关键词扫描/硬拒/系统改写闭环，先修下述精确信号错误再异构复放判断模型波动。
- `B1052-TARGETSELFSYMPTOMREADERCARD1/P1-high`：事实卡不能仅以 OnChainCauses bucket 判断根因资格。目标主体、rank=0、effective=0、且无 semantic/span carrier 的自状态是症状；正 rank 的目标 runnable/D/IO/compute-supply 与目标确定性语义工作仍可保留。已按 typed 字段施工并加正负 pin。
- `B1048-REQUESTEDBRANCHREACHABILITY1`：r668 仍为 implemented/not-triggered；最终回退结论正确，但 import branch 未成 typed evidence，造成末尾自相矛盾 caveat。继续改 analyzer role 的软教学/异构覆盖，不用请求或答案关键词硬门。
