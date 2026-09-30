# HMC §202：唤醒图与转换质量人工审计

- date: 2026-09-30T09:53:25Z
- sweep_start_ts: 20260930-025323
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

冻结代码 `cba31daf5b07948ab58bfef1e0f842ef1a1ac907`，构建版本 `cba31daf5b07`。runner 核对 HEAD/构建并复制私有执行快照；结束后按既有清理逻辑移除快照，原 `codrax` 未重建。恰好 2 并行 × 1，无第三例。机器 2/2，完整人工 0/2；机器只检查声明的粗粒度 oracle，不代表内容、图或因果边通过。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_existing_sqlite_hisys_scalars | PASS | eval/results/trace_existing_sqlite_hisys_scalars-20260930-025325 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 143s | 37 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 8 行清单保留；质量信息已到场，最终仍误判缺口/计数且遗漏坏时间披露 |
| 1 | trace_query_sleep_dependencies | PASS | eval/results/trace_query_sleep_dependencies-20260930-025325 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 400s | 52 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=5,inv=2/0,fin_reject=4,unavail=0,prune=0 | FAIL | 时间明细改善；缺边错误归邻近事件，图重复跨查询实例且缺 app 入边 |

## 1. 原题与过程

两题仍为自然问题，未把时间/字段/因果/去重防护塞回 QUESTION：sleep 问 app-100 在 5.000–5.041 秒的唤醒依赖、上游 dep2 几次休眠及图表；SQLite 问 2.000–2.080 秒系统事件的域/名称/线程/内容与完整性。两题均没有客户源码读取。

SQLite 一次 attached_trace/event_search 返回 8 行，finalizer 两轮（一次成功发射、一次所求维度补标 patch）。sleep 首个 wakeup_chain 未选线程，随后两个 event_search；系统补采 app/dep2 wakeup_chain、window_stats 和 dep2 event_search，共 8 次查询。不能把系统自动补采计成模型全程正确选路，也不禁止自动补采。四次 finalizer 拒绝均是关系/修补事务问题；未触发完成降级，未跳过答案合同。

sleep 最终使用的状态明细已完整：app 的 S 40ms；dep2 四段 10/6/5/0.8ms，合计 21.8ms。第 3 段 5.022–5.027 没有对应 sched_wakeup，5.0288 的 dep5→dep2 只能解释第 4 段结束，不能借给第 3 段。最终表虽把四段端点/数值写对，却把第 3 段唤醒者填 dep5；正文再声称一个事件覆盖两段边界、app 整个休眠实质等 dep2 所有工作，超出直接唤醒关系的证明能力。完整答案 FAIL。

## 2. 图：能渲染不等于关系正确

未修改 `run-1.primary.md`，用仓内 `internal/preview/assets/mermaid.min.js` 在 headless Chrome 中 parse/render；外部 HTTP(S) 请求禁用。1 张 sequenceDiagram 语法及 SVG 生成通过（36,121 bytes），截图 `/tmp/codrax-hmc202-sleep-diagram.png` 已实际查看。

图有 **15 条生命线、6 条箭头**，而原件只有 4 条相关唤醒事件。5.012、5.020、5.0288 各被画两次；5.040 dep2→app 仍未出现在图中，app 仅剩 Note。原模型声明与自动新增声明并存，dep2 多条重复生命线。这不是旧 Note 换行语法错误；修语法不能修关系语义。

actual finalizer 的 `Available runtime diagram anchors`（原日志 3193–3201）提供前 8 条：同源同一行 5.012 重复 4 次、5.020 重复 4 次，另外 6 条被预算省略。**本轮的新 decoder 已保住末点，新的丢失原因是跨查询重复占据展示预算**，不是原来的 matched_end 右开误删。每次查询的事件凭证应继续独立，不能只删除 query 身份或按线程名合并；展示/修补候选应在不改变硬证据权限的前提下，优先覆盖不同实际事件，再处理别名和空声明。

