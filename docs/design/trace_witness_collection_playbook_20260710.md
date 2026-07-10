# Trace 开放 gap witness 回访操作手册（2026-07-10）

适用范围：只有生产 trace 才能裁定的显示/因果/格式边界。目标是让现场同事用**零 LLM、纯只读**命令生成最小可回传证据；默认不要求上传原始 trace。

## 1. 通用采集（每个候选场景都做）

先记录构建身份与原 trace 指纹：

```bash
./codrax version > codrax_version.txt
shasum -a 256 <trace文件> > trace_sha256.txt
```

跑全文件格式普查：

```bash
./codrax --tracediag examples/tracediag/collect_format_census.yaml \
  --trace <trace文件> --out format_census.txt
```

复制通用窗口模板，按文件头注释填写目标 TID 与精确窗口，再执行：

```bash
cp examples/tracediag/collect_open_gap_witness.yaml open_gap_witness.used.yaml
# 编辑 open_gap_witness.used.yaml：四个 pid + 唯一父窗 defaults.window
./codrax --tracediag open_gap_witness.used.yaml \
  --trace <trace文件> --out open_gap_witness.txt \
  2> open_gap_witness.stderr.txt
```

新版通用模板是 `version: 2`：客户仍只填写一个父窗口，`pairing_integrity`
会从物理文件开头完整重放 block/storage 端点状态，自动选择或拆分最多 2 个
`<=50ms` 补采窗；`raw_io_pairing_rows` 按这些 typed 窗自动展开。报告顺序固定为
“自动窗发现结果 → 已解析执行计划 → 各窗证据”，并回显 candidate rank、端点
core、选窗依据和 source-universe 指纹。系统派生窗标为
`FrameWindowAutoDerived=true`，不会冒充用户显式帧窗。

`tracediag` 任一步失败会返回非零退出码，但报告仍覆盖全部独立步骤；请同时回传
报告和 stderr，不要因为退出码非零删除产物。`dependency_empty` 不会静默回退父窗；
`generated_window_compacted` 会给出 `matched/emitted`，表示已见原始行仍可用、但
不能声称 N/N 完整。大 trace 不要先扩大行帽：父窗口仍建议不超过 1 秒。

需要复现最终报告 UX 时，再跑一次真实管线：

```bash
./codrax --repo . --branch main --htrace <trace文件> \
  --request "只分析这份 trace，不分析代码。目标线程是 <comm-tid>，请分析 <start>s 到 <end>s 的卡顿根因。" \
  --log-level debug --log-stdout 2>&1 | tee replay_full.txt
```

回传本次生成的 `.codrax/output/<timestamp>.md`、同名 `.html` 和 `replay_full.txt`。不要回传 `providers.yaml`、token、模型凭据或整个 `.codrax/blob`；若需要某个 tool-call JSON，只取本手册明确点名的文件并先检查敏感信息。

## 2. 各 gap 的触发判据与回传面

### block/storage 并发请求身份

- 采集：`format_census.txt` + `open_gap_witness.txt` 的 `raw_io_pairing_rows`。
- 通用包的 2 个自动窗不足（例如报告写
  `candidate_requires_more_than_hard_window_budget` / `generated_windows=0`）时，不要
  手工猜窗；改跑专用模板，它会原子覆盖一个候选所需的最多 8 个小窗：

```bash
cp examples/tracediag/collect_io_pairing_witness.yaml io_pairing.used.yaml
# 只编辑 defaults.window；无需 PID
./codrax --tracediag io_pairing.used.yaml \
  --trace <trace文件> --out io_pairing_witness.txt \
  2> io_pairing_witness.stderr.txt
```

- 阅读顺序：先看 `candidate kind=ambiguous_closed`（真实同键并发）；若不存在，
  `schema_probe` 是完成 pair 的格式样本，不等于已观察到同键并发。只有
  `complete=true`、`identity_complete=true` 且各执行实例没有
  `generated_window_compacted` 时，自动包才是所选 candidate 的完整端点 witness；
  这仍不是“整个父窗不存在其它格式”的证明。
