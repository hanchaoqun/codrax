# Trace 分析开放 gap 账本（2026-07-10）

本账本是本轮 trace correctness / capability 后续项的落盘入口。状态依据当前工作树审计，不把“保守拒绝配对（fail-close）”误写成“已具备精确解析能力”，也不把尚无生产 witness 的理论风险写成已发生 bug。

## 状态口径与批次顺序

- **已修已推送**：生产代码和回归 pin 已进入远端 `main`；状态栏同时记录首个闭环提交 SHA。
- **已修待提交**：生产代码和回归 pin 仍只在当前工作树，需在对应小批验证后立即提交、推送。
- **部分修**：错误结果已被 fail-close 阻断或披露，但更精确的 typed 能力仍缺失。
- **开放**：代码审计已确认剩余能力/作用域缺口，可排开发批；若依赖未知生产格式则仍需 witness 后施工。
- **witness 触发**：当前行为是保守或信息欠披露，尚无足够生产证据证明值得改变语义。满足下表触发条件后再立实现批。
- **结案**：已具备精确能力和回归保护，当前无剩余施工项。

建议 owner batch 顺序：`B0-current-correctness` → `B1-io-request-identity`（有格式 witness 才开）→ `B2-integrity-scope` → `B3-en-hygiene`；其余进入 `W-production-witness` 池，不抢占 correctness 批。

## 高 ROI 主队列

