# 客户 OOM 审计:成文 citation quote 归一化整读 1.1GiB trace(2026-07-03)

来源:客户 CLI 真实执行记录 oom_again.txt(Windows,MiniMax-M2.7,berlin.systrace 1104 MiB referenced-in-request,问题="分析 42591 滑动卡顿…不要分析代码")。探索阶段 15 轮 trace_query 分段全部正常完成,**崩溃发生在成文第 1 轮 emit_answer_document(blocks=3 citations=5)**:

```
runtime: VirtualAlloc of 1157685248 bytes failed with errno=1455   ← 1.08 GiB 单次分配,ERROR_COMMITMENT_LIMIT
runtime.slicebytetostring(..., 0x4500c3f1)                          ← 1,157,678,065 B ≈ trace 全文
tool.currentSourceCitationLines   answer_document_citation_quote_normalize.go:63
tool.currentSourceCitationLine    :47   (lineNo=0xe5e59=941,657 — 模型引用了原始 trace 行号)
tool.normalizeCurrentSourceCitationQuotes :29
tool.executeAnswerDocumentV2      emit_answer_document_v2.go:237
```

机制:模型在 citations[] 里放了 `{file: berlin.systrace, line: 941657}`(把 trace 行当 repo 源码引用——prompt 明文禁止但为软引导);系统侧 quote 归一化 pass 把它当"当前源码文件",`os.ReadFile` 整读 + `string(data)` 双份 1.1GiB → Windows commit limit 崩溃。**8 分钟已完成的探索分析在最后一步全损。**

## Gap 清单(按严重度排序)

| # | 级别 | Gap | 类(per-class) |
|---|---|---|---|
| O-1 | **P0 直接根因** | `currentSourceCitationLines`(citation_quote_normalize.go:59)无大小上限整读 + 无 per-doc 缓存(同文件 5 条 citation 重复整读)+ containment 检查≠安全(repo 目录内的巨型工件照读) | 模型可控路径→无界整读 |
| O-2 | **P0 同类洞** | `ground.readRepoFile`(scope_dispatch.go:480)同形态:evidence 接地(negative/crossfile/section)对模型给的 file 无界整读——模型发 `negative_query.file=berlin.systrace` 即同样 OOM | 同上 |
| O-3 | **P0 同类洞** | `read_file` 工具(builtin.go:4665)`os.ReadFile` 无大小守卫——分页窗口是读后再切,巨文件先整读进内存 | 同上 |
| O-4 | **P1 防线缺失** | 运行时工件路径进入"当前源码"归一化面:typed 工件身份(AttachedHitrace/bundle 源路径)存在但 citation 面不消费;应确定性跳过并留 typed 说明,而非依赖 prompt 软禁止 | 工件身份→系统兜底 |
| O-5 | **P1 防线缺失** | 用户明示"不要分析代码"(ExternalObservationCurrentSourceExclude,typed 枚举已存在)时,current-source quote 归一化 pass 仍执行:observation-only run 中该 pass 应整体惰性 | typed 意图→pass 门控 |
| O-6 | **P2 摩擦** | 探索第 1 轮浪费:referenced-in-request 工件,模型猜 source=attached_trace 被拒后自行改 path(1 轮)。单工件场景可机械修复(typed 注册表精确信号→auto-resolve+repair 记录) | toolparam 机械修复 |
| O-7 | **P2 验证项** | 探索散文以"优先级反转"为主叙事,但全窗口 runnable 仅 15.1ms(0.45%)——R5d 门控(已交付)应在投影侧把反转影响折到 ~0;答案未写完无法确证。修复 O-1/O-4 后同参数复跑验证门控在最终答案的呈现 | 已有门控的呈现验证 |
| O-8 | 观察 | write 侧多处 os.ReadFile(apply_patch/change_plan_validate/structured_edit)属 worktree 受控 plan-touched 面,不在本事故类;fieldValue 扫描(emit_investigation_complete.go:8919)有 production-source 过滤,统一 helper 时顺带收编 | 审计记录 |

红线核对:全部修复为系统侧确定性守卫(stat 大小=精确整数信号;工件身份=typed 注册路径 verbatim 匹配;exclude=typed 枚举),不新增硬门、不改 citation floor/pool 等不可软化门、不做散文匹配;OOM 是 Go fatal 非 panic,不可 recover——**预防是唯一防线**。

## 分批任务

- **Batch O1(P0,崩溃类灭绝)**:width 层加 `SourceReadMaxBytes`(单源常量)+ `ReadFileBounded`(stat-first,超限返回 typed oversized);O-1 citation 归一化改 bounded+per-doc 缓存+超限跳过(advisory 日志);O-2 ground.readRepoFile 接 bounded(超限→既有 Ungrounded note 路径);O-3 read_file 超限拒绝+typed repair(引导 grep 行窗/trace_query);测试:小 cap 合成超限文件三面 pin + 缓存单读 pin。
- **Batch O2(P1,防线)**:O-4 citation file 命中 typed 工件路径(AttachedHitrace/AttachedLog/bundle 源路径 verbatim/basename 匹配)→ 跳过 quote 归一化 + pre-emit 确定性 detach/改挂 runtime provenance(双语 caveat,复用 G6 detach 链);O-5 ExternalObservationCurrentSourceExclude → normalizeCurrentSourceCitationQuotes 整体跳过。
- **Batch O3(P2)**:O-6 trace_query source 机械 auto-resolve(单工件 typed 精确信号)+ 文档刷新 + O-7 复跑验证记录。

每批:实现→测试看护→`go test ./...` 全绿→复核→commit/push→本文档进展刷新。

## 进展

- 审计落盘(本节)。
- **Batch O1 交付(2026-07-03,1c980e5d)**:`width.ReadFileBounded`(stat-first,typed oversized 错误,零分配;SourceReadMaxBytes=8MiB 单源)接入三面——citation quote 归一化(+per-doc 行缓存,同文件 N 条 citation 单次读;超限保留模型 quote+advisory 日志;pre_emit metadata surface-term 读取面同修,普查漏网被签名改动抓出)、ground.readRepoFile(超限走既有 Ungrounded note)、read_file 工具(64MiB 整读墙,typed repair 引导 grep/trace_query)。sparse 文件测试三面 pin(超限必须在 stat 即拦截,不分配)。
- **Batch O2+O3 交付(2026-07-03,5861b4cf)**:O-4=citation 命中 AttachedHitraceSource typed 拼写(cleaned-path/basename verbatim)→ 两个 current-source 读取面跳过;**裁定:citation POOL detach 本批不做**(pool 变更与不可软化 citation floor 交互需单独裁定;尺寸墙+跳过已灭绝 OOM 类);AttachedLog 携带的是摘录内容非路径,无拼写可匹配,由尺寸墙覆盖。O-5=typed ExcludesCurrentSource(事故 run 即此形态)→ 两 pass 整体惰性。O-6=source=attached_trace 无附着 blob 但模型已给 stat 实存 trace 文件 → 确定性回落 source=path(零猜测),灭绝事故首轮浪费。全量 `go test ./...` 绿。
- **O-7 关闭(2026-07-03)**:eval 路径修正(../../customlogs)后 donghu_real 双案 2/2 PASS,R5d 门控构成在最终答案如实呈现;**O-8 关闭(f022aa67)**:8 处 write/scan 侧读取收编 ReadFileBounded。本文档全部条目交付完毕。
