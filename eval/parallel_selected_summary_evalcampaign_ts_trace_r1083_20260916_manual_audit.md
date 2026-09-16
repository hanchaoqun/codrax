# r1083 人工审计：TS workspace 调用链与真实短窗 Trace

- date: 2026-09-16T09:05:39Z
- sweep_start_ts: 20260916-020539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

清洁已推送473a1669e构建，09:10:13Z结束，恰好两例并行、各一次。原read15步、CAP5未用于本对。机器汇总保持原样，不追绿重跑、不回填oracle。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_a5_excerpt_degenerate_window | PASS | eval/results/real_trace_a5_excerpt_degenerate_window-20260916-020539 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 123s | 30 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 状态/覆盖口径解释错误；另有B1709、B1710系统供给缺口 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260916-020539 | answer_regex,answer_contains | none | 274s | 32 | read=12,repo_map=2,list=1,trace=0,source_lens=1 | midloop=6,inv=3/0,fin_reject=4,unavail=0,prune=0 | FAIL | 主链正确，重试次数/条件错误；第四次拒绝为B1711系统修补缺口 |

## Trace：事实基线与原稿

原稿`.codrax/output/20260916-020740.147-68750.md`；上表结果目录的原日志`run-1.logs/codrax-20260916-020541-000-68750.log`。附件`eval/fixtures/real_traces/donghu_short_excerpt.systrace`，持久化blob行号比附件多1。

- 请求2942.200–2942.300共100ms，附件实际覆盖2942.244845–2942.245401，仅556µs。采集外99.444ms；另开头10µs在采集内但目标状态未知。终稿L26把目标未归账99.454ms统称“没有任何trace记录”，错误。
- 附件L18→55 CPU8 Running367µs；L55→68/69 D→wake91µs；L68→100 Runnable88µs。目标已知三段546µs，不能称“两段”或把88µs括入91µs的IO等待。实际switch-in均CPU008，wakeup target_cpu000不是运行CPU；终稿没有犯后者错误。
- L53 issue→L67 complete为99µs请求驻留，与91µs线程阻塞重叠，不能相加。相同dev12,48/opRS/sector208305872/len8，completion与issuer唤醒闭合。请求耗时不能自动当线程关键路径耗时。
- 终稿L19“无直接磁盘阻塞（D-state为0）”错误：互斥统计已把IO等待单列，不是原D不存在。timeline JSON保有prev_state_raw=D，finalizer日志L2648明确供给不可中断等待91µs；这是模型误用，不是原始状态被删。
- 两段AssetManager::OpenAsset业务切片21µs/12µs应保留，但时间邻近不证明后续请求属于该文件。两个page-cache add分别属于inode0x81251、0x1，不能将总2页都归某个资源。

### 系统缺口

**B1709 / P1：合法任务名使块请求配对身份失效。** block_rq_issue的opaque comm为`[[GT]ColdPool#6]`；parse.go尾部正则不容内部`]`，critical payload `trace-query-result-0bdfbcb5.json` L209附近记录identity-invalid/fail-closed，99µs请求未进入IO latency供给。同型涉及BIO queue及legacy inventory。按非身份文本修正，不放宽dev/op/sector/len/source/completion status；S与D均需真实完成→issuer唤醒闭合才获链上IO权威。

**B1710 / P1：全窗背景被附着为线程等待资源身份。** computeIOBurstEpisodes把全窗IOPressureSummary的TopInode/PageCacheChurn等复制进每个线程IO/D等待episode。`trace-query-result-102f3055.json` L1923–1944因此把91µs等待附上inode0x81251/churn2；最终日志L2462–2463进一步显示object=0x81251。虽标context-only，仍混装归属与统计口径；不能全部推给模型。需独立公共回归后修复，保留全窗背景，不凭邻近铸造身份。

### 不应误报的事项

wakeup_chain用exact_tid正确选中36644；实际输出一个节点、三段状态及0.546ms。未指定min_duration_ms时默认1ms，各段均更短，故未展开因果边。evidence=0不等于无状态数据，不为本例调整阈值。