| 项 | 当前状态 | 代码 / 测试证据 | 剩余定义（fail-close 与精确能力分开） | 触发 witness / 推荐采集 | ROI / owner batch |
|---|---|---|---|---|---|
| **BLIND-3：`C|` counter 结构化解析** | **已修已推送**（`5d91b433d`） | 解析与 series 预算：`internal/tracequery/trace_counter.go:35-185,231-430`；接入 `WindowStats`：`internal/tracequery/query.go:2128-2147`；wire：`internal/tracequery/types.go:2068-2149`；回归：`internal/tracequery/trace_counter_blind3_test.go:14-396`、`internal/tool/trace_query_counter_blind3_test.go:10-49` | 已有精确 typed identity（物理 source + payload owner/scope + name + trailing tag）、有限数值校验、首个窗内样本 baseline、8192-series 超限整族 fail-close、invalid/non-numeric 有界披露。**剩余不是 BLIND-3 本体**：单位仍诚实标为 `unknown`，窗头前值不猜，不能把 delta 当绝对资源成本。 | 当前客户 `C|23106|Heap size |(KB)|205455` 已覆盖。复核：`./codrax --tracediag examples/tracediag/collect_berlin_pairing_witness.yaml --trace <trace> --out berlin_pairing_witness.txt`；关注 `counter_quality`，若 `series_budget_exceeded=true` 缩窗。 | **高，B0-current-correctness** |
| **block/storage 同粗键并发的 typed request identity** | **部分修；能力仍开放** | block 只认精确 rq/bio endpoint 和 `family/dev/op/sector/len`：`internal/tracequery/block_pairing.go:24-147`；同粗键重叠整 cohort 抑制、拒绝 FIFO 猜配：`internal/tracequery/block_pairing.go:150-353`；generic storage 同样整 cohort 抑制：`internal/tracequery/query.go:8059-8223`；回归：`internal/tracequery/block_storage_pairing_test.go:69-158` | **已修的是正确性防线**：跨 artifact 不配、粗键并发不铸时长、有 caveat。**未修的是精确能力**：尚未从原始载荷解析并验证 `rq/request_id/req/mrq/bio/cookie/tag/cmd_tag/task_tag/unique_tag` 等稳定 request token，因此会保守丢弃可配对 cohort。没有生产 token 形态前不能臆造字段优先级或跨 family join。 | 使用 `collect_open_gap_witness.yaml` 的 `raw_io_pairing_rows`；必须回传同粗键第一个 start、第二个 start、全部 done 及前后各 5 行。立案条件：两端存在稳定同名 token，或 completion 顺序无法由粗键唯一决定。操作细节见 `docs/design/trace_witness_collection_playbook_20260710.md:45-56`。 | **高；P1，B1-io-request-identity（witness 后）** |
| **P2：非 scheduler malformed endpoint 完整性** | **Workqueue/DMA 已结案并推送**（`d729f634f`）；**generic storage 仍部分修** | 既有 scheduler/trace-mark/IRQ/block 门保持；新增 `internal/tracequery/workqueue_dma_integrity.go` 与 `workqueue_dma_integrity_test.go`。Workqueue 只认 exact `workqueue_execute_start/end`，硬身份=`PID + 有效 work pointer + physical source`；DMA 只认 exact `dma_fence_wait_start/end`，硬身份=`PID + driver + timeline + unsigned context + unsigned seqno + physical source`，`signaled` 不闭 wait。raw event-column fallback 在 Event admission 前捕获坏 PID/CPU/时间戳/字段，affected family fail-close；同键并发整 cohort 抑制且归零后恢复；typed tuple 用 NUL 分隔避免自由文本碰撞；function 多值显式标 `multiple`。三包回归：`go test ./internal/tracequery ./internal/tool ./internal/types -count=1`。 | **已关闭** Workqueue/DMA 的 malformed/FIFO 猜配正确性缺口，并保留 vendor inventory。**剩余只属于 generic storage**：当前会以 layer/base/dev/inode/op 等粗身份整 cohort 抑制，尚无生产 schema 证明哪些 request token 可作为 required hard identity；在 witness 前继续 fail-close，不能把 `unknown/-` 升为可配对身份。 | 通用模板已新增 `raw_workqueue_dma_rows`，仅用于厂商兼容扩展；generic storage 仍采 `raw_io_pairing_rows`。storage 立案样本至少含合法 pair、缺关键字段 endpoint、同粗键并发 cohort，并保留两端 token 相等关系。 | **中高；P2 correctness 已缩至 generic storage，B1-io-request-identity（witness 后）** |
| **P2：统一 CPU ID 合法性** | **已修已推送**（`5d91b433d`） | 单一合法范围与 strict scalar/set parser：`internal/tracequery/cpu_input_integrity.go:10-187`；全局 header CPU、cpu_id/target_cpu/orig_cpu/dest_cpu/affinity/perf CPU 有界 witness：`internal/tracequery/cpu_input_integrity.go:189-350`；ParseLine 对非法 header 全行拒绝：`internal/tracequery/parse.go:2393-2421`；回归：`internal/tracequery/cpu_input_integrity_test.go:11-188`。 | 已做到 malformed/负数/溢出/`>4095` 不回落 cpu0，非法显式 `cpu_id` 不回落 row header，cpuset 全有或全无、扩展有上限，并有 query caveat。当前未发现仍绕过 strict parser 的生产 CPU attribution consumer；后续新增 CPU 字段必须复用这些 helper。 | 不需新生产 witness 才能合入。验收：`go test ./internal/tracequery -run 'TestCPUInput' -count=1`；客户 trace 可检查报告是否出现 `cpu_input_integrity_degraded=true`。 | **高，B0-current-correctness；已结案** |
| **P2：收窄无关 TID-reuse 影响** | **部分修**：`perf_timeline`（`b303c3fd5`）+ Workqueue/DMA（`6405c94cf`）+ direct resource/plugin 六族（`7af38bc23`）已推送 | contributor-scoped PID-set guard：`internal/tracequery/thread_incarnation_guard.go`。`perf_timeline` 从实际 sample 建依赖集；Workqueue/DMA 及 `BIO/Filesystem/PageFault/Ability/XPower/HiSystem` 直接摘要均从同一次严格 in-window admission 返回的实际 kind 收集完整正 PID 集，按 family 独立 gate。无关复用、单族贡献者复用、PID=0、跨族不连坐与 lifecycle-audit cap 回归：`thread_incarnation_perf_scope_test.go`、`identity_resource_failclose_test.go`。生命周期判定只认 `sched_wakeup_new` / X/Z 后再现，不用 comm 漂移。 | 已关闭 perf、Workqueue/DMA 与六个直接摘要族的无关 TID 误伤；贡献 PID 复用只压本族，audit cap 对非空 PID 集继续 fail-close，PID-less family 不虚构冲突。**剩余**是全局/复合消费面：WindowStats/off-CPU/CPU pressure 本身是全局人口统计；scheduler latency 依赖全局 busy/pressure/competitor context；FileIO/PageCache/storage 继续派生 TopIOInodes/IOPressure/BlockIOByInode/IOBurst/root rank。若只收窄早期 guard，会把被抑制 context 伪装为数值 0。 | scheduler/CPU context 先增加 typed completeness；block carry-in 依赖集须由 pairing result 携带，FileIO/PageCache/storage 须把完整性贯通所有复合派生物后再放宽。当前 direct 六族已吃尽无需新 schema 的安全独立面。 | **中；P2，B2-integrity-scope，复合面继续开放** |
| **P2：EN 闭集机械化** | **已修已推送**（`5da3fbed0`） | 系统文案统一：`internal/tool/answer_document_mutation_runtime.go`、`answer_document_mutation_runtime_tree.go`、`internal/agent/answer_document_evaluator.go`；全脸闭集与 raw-pass-through：`internal/tool/answer_document_projection_en_closed_set_test.go`；窗外注记 pin：`internal/agent/answer_document_evaluator_cmpb_test.go`。 | 已建立 canonical 词族：`Analysis window`、`focused thread/focused-thread`、`Trace file`、`root-cause rank #N`；退役 `Requested/Projection window` 误称、system-authored `Target/Artifact/rank=N` 混用。闭集只扫系统生成 trace supplement，原始线程/span/file label 与 raw audit `rank=N`、wire token 逐字保留。Markdown 用户面板与 HTML 同源词面有端到端 parity pin。 | 无需生产 witness 才成立；以后新增英文 trace 展示面必须扩展同一 closed-set fixture。客户若回传新混用词，只能在先判定“系统文案还是原始 payload”后入词表，禁止全局替换。 | **中；P2，B3-en-hygiene 已结案** |
| **SEM-DETAIL：链上语义项明细措辞自相矛盾** | **已修已推送**（`3522a10e6`） | 分流点：`internal/tool/answer_document_mutation_runtime_tree.go::runtimeTraceProjDetailPositionMerged`；引擎实铸 ZH/EN/HTML 与 contradictory-token 回归：`internal/tool/answer_document_projection_semlead_test.go`。 | 链上 VerifyClass/JIT/GC/Shader/Texture 等已按 SEM-LEAD 全权参与主因排序，明细不再写“优化项,非根因”，统一为“链上参与根因排序”；off-chain 控制仍保留非根因措辞。显式 `chain_relevance=background/adjacent` 权威高于陈旧 `causality=on_*`；仅 relevance 缺失时才用 wakeup/dependency causality 兼容。无需 `Rank>0`，避免 TOP N 外语义项丢失参赛/提及义务。 | 后续任何语义类展示面必须复用 typed relevance 优先级；禁止凭 span 名或自由文本判断 on-chain。 | **高；P1 correctness 已结案** |
| **HTML-DTL：因果投影明细列尾标题孤悬** | **已修已推送**（`5e441d362`） | `internal/preview/server.go` 为 `section.trace-projection-detail > h3` 增加 column break 约束；`internal/preview/markdown_trace_sections_test.go` 机械 pin。 | 宽屏/打印双栏中的每个 E# 三级标题与首个属性块保持同列，避免标题独留左列底部；窄屏单栏、Markdown 与终端不变。同步清理 `finalizer_auto_repair.go` 中已被 C15 幂等实现淘汰的旧立案注释。 | 纯 HTML 排版，不改变 AnswerDocument 顺序、因果语义或证据内容。 | **低；UX 卫生项已结案** |

