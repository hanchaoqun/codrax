# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T19:03:44Z
- sweep_start_ts: 20260806-120342
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-120344 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S18 闭环：首稿即发 kind=summary+principal+bounded caliber，0 reject/0 patch，显式窗、双轴结论与系统投影均在。语义仍越权：把 D-state 10.433、io_wait 7.386、io_latency 6.673 相加为 24.492ms；把代表段/包络关系升级为约95ms共同持锁竞争；在 frame evidence absent 时把唤醒频率牵到 VSync。维持模型关系综合 gap，不增正文扫描硬门。 |
| 2 | github_issue_libgit2_foreach_worktree | FAIL | eval/results/github_issue_libgit2_foreach_worktree-20260806-120344 | write_apply,write_patch_oracle | none | 249s | 20 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | S17b 结构闭环：plan2 只有当前 rebase fallback，累计域无旧 fallback，controller task/header/goal 均为当前代。交付仍错：plan1 把原 `!= 0` 改成 `< 0`，虽通过不完整的三测试，却改变正非零 callback 的原谓词，runner oracle 正确判 FAIL。根因在更早的 write-analyzer：首稿两处修法均正确，仅因 exact contract 缺 evidence_ref 整份打回；重试未携带前一 typed payload，模型全量重填时丢失正确谓词。登记局部合同失败导致全量语义重写 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
