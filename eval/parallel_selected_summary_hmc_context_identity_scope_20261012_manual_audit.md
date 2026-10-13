# §234 原件人工审计

代码`a76d7fb12fcb`，固定两例并行各一次。机器1/2，完整人工1/2；后续修复与本次原始结论分开记账。

## A. 多应用压缩SQLite清单：FAIL

原件目录：`eval/results/hmc_context_identity_scope_20261012/trace_gzip_sqlite_jank_multiapp_inventory-20261012-233733/`。日志`run-1.logs/codrax-20261012-233740-000-42899.log`，输出`.codrax/output/20261012-233905.432-42899.{md,html,root-causes.json,answer-surfaces.json}`。

1. 实际完成gzip解压校验、只读SQLite快照及完整systrace导出；不是converter stub。原SQL与导出均含6条jank marker，明确窗口内达到至少3帧的有4条、覆盖appid 27599与41002。该合成输入不冒充实机BRBE/SPE。
2. analyzer接受显式1.0–1.5秒、bounded_fact_set、`全部列成表`完整性要求，未再发出虚构declared_count。字段维度仍有非逐字英语source_quote草稿，这是归一化/教学观察，不声称均进入接受合同。
3. 日志1260行唯一trace_query添加start_ts/end_ts端点过滤，并把`start_ts >= 1000000000`等合成条件填入source_quote。1261行原生工具返回ok=false，明确没有执行、没有产生count。新出处防线真实命中；未偷偷移除过滤或转成成功。
4. 紧接着1264行记录`external-observation-sufficient`提示键，1266行提示正文依据3条已有runtime_artifact观察；这些是可定位预览，不具全量统计资格。模型未修复查询，1307行直接用两条预览发射principal scalar/member_set及resolved；1314行工具接受，1336行HasEnoughFacts又由3条外部观察升为足够。这个共享交接缺口是确定系统问题，不能称已证明的模型波动。
5. 最终正文/表格写“共2条、均appid=27599”，漏掉appid=41002的4帧（1180000000→1240000000，60ms）和7帧（1320000000→1420000000，100ms）；已列两条端点/20ms、60ms正确，不补偿总体错误。附带解释仍暗示仅漏低于3帧记录，不足以披露真实覆盖缺口；还保留“先通过trace_query…”计划话术，不能充当成功查询证据。
6. 无图、root-causes旁路为空且contract未激活，未新造因果链；这不是图/因果验收。上下文估算75150tokens/38%触发效率advisory，只记噪音成本，不单独归因失败。

剩余系统缺口挂01.3/16.4/18.3/18.4：查询失败后未完成修复与预览资格没有统一进入完成判定；不以裸观察数量或模型aggregate_facts证明全量，不以问题/答案关键词硬门补救。本次FAIL保持，确定性修复另记统一账本。

## B. 只读登记与context包恢复：PASS（限定路径）

原件目录：`eval/results/hmc_context_identity_scope_20261012/native_registration_restored_terminal/`。真实planner完整读取value.py 2行、test_value.py 4行，登记1条`assertion_ref`并绑定合同`increment-result`，源码变更/probe均0。真实断言引用为`assertion_5e00ef893cb00906797f59688e59661e1578994f5955ca0cce69ae05e72a5247`，不把合同名当断言身份。独立child PID43906、parent42589，新原生执行`native:ab82:18de036ee914abc0:1`不同于旧执行及来源核验；断言1、失败0，3批和整体verified，readonly_checked/readonly为true，HEAD/文件hash/diff/status留证，成功receipt实际落盘。

5份context快照均6包、6个批次作用域内逻辑ID、6个存储ID。child到durable的逻辑内容全部相同；4份durable快照（原JSON、Load及两次复保存）全部对象相同，各批引用2/2/2，内容hash和实际存储文件一致，无碰撞、无受管引用累积。本例没有未知旧引用或slice引用，相关行为由确定性矩阵覆盖，不冒称本次live已验。

边界：真实planner+脚本controller+独立子进程验证，不是完整CLI/模型controller或18.5永久完成；不倒签此前失败。原子产物先写、workflow后写，不是多文件事务。非阻断剩余：planner首次请求未暴露repo_map被schema拒绝；系统仍带通用repo_map/grep探索提示，应按当前工具能力消噪，挂01.3/16.4，不阻断本批持久化闭环。
