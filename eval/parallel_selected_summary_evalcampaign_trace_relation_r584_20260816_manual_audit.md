# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T20:45:26Z
- sweep_start_ts: 20260816-134524
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-134526 | answer_regex | none | 144s | 26 | read=0,repo_map=2,list=1,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B932 生产命中并要求模型补 exact anchors；但紧凑 capsule 把已证 `py.tokenize_bytes -> tokenize_bytes` wrapper/core 边挤出，终稿引用又错位，不能人工签 pass。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-134526 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 224s | 39 | read=3,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B931 生产闭环，完整因果补采、链上排序和双轴均保留；但模型把 blocked_reason caller 误称等待对象，并在 typed incomplete 枚举下声称“无省略”。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### `real_trace_h7_self_seat_full_spectrum`

- B931 获得生产正证。Analyzer 第一次把 `causal_diagnosis` 与有限 `fact_families`
  同时发出后被 fail-loud 拒绝；第二次保留 `scope=causal_diagnosis`，并携带
  `causal_contributor_set` 后通过。没有再被 intent/scenario 降格成有限事实车道。
- 探索只执行 2 次 `trace_query`（`window_stats + root_cause_rank`），最终仍形成完整
  `Trace 因果投影`。模型把 65.912ms 的运行供给折算、36.757ms 的 D-state、链上优先级反转、
  邻近 49.623ms background，以及业务 span 的原始占用分开表达；邻近席未晋升链上主因，
  “实际占用/新修向”与“现有规则可消除量”双轴没有丢失。
- 新确认 B933（高 ROI、上下文精度）：typed census 已明确给出
  `dma_fence_default_w` 是 kernel call-site，不能据此推断等待对象/资源持有者/子系统；但该语义只在
  早期大段 guidance 和系统后置投影中出现。最终成文尾部的
  `blocked_reason_state_relation` 只重放“记录数/Σdelay 与 sched_switch 墙钟不可相加”，漏掉
  call-site 身份上限。模型因此在 summary/state/table 中把 caller 越权写成“DMA fence 等待对象”和
  “由该对象引起”。这不是 trace 缺证，也不是答案 JSON 问题。
- 新确认 B934（同根显著性）：`Runtime Enumeration Authority` 在 Observation Ledger 之前已发布
  `status=incomplete`，系统投影也诚实披露 `root_cause_rank 12/53、12/67` 等边界；但最终 typed
  decision tail 没有重放该权限。模型仍写“贡献很小来源已全部列入，无省略”。最优修复是把同一
  typed authority 的紧凑权限放到最终尾部，不新增模型字段、不扫描答案原文、不由系统改答案。
- Runner PASS 不能覆盖上述两项事实越权，因此人工判 `partial`。

### `mr_poly_binding_chain`

- B932 获得生产正证。模型第一稿 principal ordered list 声明 `call_edge/registration_edge`，却没有
  `edge_anchors`；系统只发同轮 typed 修复提示，模型自行从 recipe 复制了三条 exact anchor，未强制
  Mermaid、未由系统生成关系，随后通过。
- 新确认 B935（跨语言、非 Python 特例）：生产 evidence 已含
  `py.tokenize_bytes -> tokenize_bytes`，但 registered-export handoff 在 direct exact-export 分支保留了
  binding 的短名 `tokenize_bytes`，没有用同文件、同 owner、已证 wrapper caller 精化成
  `py.tokenize_bytes`。随后紧凑关系优先级把所有 `tokenize_bytes -> helper` fan-out 误当成 export
  incident edge，8 席 cap 挤掉真正 wrapper→core 边。终稿只能靠文字重述该 hop，typed anchor 中缺席。
  根修应在 handoff 构造时用已有 exact owner/reference join 精化短 callable，且继续对歧义 fail-closed；
  该方案对 PyO3、JNI/NAPI/FFI、ArkTS/Cangjie native binding 等同构。
- 终稿另有引用错位：wrapper→core 行引用注册行 47，core→best_merge 行引用 wrapper 行 42；已读过的
  `_tokenize_slow` 定义又被说成证据未包含。它们记录为后续 citation/typed-support 消费观察项，不能用
  case 字符串重绑或强制留文掩盖 B935。
- 人工判 `partial`。

## 红线复核

- 两案均无畸形 JSON、旧稿降级、空答案或 active-stream 固定 4ms/累计年龄降级。
- 本轮系统没有删除、替换模型结论，没有生成关系或图；B932 的修复由模型同轮提交。
- Trace 显式窗、因果投影、自动补齐、链上-only 主因、背景 support-only，以及占用/计价双轴保持。
- 后续修复只消费 typed census、typed enumeration authority、typed source owner/reference/edge；禁止使用
  用户题面或模型/最终答案关键词作为硬门。