6次模型trace_query及一次系统window_stats补采，121项自动事实保留；无repo读取。perf首次把observations写成字符串数组，被准确拒绝后改正；最终成文0拒绝/0patch，无JSON降级。runner的enumeration_push=1仅命中通用“Principal Enumeration Rows”提示，不证明真实源码枚举泄漏。

本题为bounded facts，未要求完整根因投影，无图，均N/A而非能力丢失或通过。必选侧车存在，schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active，不是漏生成。

## TypeScript：主链正确，条件/次数及修图流程有缺口

原稿`.codrax/output/20260916-021011.654-68753.md`；原日志为上表结果目录的`run-1.logs/codrax-20260916-020541-000-68753.log`（含NUL，按文本审读）。

真实主链`run → ApiClient.fetchUser → HttpTransport.send → dispatchOnce → fetch`，终稿及图方向正确。FixedDelay(200,3)包含首次发送共最多3次，即最多2次重试和2次200ms等待；仅响应status>=500且仍有下一次尝试才sleep，网络异常直接传播。终稿L26“最多3次重试”及“每次失败后sleep”错误。日志L1200–1242/L2256–2298提供源码及“包含首次发送”注释，finalizer L3499–3500/L3665提供条件；未发现系统供给“3次重试”，此处不新增文字硬门。

`@app/core → packages/core/src/index.ts`映射正确，barrel的export和包tsconfig继承解释不完整。不能推导Node自动运行时alias改写。原图实际Mermaid parse/render成功，SVG28443bytes；L44–45空opt是无信息的语义空壳，不是语法错。图可选，不强加未要求的全部重试关系；函数名标签较多，业务表述仍可改善。

四次成文拒绝、四次patch分别归因：

1. L3757→3770–3832：两个无证self-call拒绝合理；**后续B1712复审纠正**：不能将其中重复call-site子项也判合理。源码for循环允许同一调用点重复运行；系统把静态证据行数硬当调用次数上限，要求删掉opt里的“再次调用”，最终空框由此形成。整稿仍因self-call需要修正，但这个子项是系统误拒。
2. L3886→3891：模型删除列表已证明maxAttempts/sleep，违反局部保全。
3. L3937→3941：同时原子修图和整块替换受保护图，权限拒绝合理。
4. **B1711 / P2**，L3973→3982–4000：模型只执行三个发布remove refs；删除第二个重复Transport→Once body时，系统连带删唯一合法anchor，使保留第一条箭头被拒。L4029 attach后L4044接受。这是确定性额外重试，不能算模型第四错。lease按pair唯一anchor分类，未区分越额body occurrence；执行器随body一起删anchor。需公共emit→opaque ref→remove回归及精确保全修复，不放松原无证边门。

另analyzer一次缺字段拒绝L599–600；无JSON恢复或严格解码重映射。系统供给足够支持正确主链/重试条件，不需新增必填字段或扫描答案文字。

## 封存与边界

23项case/fixture/runner的before.sha与after-check一致，构建输入after-check一致；结果目录原文件以`20260916-r1083-results-original.sha`封存；两MD/HTML、Trace侧车及机器汇总六项以`20260916-r1083-original-answers-summary.sha`封存，均在`.codrax/tmp/`。原稿、fixture、oracle、机评不改。

B1708本对未自然触发required-diagram/no-directed-path边界，生产N/A，不借此关闭全图矩阵。B1711只处理修补定位，不代表上游B1712静态次数门已修；其原始首拒全文见`.codrax/blob/20260916-020541-000-68753/tool-call_function_f7n19yj6q2iu_1-emit_answer_document-result-de2f45b1.txt:4`。循环/条件框的语义不可凭“重试”关键词硬放行，应取消源码关系错误的次数推断，保真实关系/方向与Trace事件计数权威。

上一批已有Python apply，本批read/read不代表写能力全面通过。600/300/600s及活跃流续期未改；不因4ms或旧4分钟无正文降级，显式取消和独立预算另行管理。
