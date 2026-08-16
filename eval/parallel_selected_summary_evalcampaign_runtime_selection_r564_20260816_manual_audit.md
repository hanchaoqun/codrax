# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T11:38:39Z
- sweep_start_ts: 20260816-043837
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_answer_document_tools | FAIL | eval/results/read_combo_answer_document_tools-20260816-043839 | answer_regex,answer_contains | none | 113s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B898 字段存在门生产生效：第三轮因整字段缺失被拒，第四轮补出 carrier。但 generic retry 只要求补字段，模型机械填 `runtime_selection_required=false`，没有重做“首次完整 vs retry patch”的当前请求判定。Explorer 因此只读 3 文件，未形成 runtime-selection handoff；终稿继续把两工具适用时机写成固有规则并因所有关系边无 typed 证据而删除用户要求的 Mermaid。Runner FAIL 与人工 fail 一致。 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-043839 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 216s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B899 获生产正证：r563 的三个错误 `completeness=incomplete` 算术附注全部消失；模型精确保留 233.190ms 状态账、8 个 CPU 累加=157.248ms、CPU4 同核 policy + target-running 但缺 slice-overlap，因此目标受限未证。Runner 因词距/同义形未命中是假阴性候选。人工仍不能 pass：表后写“大部分时间处于 Sleep”，与同页 Running=67.4% / Sleep=30.2% 直接矛盾；并泄漏 `state_partition_coverage=complete`、重复 section 标题。单轮更像模型成文波动，禁止系统改写，先留观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `B898-RUNTIMESELECTIONCARRIEROMISSION1` 的存在性修复获得生产正证，但不能收账。字段级 repair 没有重申
   selection true/false 的精确决策，模型为过 schema 直接补 inert false。新件
   `B900-RUNTIMESELECTIONMISSINGFIELDREPAIR1`：只在 missing roster 含 `call_chain_endpoints` 时追加软教学，
   要求按 CURRENT request 重判，明确禁止“为补字段默认 false”，并重申 initial/full versus
   retry/error/patch 与 verbatim quote 形；不新增任何 request/model/final prose 硬门。
2. Combo 的一次 Finalizer reject 是正确的：Explorer 没证明 evaluator→signal、signal→full/patch 等方向边，
   系统不能为满足 Mermaid 要求造 call/guard/data-flow。图删除是上游 selection 调查未激活的级联，不应降低
   relation validator。
3. `B899-ARITHENUMERATIONCROSSQUERYTAINT1` 获生产闭环正证。完整 target state 三个比例不再借用 capped
   event_search 权限；没有新增答案改写或拒绝。状态总账与同 CPU 供给 capsule 均准确进入 Finalizer。
4. 冻结 `B901-TRACESTATEDOMINANCEPROSECONTRADICTION1` 为观察项：模型同页先给 Running=67.4%、
   Sleep=30.2%，随后声称“大部分时间处于 Sleep”。typed 上下文足够且第一段正确，当前只出现一次，按模型
   波动处理；不得扫描最终文案后替换结论。若异构回放重复，优先压缩成语言内的 typed ordered-state capsule
   与软算术教学，而不是系统接管答案。
5. H4 runner 两条 limit oracle 对正确的“策略记录 + 缺目标 slice overlap + 因果不足”词形仍未命中，记录为
   oracle 假阴性候选。不能为 runner 绿而强迫固定短语；后续若修改只能围绕 typed 语义成员做结构 oracle。
