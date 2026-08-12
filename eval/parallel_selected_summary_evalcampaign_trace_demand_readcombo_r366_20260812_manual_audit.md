# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T05:42:44Z
- sweep_start_ts: 20260811-224242
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-224244 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 165s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 模型把两个已披露区间重叠、不可直加的链上席 23.994ms 与 19.041ms 相加为 43.035ms，并把 priority-inversion candidate 升级成已定的调度阻塞机理；其余显式窗、typed 链、链上/邻近/背景、实际占用/现规则可消双轴、业务线索、算力正值次席、因果投影与自动补齐完整。相同精确上下文曾通过，按模型波动留档，不新增正文扫描、硬拒或系统代写。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-224244 | trace_attachment,answer_regex | perf_triage+trace_query | 475s | 42 | read=8,repo_map=1,list=0,trace=1,source_lens=1 | midloop=11,inv=2/0,fin_reject=7,unavail=1,prune=0 | fail | 确定性系统自冲突：模型首稿只有一个机制 ordered_list，但 pre-emit 又按 supporting member_set 自动追加一个 system-generated ordered_list，随后 RequiredBlocks MaxCount=1 把模型连续拒绝；错误提示不暴露系统 block，模型无法修复。最终模型列表被挤掉，系统清单替代关系面，且答案仍把 recovery 分支写成 normal path、把 flavor vote 写成事件语义分类、把 pairing key 错写成 PID+name+timestamp。首轮另受 projected claim_form enum 与静态说明矛盾影响。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### real_trace_d4_demand_vs_supply

- 结构 runner 通过，人工失败。模型明确收到“跨席不可直加/区间重叠”和 candidate 机理上限，却仍写出
  `23.994 + 19.041 = 43.035ms`，并据此强化主因措辞；这是结论层模型波动，不是 Trace typed 投影缺字段。
- 不新增基于用户原文、模型思维或终稿字符串的 hard gate，也不由系统改写模型结论。显式时间窗、链上根因资格、两种耗时维度、D/IO、调度/算力供给、
  确定性语义工作、链上业务线索与非链背景降级均在本轮答案中可见。

### read_combo_trace_current_source_explanation

- 第一次完整成文因 caveat 使用 `text_reference_fact` 被 schema 拒绝。该 form 在通用说明中被正向解释，但本 dispatch 动态
  `claim_form.enum` 不包含它，模型当场指出“description says it, enum does not”。这是系统 JSON 教学自冲突。
- 第二次完整成文只有 `summary + ordered_list + caveat` 三块，模型只写了一个 `span-parse-chain`。随后
  `normalizeAggregateMemberSetCarriers` 从一个 current-source diagnostic mechanism `member_set` 自动铸造 7 行
  `AnswerSystemGeneratedPrincipalEnumerationRows`，同为 `ordered_list`。`preCheckRequiredBlocks` 于是按 `MaxCount=1` 报“两块”。
- patch 提示不携带系统生成 block ID；模型连续五轮只能替换自己看得见的列表。删除模型列表后 patch 一度通过，证明剩余列表来自系统；下一轮为了满足
  `MinCount=1` 再加模型列表，又被同一 max 拒绝。475s 主要来自这条确定性重试风暴，不是单次活跃 stream 超龄。
- 最终系统成员清单挤占了模型关系链，系统又披露“主路径上的关系未完整呈现”。这违反模型答案所有权边界，不能让模型为系统自己制造的配额冲突负责。
- 机制内容仍不正确：`restoreCarvedTraceMarkPayload` 是兼容恢复分支，不是所有行 normal path；`event_harmony_render` 是 flavor voting，
  不是 RenderService span 语义分类器；sync pairing key 是 physical source identity + emitter PID，不含 span name/timestamp。

### 已冻结的泛化根修

- `B613-AGGREGATECARRIERCARDINALITY1/P1-high`：统一 typed “current-source diagnostic mechanism narrative”判据；无显式穷举/关系/源码操作点/变更影响集合义务时，
  explorer member_set 只作 supporting coverage，不能自动铸造可见主列表或与模型 required block 争配额。可见性降级不得放松 current-source
  `support_refs` 责任。
- `B614-DYNAMICSCHEMAPROSECONFLICT1/P1`：动态 JSON enum 是 conditional field 的唯一可用值权威；通用 description 不得正向教授当前 enum
  未暴露的 claim form。
- `B615-ACTIVEHEARTBEATAGEDEGRADE1/P0-redline`：当前 OpenAI SSE adapter 的 `2×request_timeout` transport-only total cap 在默认配置下约四分钟；
  heartbeat-active、但暂未公开 reasoning/content 的网关会被按 elapsed 中止。累计 age 不是精确信号，必须撤销该 adapter 发射臂，只保留 byte-silence
  first-byte/stall、transport failure、caller cancel、安全/解码失败等 typed 退出条件。
