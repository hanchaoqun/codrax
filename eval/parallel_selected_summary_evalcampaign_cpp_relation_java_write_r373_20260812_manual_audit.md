# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T07:56:04Z
- sweep_start_ts: 20260812-005603
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260812-005604 | write_apply,write_patch_oracle | none | 179s | 23 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 生产改动只为 `LazilyParsedNumber` 增加基于原始 `value` 的 `equals`/`hashCode`，未改测试；`make check` 的 source/static 覆盖通过。系统随后主动选择更高权限的 manifestless Java main 行为测试，但宿主没有 Java runtime，精确记为 `runner_missing` 并保持 `unverified`，没有把静态检查冒充 Java 行为绿灯。runner FAIL 是环境能力缺失下的正确 fail-loud，不应降门。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260812-005604 | answer_regex,answer_contains | none | 198s | 24 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | 答案正确识别 factory 选择段与 Logger 写入段没有已证绑定桥，并正确删除了含未证 caller/dispatch 边的可选图；但条目又断言 `sink_` 运行时实际绑定 `ConsoleSink`，与自身边界披露矛盾，并把 `stderr` 写入扩大成“每次直接落地终端”。更确定的系统 GAP 是：本题没有 `diagram_hint`，QFCallChain 固定 soft-required 的 `diagram_spine` 却把“可选图删除”发布成“已要求关系图缺失”，额外制造一次拒绝和错误用户 caveat。B626 已改为只有 typed diagram intent 才创建图 facet。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `runner=1/2`, `human=1/2`。Java 写模式是正确补丁加诚实的宿主能力不足；不得为了机器 PASS 把 source/static 提升为 Java 运行时验证。
- `B626-DIAGRAMINTENTDEBT1/P1` 已确认并修复：问题家族只定义语义，不授权展示形。无 typed `diagram_hint` 时图 facet 不进入合同；advisory hint 才是 optional；显式请求才是 hard。关系证据门保持原样，系统不补边、不代写图或正文。
- C++ 正文的绑定/落地表述属于模型对现有证据权限的服从性失败，本批不通过扫描终稿或系统改写来修；后续异构调用链继续验证 B624/B624b 的软权限是否足够，若重复再从 typed handoff 收窄。
- 两案均未触发 Trace 路径；显式时间窗、链上根因、因果投影与自动补齐未改变。本轮最长 198s，无固定总时长降级信号。