## witness 触发池与已闭项

| 项 | 当前状态 | 代码 / 测试证据 | 当前裁定、触发条件与动作 | ROI / owner |
|---|---|---|---|---|
| **A3：claim binding 第四 fallback** | **witness 触发；低风险开放** | `CompileAnswerClaimBindingsFromAggregateFacts` 在 origin 空集时仍 fallback 到 current_source：`internal/types/answer_claim_binding.go:56-73`。但常规模型 fact 已投影 `system_inference`，ledger 的空集 fallback 也已改 advisory：`internal/types/observation_ledger.go:1021-1055`；回归覆盖 NegativeObservation/Excluded 不再满足 current source：`internal/types/csp_current_source_exclude_test.go:267-319`。 | 当前可达面主要是 NegativeObservation/Unknown 的**软 binding**，不喂 `CurrentSourceSatisfied`，没有已知出厂误判。只有当无 Grounded/Recovered/tool witness 的该 binding 实际改变 confirmed/rejected、retry 或主结论时才改；届时将 claim-binding fallback 对齐为 `system_inference` 并 pin 下游行为。采集需 `replay_full.txt`、最终 MD/HTML、相关 `emit_answer_document` 参数，`tracediag` 不能证明。 | **低；W-production-witness** |
| **B5：同 token 跨车道双席** | **witness 触发** | 跨 producer 的同线程同 type 行明确禁止相加：fold key 包含 source，`internal/tracequery/rank_family_fold.go:612-655`；回归 `internal/tracequery/rank_seat_ord_test.go:440-451`。精确同一 wakeup occurrence 的 causal/root twin 已在铸点只留一席：`internal/tracequery/root_cause_rank_admission_gap_test.go:13-59`。 | 数值双计防线已存在；剩余仅是 RootEvidence 与 window_stats 两种视角可能各有一行。触发：同一 E# / 同物理区间 / 同 typed subject+value 同时可见，且客户自然理解成重复计数。动作优先显示层互指或精确裁定表，不能跨 ruler 求和。采集见手册 `:58-62`。 | **中低；W-production-witness** |
| **B7：tie-rank chip 只标先见/供席窗口** | **witness 触发** | 合并行完整保存 `MergedQueryWindows`：`internal/types/trace_causal_projection.go:444-450`；但 rank chip 只取 `RankQueryWindow*`/row window：`internal/tool/answer_document_mutation_runtime_tree.go:2184-2249`。 | 这是欠披露而非伪造，详细窗 roster 未丢。触发：同一 rank 合并成员确来自多个窗口，chip 的单窗标签让用户误以为整行只来自该窗。动作：chip 展示“双窗/多窗”，或指向明细窗来源；不跨窗比较 rank。采集两个单窗报告 + 一份多窗报告，见手册 `:64-68`。 | **低；W-production-witness** |
| **B9：off-chain 语义 span 双席** | **已修已推送**（`5d91b433d`） | 当前构树先跨 on-chain/adjacent/background 精确折叠 semantic/rank twins，并禁止 off-chain semantic 进主因树：`internal/tool/answer_document_mutation_runtime_tree.go:1368-1410,2918-3009`；客户 witness 回归：`internal/tool/answer_document_projection_semlead_test.go:332-410`。 | `cust_trace_vc_710` 已成为生产 witness，故不再属于等待池。精确 join 要求 relevance lane、subject、typed semantic class、line range、value/member/window 一致；不一致 fail-open，避免误并。 | **高，B0-current-correctness；已结案** |
| **C10：fold 键缺 typed wait-object identity** | **witness 触发** | parser 已保存 `WaitObject`：`internal/tracequery/lock_contention.go:55-71,138-157`；但 contention twin fold 的硬键是 BlockingKind + holder PID + waiter PID + overlap，没有 wait object：`internal/tracequery/query.go:15458-15471`。 | 不能把自由文本锁描述直接升为硬键。触发：同 owner/waiter 下两个不同锁对象被折成一行并改变定位动作；需保留 owner、原始对象、holder/blocking site 与行区间。优先新增 parser 产出的 typed object token；若格式不足，只披露“含 N 种等待对象”，不猜 identity。采集见手册 `:76-80`。 | **中；W-production-witness** |
| **C12：blocking_span 树层级链身份** | **witness 触发 / 当前行为保守正确** | 只有 typed `BlockingKind` 且 counterpart resolved 的 blocking_span 才能直接 on-chain：`internal/tracequery/query.go:12618-12636`；显示层对无树身份行降背景/平铺：`internal/tool/answer_document_mutation_runtime_tree.go:2504-2535`。 | 无 typed 链身份就不缩进，因为缩进本身是因果宣称。触发：peer/holder/wakeup edge 已由 typed 行唯一解析，仍未入树且明显影响定位。只有时间重叠或文本相似不是 witness。采集见手册 `:82-86`。 | **中低；W-production-witness** |
| **C13：症状容器 bar 压过真实原因** | **witness 触发** | target 自身 wait-on-counterpart 已由 `tier=target_self_state`、Rank=0 排除主榜；该语义在 `internal/types/trace_causal_projection.go:838-849`，生产 rank-limit pin 在 `internal/tracequery/root_cause_rank_admission_gap_test.go:117-146`。但报告仍可展示症状时长/bar，覆盖句单独按 typed symptom denominator 计算：`internal/tool/answer_document_mutation_runtime_tree.go:8363-8488`。 | 参赛 correctness 已修；剩余是视觉基/降权记号 UX。触发：完整报告中症状容器的 bar/百分比让用户实际误判其为主因。动作只改显示基或样式，不把症状重新参赛，也不隐藏时长。回传树、关键指标、明细三处，见手册 `:88-92`。 | **中；W-production-witness / UX batch** |
| **C16：gated-only 行缺 split audit** | **witness 触发** | 当前 wire 已有 gated runnable / running-deficit / capability / topology 总量：`internal/tracequery/types.go:2396-2408,3374-3397`，因此结果值可审计；但没有“首个判型 split”的 CPU 对、时间戳与判定臂明细。 | 不是现有总量错误。触发：`freq_only`/簇不可判仍出现且跨窗分叉，用户无法凭总量复核。届时给 `WakeupCausalImpact` 增加有界 typed split-audit 样本（不是散文），并贯通 aggregate/rank/tool。使用 `collect_cap2.yaml`，见手册 `:94-104`。 | **低中；W-production-witness** |
| **BLIND-1：unknown print 载荷** | **witness 触发；采集面已补** | 通用模板已提供 `event_types: [unknown]` 有界原始样本：`examples/tracediag/collect_open_gap_witness.yaml:53-56`；Berlin 模板同样保留 unknown：`examples/tracediag/collect_berlin_pairing_witness.yaml:44-47`。 | unknown 比例本身不是 parser bug；应用自由文本长尾不值得硬解析。触发：unknown 占比稳定，top-N payload 呈可归纳的 key/field/value 结构。届时按结构新增 typed family，并保留 unknown inventory/caveat；不以关键词硬分类。采集/脱敏规则见手册 `:106-113`。 | **中（有结构 witness 时）；W-production-witness** |

