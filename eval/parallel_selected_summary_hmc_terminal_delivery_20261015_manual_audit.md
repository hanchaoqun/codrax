# HMC253 完整人工审计：0/2

本报告区分原始采集 oracle、真实工具执行、最终模型答案、系统交付区和物理文件。两例各一次并行真实运行；无第三例追绿。构造 SQLite/操作者 checkpoint 不称实机或自然中断。没有 Mermaid 或调度/IO/根因结论，相关自然能力记 NOT_TRIGGERED，不能签 PASS；未独立验浏览器布局。

## 共同 oracle

窗口 `[10.000,10.050)` 秒。按原件与 TID 独立核验：

| 文件 / TID | 事件 / 原始帧 | 内容 |
|---|---:|---|
| captures/capture.data / 101 | 3 / 8 | 2 Alloc + 1 Free |
| captures/capture.data / 202 | 1 / 3 | 1 Alloc |
| captures/nested/capture.data / 101 | 1 / 3 | 1 Alloc |
| captures/nested/capture.data / 202 | 0 / 0 | 当前精确空查询 |

载体没有声明 size 单位；深度 `0/0/1` 不能强行串有序调用，`0/2` 必须保留缺层。线程内部键 itid 与公开 TID 分开；历史一次成功不是当前执行。独立 SQL oracle 保存在各 case 的 `operator/*.oracle.json`。

## A：恢复与原名物理包通过有限轴，整答 FAIL

正式 receipt：`verification-receipts/codrax-hmc253-A-live-v1-receipt.json`。主日志 `evidence-export/logs/codrax-20261015-233452-000-17933.log`；最终 MD `evidence-export/workspace/.codrax/output/20261015-234025.246-17933.md`，SHA `bb0b44b815095baef539ab2f1580106bc6eb7abdf11afd5e66ceaeed5345f3d3`。

1. 初始来源分类和参数仍有错误；自然读取 repaired read_file 后，resume 去掉错误额外 ID、查询去掉错误 wrapper，四个 ref-only 计划真实成功执行，结果符合 oracle。不能把这些失败调用抹去或称零重试。
2. 日志 3931/3932 的最终 request/receipt 是真实生产者记录，原目标 `capture-review` 未被另名替代。逐字节提取 JSON 的 provenance 固定源日志 SHA、行号和偏移；没有从 manifest 反造收据。独立 current saved_index 来自成功 resume 的原始返回。
3. 只读 helper 的六个物理轴 verified：正式退出、当前 fixture 执行、包与已登记集合一致、引用材料完整、包完整性、退出后真实渲染链接。四 payload 加 report 五文件逐一 SHA/大小正确；manifest SHA `69fefef38a13a96c2e1b1511b7a5bef9b95ab5601a1fe0f6367fdc58af3b7867`。默认 MD/系统附区给出的是真实原名路径，内容未冒称诊断完成。
4. **范围限制**：日志 2409–2440 另有四次查询对准备后 manifest 的直接路径，参数仍是相同两原件/两线程/同窗，载荷与原四组对应。登记入口只按原件精确路径匹配，故 current/manifest 仍为 4 attempts。内容覆盖与执行尝试覆盖不同；`current_match=true` 只证明当前登记集合，不证明全工具集合。准备产物→原件的私有来源交接仍需补齐，11.3 不销账。
5. 正文/手填表仍把未知单位 512/256/128 写成字节、把重复或缺失深度拼箭头，并漏原始帧。末尾已发布的系统组件正确，但正文保留 `pending_publication` 的旧阶段状态，用户看到冲突。把 checkpoint 路径叫源码锚点也不符合 artifact-only 请求；内部长路径仍增加噪音。原生事实齐全不抵销这些错误。
6. primary/principal 同样保留 pending，且不含终态交付链接；默认 MD 末尾的 published 组件才是当前真实交付。不能直接向主结论塞系统交付区，应以明确状态字段的归属消除重复。日志 3467–3529 已完整给出四查询、五事件、十四帧和未知单位，零预算省略；但 3573 的 advisory 解释仍含“无泄漏”，与原生事实竞争。该无泄漏判断未进入最终答案，不能冒称发生了最终泄漏误判；它仍是需要治理的上下文冲突。`checkpoint.json:1` 仅为 `{`，不能支持系统补表所写参数路径，导航与源码证据角色仍未分清。

结论：已登记集合的命名终态交付有限 PASS；完整答案 **FAIL**；全执行登记覆盖仍有缺口。未因系统附表正确给模型正文签绿。

## B：原生入口未触发，整答 FAIL

正式 receipt：`verification-receipts/codrax-hmc253-B-live-v1-receipt.json`。日志 `report-writeback/logs/codrax-20261015-233454-000-18011.log`。完整可见答案为 `report-writeback/run.out:299–390`，SHA `f254261e3b78b7651dd9395988853efc37c62eea6c3b46e7016beb238ea291bb`；前面的 thinking 不当最终答案。没有默认最终 MD 或 answer-surfaces，不能伪造。

1. 日志 41：raw hybrid 经 guard 到 operation，current_source=required、target_surface=file_artifact、side_effects=local_file_write。把采集分析和自产证据交付当独立操作；该流程不能调用原生 reader，也无结构化回交。第 3 轮已经识别需要 trace_query，仍只能继续 shell。既有恢复读取及本批 pending/terminal 路径均未触发，不能凭这例否定或验收其内部机制。
2. 共五次真实 command plan。先错用 itid=101/202 得空，再取消线程/时间限制取全局 5 行，最终把根文件 5 行全归 TID101，TID202 写 0，纳入 10.050 右边界；10.015 行实际属 TID202。后续 `d.name` 列不存在失败。不能将 SQL 执行成功当原生对象/范围已验证。
3. 正文/表又把 size 写 bytes、历史 payload 当当前调用线索、错误箭头和深度、事件数差异当采集覆盖率。承认包未完成/符号查询失败是正确的局部披露，但不能抵销错误数值。
4. 系统把五次执行加一条同 plan ID、无 StepResults 的 budget terminal 显示成六轮，模型随之复述；已定位 `command_operation_cli.go` 与 `messages.go` 的 `len(records)` 口径。真实失败执行应计数，纯状态和历史记录不应计数，不靠只数 StatusExecuted 修补。
5. 无 export 接受/包/最终报告记录。日志 123 与输出 391 明示旧报告未替换；两份报告仍为原 184 字节，SHA `0464b367df35fe5e4e0d8577e5cbb136f630d7987b8eefbb30fb2b7e71468047`。这是保原件，不是成功安全回写；上一批 B 的物理 PASS 保留为上一批证据。

结论：完整答案 **FAIL**；原生恢复、命名终态导出、报告发布 **NOT_TRIGGERED**。不提供伪 export 收据或 current index。路由工具能力不匹配是可证系统缺口，不是增加预算就能修好的模型偶发表述。

## 保留与下一修复边界

两例原采集、checkpoint、question 均按准备 SHA 复核；CLI 正常新增 `.gitignore`，不称工作树零变化。全部失败原件及两个独审报告本地归档。优先用私有来源凭证贯通执行登记；其次用明确 typed acquisition/delivery-owner 区分原生分析交付与独立操作，保真实源码义务和写权限。同源展示/状态复用与计数走结构信号，不改模型问答原文、不按关键词硬门。

这些是原 56 项内的验收欠项，不再造一批重复父任务。§250/252 完整 FAIL 与 §251 INTERRUPTED 原样保留；没有证据宣称本批问题只是模型波动。