- 关注：同一 family/dev/op/sector/len（或 storage layer/base/dev/inode/op）在前一个 start 未闭合前再次 start；以及原始载荷里的 `rq`、`request_id`、`req`、`mrq`、`bio`、`cookie`、`tag`、`cmd_tag`、`task_tag`、`unique_tag`。
- 立案 witness：同粗键并发后 completion 顺序不能仅凭 FIFO 唯一判断，或 start/done 两端存在可稳定对齐的显式 request token。
- 必须保留：event name、原始 key 名、token 值是否存在（可一致替换 token 内容，但不能把“缺失”和“0”混为一类）、时间戳、物理行号、artifact 名。

### workqueue / DMA fence 厂商兼容扩展

- 采集：通用包内已独立提供 `raw_workqueue_dma_rows`，无需再改写 IO step。
- 当前正确性基线（`d729f634f`）：Workqueue 只以精确 `workqueue_execute_start/end` 和 `PID + work pointer + physical source` 配对；DMA 只以精确 `dma_fence_wait_start/end` 和 `PID + driver + timeline + context + seqno + physical source` 配对。`dma_fence_signaled` 只作瞬时 inventory；同 typed key 并发整 cohort 抑制，不作 FIFO 猜配；缺字段、坏 PID/CPU/时间戳及解析拒绝均按 affected family fail-close。
- 新立案条件只限**兼容能力**：生产 trace 使用不同的厂商事件名/字段名，或标准字段之外存在稳定 typed token 且当前报告保守抑制了可唯一配对的 cohort。不要因看到 `pairing_suppressed` 就要求放宽硬门。
- 回传：合法 pair、并发 cohort 前后至少各 5 行，包含第一个 start、第二个 start、全部 end；字段值可一致脱敏，但必须保留“字段存在/缺失”和两端相等关系。

### B5：同 token 跨车道双席

- 采集：通用包 + 最终 MD/HTML。
- 立案 witness：同一 E#、同物理行区间或同 typed subject/value 同时出现在 wakeup/root evidence 与 window/rank 榜，用户会自然理解为重复计数。
- 回传：两处可见位置、对应 E# 明细和 `open_gap_witness.txt`；不要只截一张图，否则无法核对值是否真的重复。

### B7：tie-rank chip 只显示先见窗口

- 对每个窗口分别复制并运行一次通用包，产物命名为 `window_a.txt` / `window_b.txt`；再跑原多窗对比问题生成 MD/HTML。
- 立案 witness：同一 rank 合并成员确实来自两个不同窗口，但 chip 只标其中一个窗口，造成来源欠披露。
- 回传：两个单窗报告 + 一份多窗报告；同时给出两个精确窗口端点。

### B9：非链语义双席

- 采集：通用包的 `root_cause_rank_snapshot`、`raw_trace_mark_rows` + 最终报告。
- 立案 witness：同一 VerifyClass/JIT/Shader/Texture 等物理 span 在非链语义面和 rank/detail 面各占一席，且没有“同源观测/链上并入”说明。
- 回传：span 的 B/E 或 S/F 原始行、两个 E#、报告中两处位置。

### C10：wait-object 身份不足导致误折叠

- 采集：`critical_blocking_snapshot` + `raw_trace_mark_rows` + 最终报告。
- 立案 witness：同一 owner TID 下至少两个不同锁/条件变量描述被折成一行，客户定位动作因此改变。
- 必须保留：owner tid、等待对象原文、blocking-from/holder-site（若有）、各自行区间。等待对象文本只作披露 witness，不应直接成为新的硬折叠键；后续实现仍需 typed token。

### C12：blocking_span 缺链身份、树过平

- 采集：`wakeup_chain_snapshot`、`critical_blocking_snapshot`、`root_cause_rank_snapshot`。
- 立案 witness：blocking span 的对端/唤醒边/holder 能由 typed 行唯一对上，但该节点仍只能落平铺席，明显阻碍定位。
- 反例：只有时间重叠或文本相似，不足以要求树层级；因果缩进不能靠显示层猜测。