## 统一采集与回访命令

```bash
./codrax version > codrax_version.txt
shasum -a 256 <trace文件> > trace_sha256.txt

./codrax --tracediag examples/tracediag/collect_format_census.yaml \
  --trace <trace文件> --out format_census.txt

cp examples/tracediag/collect_open_gap_witness.yaml open_gap_witness.used.yaml
# 编辑四个 pid/thread 和 defaults.window（建议不超过 1 秒）
./codrax --tracediag open_gap_witness.used.yaml \
  --trace <trace文件> --out open_gap_witness.txt \
  2> open_gap_witness.stderr.txt

./codrax --repo . --branch main --htrace <trace文件> \
  --request "只分析这份 trace，不分析代码。目标线程是 <comm-tid>，请分析 <start>s 到 <end>s 的卡顿根因。" \
  --log-level debug --log-stdout 2>&1 | tee replay_full.txt
```

完整操作与最小回访包见 `docs/design/trace_witness_collection_playbook_20260710.md`。回传 `codrax_version.txt`、trace 指纹、实际使用的 yaml、报告/stderr，以及成文类问题的 MD/HTML；不要回传凭据或整个 `.codrax/blob`。

## 维护规则

1. 每次相关提交必须在本账本更新状态、commit SHA（合入后补）与验证命令；不得只在聊天里销项。
2. fail-close 项只有在“错误时长不会铸造 + caveat 可见”时可写“正确性防线已修”；只有解析了稳定 typed identity 并有正/负/歧义 pin，才可写“精确能力已修”。
3. witness 触发项没有满足触发判据时不得升级成“生产 bug”；满足后应把最小脱敏 fixture 固化为测试，再进入开发批。
4. 客户原始 trace、采集输出和带业务 payload 的 fixture 不进入仓库；只提交最小合成/脱敏回归。