原日志 3191 的共同教学已经声明“唤醒不是全部等待归因”，但仅加这句话不足以解决多查询候选拥挤。HMC-12.5/16.4 保持系统 P1；单查询四事件/重复双端/显式右界的公开回归不能代签此多查询生产组合。

## 3. SQLite：质量交接修复，成文仍 FAIL

actual finalizer 日志 2089 的事件库存已含 `hisys_all_event rows_read=10 rows_emitted=8`、`invalid_timestamp=2`、`invalid_source_tid=5`、`capture_state=unknown`、`counts_scope=source_table_before_query_filters` 以及“诊断计数可交叠/无时间行不可分配查询窗”。相比 §201，该信息不再被 94 项库存前 24 条裁掉；保真数据与源 TID raw 均到达最终模型。

正确范围：8 条有效时间记录从 2.000000001 到 2.070000008 秒；2 条时间非法源行不能判断是否属于所问窗口；5 个非法 TID 是字段诊断，不是额外丢掉 5 行。NULL 来源线程未知，INTEGER 0 是合法源字段但不给真实线程实体身份。缺 stat 不能证明原始采集完整。

最终保留 8 行时间/域/事件/内容，但把 0 写成非法，把 `declared_authoritative_known=9` 改述为数据库已知事件总数 `declared_known=9`，与筛选出的 8 行跨口径比较；还凭末 10ms 无事件断定存在记录缺口，未披露 2 条无合法时间源行。9 是导出物的已知语义行口径，包含非 HiSys 行，不是本查询应有的事件数。因此完整答案 FAIL，不归纯波动，也不自动写答案或扫描原文拦截。

剩余系统工作归 HMC-17.7/16.4：质量摘要按所查事件族/表和统计范围精确交接、压低与本题无关的 resolver/字段实现说明、区分转换覆盖与采集完整性。本轮只是泛化展示排序，不保证超过 24 个有数据表时每个所查表都入选。附件统一时标的 0..0 修复有公开 Prepare→TraceQuery→实际 finalizer/EmitPerfTrace 回归；此 live 没有 emit_perf_trace 调用，不把公共分支验收说成 live 命中。

## 4. 证据保存与退出边界

原运行目录、日志、主答和旧 §201 失败均不覆盖。冻结二进制 SHA 来自本批未重建的 `codrax`，不是已清理执行快照的事后读取。

| 原件 | SHA-256 |
| --- | --- |
| `codrax`，revision cba31daf5b07 | `8b0fec7d6e810a4b2cd18204b9eb46e39cafc84af46a53ca6c9408b0428cc4fe` |
| sleep `run-1.primary.md` | `0f5db6dafa2905bfc94f0d3c4f2bfed1fd8ea98025f460c39d872c5d8ea280a2` |
| sleep `run-1.logs.all.log` | `74dbff64de5ef468c9bd028fed677e9e00c33e4aa93a05b72bb729c99f80053b` |
| SQLite `run-1.primary.md` | `9ce91bf3c93e7f89693bed10d76b5cfbafd06b5db754b4d1b81cd2a16be4ab5a` |
| SQLite `run-1.logs.all.log` | `39e8ca56ddd608c239758d526640509347c7b6ff28eea24d7ffd074ad7d81f83` |
| `/tmp/codrax-hmc202-mermaid-browser.json` | `29da10080513aa806430b39358947848b4a5610e543352d845bf57d2c5656287` |
| `/tmp/codrax-hmc202-sleep-diagram.png` | `bc73a1b57aca3bbe2249d0778088debab68a9cf720a7ff27fe8fcf28a8b70bf2` |

79 稳定任务、16 实现已交付、63 开放不变。本批两组子能力不销 12.5/17.7 父项；两份完整答案继续 FAIL，5 个稳定验收父项不受本批粗粒度 PASS 影响。能力轨下一批切回只读凭证重启恢复/二进制剩余能力；缺陷轨优先解决跨查询证据展示拥挤和结构化质量统计范围，而非继续局部措辞补丁。
