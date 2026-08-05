# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T21:05:41Z
- sweep_start_ts: 20260805-140540
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260805-140541 | log_regex,answer_regex | none | 41s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 四项命令观察均真实、覆盖完整；但把 137438953472/2^30 的结果标成 128 GB 而非 128 GiB，并从 Built-In/产品名额外推断“集成 GPU、统一内存、专业级工作站”，超出观察权限。runner 的宽松 regex 未覆盖量纲和推断边界。 |
| 1 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260805-140541 | write_plan,write_patch_oracle | none | 89s | 19 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终单行 patch 正确、验收命令合理。过程先发 unsupported bash probe，随后用 Go `exec.Command(g++)` 包装绕过运行时枚举；verification_probes 本应省略。另发现 change-plan skill 内嵌与该 case 同形的 retrun→return/greet 完整示例，导致用例表现被教学污染，必须删除后再看泛化回放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