### C13：症状容器 bar 压过真实原因

- 采集：通用包 + 最终报告。
- 立案 witness：目标自身 sleep/binder_wait/blocking_span 等症状容器的 bar/百分比在视觉上压过已经核实的上游原因，且用户据此误判主因。
- 回传：树、关键指标、逐节点明细三处完整片段；注明“期望显示基/降权记号”，不要只给最终结论句。

### C16：gated-only 行缺 split audit

使用现成 CAP 脚本：

```bash
./codrax --tracediag examples/tracediag/collect_cap2.yaml \
  --trace <对应trace文件> --out cap2_report.txt
```

- 立案 witness：gated-only 供给行仍出现 `freq_only`/簇不可判，但没有首个 split 的 CPU 对、时间戳和判定臂；或相同 trace 不同窗判定继续分叉。
- 回传：`cap2_report.txt` 全文。不要只回传 cpu_frequency grep，因为缺少消费面的折算判定。

### BLIND-1：unknown print 载荷

- 先跑 `collect_format_census.yaml`；新版通用包还会用
  `raw_unknown_print_rows`（`event_types: [unknown]`）直接保留有界原始样本。
- 若已用旧版六步模板采集、报告只有 unknown 计数而没有样本，可复制通用模板后
  只保留 `raw_unknown_print_rows` 一步重跑；无需重采整份 trace。
- 立案 witness：unknown event/print payload 占比稳定且 top-N 呈现可归纳的结构化 key 形，而非应用自由文本长尾。
- 回传：event-name census、unknown samples、trace-mark shape census 三段。可脱敏业务字符串，但保留分隔符、key 名、字段个数和值类型。

### Berlin 大 trace：async/counter/interrupt/block 定向复核

Berlin census 已确认约 1022 万 events，包含 S/F async、C counter、完整
IRQ/IPI/softirq 族及大量 block endpoint。不要用全 35s 的 counter 结果代替目标帧
判断；先把模板里的 `window` 和 `pid` 改成 ≤1s 的目标窗口，再执行：

```bash
./codrax --tracediag examples/tracediag/collect_berlin_pairing_witness.yaml \
  --trace berlin.systrace --out berlin_pairing_witness.txt
```

模板分别保留 `C|`、`S|`、`F|`、interrupt、block 与 unknown 原始样本，并在
`window_stats` 发布 `counter_quality` 及 pairing caveat。若出现
`series_budget_exceeded=true`，表示该查询窗的 counter identity 已超过有界候选
宇宙，派生 Top-N 按设计 fail-close；请继续缩窗，而不能据此写“该帧无 counter”。

### A3：binding fallback（非 trace 格式问题）

`tracediag` 无法证明此项；需要真实全管线控制面：

- 回传 `replay_full.txt`、最终 MD/HTML，以及该 run 中 `emit_answer_document` 参数 JSON。
- 立案 witness：NegativeObservation/Unknown 模型 fact 在没有 Grounded/Recovered/tool witness 时，仅凭 binding fallback 改变 confirmed/rejected 或主结论。
- 不要上传全部 blob 目录；只取相关 emit 参数与最终结果，删除凭据和无关用户数据。

## 3. 最小回访包清单

建议目录：

```text
witness_<scene>/
  codrax_version.txt
  trace_sha256.txt
  format_census.txt
  open_gap_witness.used.yaml
  open_gap_witness.txt
  open_gap_witness.stderr.txt
  replay_full.txt                 # 仅显示/成文类 gap 需要
  report.md                       # 仅显示/成文类 gap 需要
  report.html                     # 仅显示/成文类 gap 需要
  README.txt                      # 目标 TID、窗口、复现问题一句话
```

`README.txt` 至少写：设备/平台类别、trace flavor、目标 `comm-tid`、精确窗口、实际看到什么、期望什么。只要这些文件能复现判据，就不需要原始 trace；若仍不足，再单独协商最小行区间，而不是默认索取整份客户数据。
