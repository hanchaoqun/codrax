# 固定 1d9a9ff643b6 双例人工审计

- date: 2026-09-21T01:25:27Z
- sweep_start_ts: 20260920-182526
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_carrier_value_20260920

固定生产二进制 1d9a9ff643b6，2并行×1；机器1/2，人工0/2。以下已读最终MD、原始过程日志及schema2旁路；不改机器原始verdict，不倒签旧FAIL，不追加同版第三例。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_native_carrier_value_20260920/trace_query_wakeup_causal_io_chain-20260920-182527 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 213s | 45 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 自身1ms runnable与14/17ms sleep不再混价；仍无证断言修复方向相互独立，并把机理未证写成链上候选无根因资格 |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_native_carrier_value_20260920/trace_query_business_marker_io_chain-20260920-182527 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 294s | 37 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 引用加重复来源/线程两次硬拒后改查51ms，正文仍称50ms；投影缺失、LoadDocumentIndex缺失、链上等待有相加暗示 |

## 明确窗口：有限改善，不关闭整体答案

报告 `.codrax/output/20260920-182856.912-78079.md`。20ms用户窗、11ms链上IO、400→300→200→100关系、窗末唤醒不等于立即运行均保留。末尾事实卡自身组成已进入最终上下文，本次正文把三项1ms解释为各自就绪等待，14/17ms睡眠另列；没有再以睡眠打折解释1ms。跨CPU具体边4→3、3→2、2→1正确，不能重用旧IRQ首边错判的FAIL理由。

仍失败：正文称“两条修复方向相互独立”，缺物理独立性凭证；又将已入链的三个优先级候选概括成“不构成…根因资格”，混淆未证明锁机制与当前链上候选资格。没有据此增加关键词硬门，也未证明新必带必拒合同。schema2文件正常生成，`status=available`，四项分别11ms与三个1ms，无背景升格。系统占用表同主体重复行、可见“席/修向”等继续留账。

## 业务实例：引用被用过，但导航因重复参数失败

报告 `.codrax/output/20260920-183018.689-78095.md`。18:28:03两次模型调用同时带正确`business_span_ref`及`source=attached_trace/thread=app-main/pid=100`，window_stats、wakeup_chain均被现有“禁止任何混填坐标”拒绝。日志记录点在dispatch前，重复字段来自模型，不是系统隐式注入；模型的确违反当前教学，尚未证明合同自冲突。随后改查1..1.051，造成6+1+44=51ms套到50ms业务窗。引用供给和导航尝试均已live见证，不能说未发或没用过；也不能把§35新增跨视图支线当此失败的根因。

正文正确保留35ms读请求、31ms唤醒前睡眠、1ms调度等待和47ms后台写请求；但仍把31ms与44ms描述成“加上”，把31ms称请求的“前置”睡眠，且称47ms全部落在44ms等待区间内（实际结束1.049晚于等待结束1.045）。没有LoadDocumentIndex；Trace投影0块。旁路仍必产且合法：`schema_version=2/status=unavailable/reason_code=trace_root_cause_contract_not_active/root_causes=[]`，不是文件没写，也不是畸形JSON。

本轮extract补齐日志为`skip reason=families_present`，不能套用旧轮`no_typed_target`。completion未接受实例焦点；旧焦点→完整补齐路线仍未得到此live验收。先核验能否将精确相等的重复坐标当断言校验后安全消去，矛盾/来源代次/显式窄窗继续拒绝，不能直接忽略任意额外参数。§35.3正文验收所有权缺口也继续开放；本次机器已缺LoadDocumentIndex，没有为其倒签旧机器检查。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
