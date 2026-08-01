# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T02:56:32Z
- sweep_start_ts: 20260731-195630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260731-195632 | answer_contains | none | 70s | 18 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 完整列出恰好两个生产 caller：BuildTypedRelationQueryWithResolvedSources@219 与 TypedRelationKindsForRequest@246，均逐行绑定 internal/types/typed_relation_hint.go；无 test caller、无跨文件错绑。模型列表与系统补全列表有重复展示，但成员、关系和文件均正确。 |
| 1 | trace_query_perf_quality_simpleperf_proto_offcpu | PASS | eval/results/trace_query_perf_quality_simpleperf_proto_offcpu-20260731-195632 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 111s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | sample_kind=off_cpu、cpu=-1/cpu_known=false、sample_cpu_scope=unknown、weight_unit=ns_off_cpu_event、symbolized/simpleperf_report_proto 全部保留；正文明确 1/1 off-CPU、running=0、7000 是 event weight 而非 CPU 时长，未归因 CPU0。小残余：模型把 cpu=-1 单独解释成“不在任何核心”，严格权限应是“CPU 归属未知”，真正 off-CPU 由 sample_kind 证明；system typed caveat 正确，结论不受影响，记模型措辞波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- perf off-CPU caliber：covered。
- typed called-by relation rowset：covered。
- 新发现仅为 `EVAL-B16-PFMV1 / P3 / model prose`：CPU unknown 的单字段解释
  偶尔过强；typed system authority 和最终结论正确，不施工 prose gate。
- 下一对：multi-trace path isolation + LoopController type-relation inventory。
