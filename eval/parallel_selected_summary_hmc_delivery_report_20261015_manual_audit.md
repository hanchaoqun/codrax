# HMC251 固定双例人工审计

日期：2026-10-15（用户时区）。本批真实模型两例各一次并行，均由会话中断停止。运行记 **2 INTERRUPTED**；整答审计 **N/A**，不是 0/2 FAIL 或 PASS。原 §250 两份完整人工 FAIL 保留，本批不追加模型追绿。汇总结算见[摘要](parallel_selected_summary_hmc_delivery_report_20261015.md)。

## 证据边界

证据根 `eval/results/hmc_delivery_report_20261015/`。两个 `question.txt` 完全相同，SHA256 `19790d0ed0bbab55816701c35cb79e4f447ce987edbc5cb105637e00cb0016f7`。B 仅宿主增加 `--report-md <本例证据目录>/final-report.md`（在其 workspace 之外）；未把报告安全规则或工具参数塞回问题。

准备材料标明 `constructed_sqlite_operator_checkpoint_not_natural_interruption`；每例 operator 0 模型/1 历史查询，历史 4 plans 中 1 success、3 not_executed。本轮不将它们计为当前执行。正式 live 进程退出收据不存在，不以 PID 消失补造退出码。读取完整工具原件，不把日志前 2000 字节预览误判成工具截断。

## A：evidence-export

完整主日志 `evidence-export/logs/codrax-20261015-201310-000-10133.log`，658 行/165936 bytes，SHA256 `7bca0caad1f4d0610f610ea626ad13e876394c4fa813a445cd0589b5b3789fdb`；`run.out` 16 行/9444 bytes，SHA256 `07c51d9a17e75c5b48cc9bb2aafc50b42e9f24fc399ea1bd4b86bf8201339c92`。

日志原时间 20:13:10.454 首次请求至 20:14:26.327 analyzer 第 5 轮请求在途，共 6 request/5 response（含路由）。实际仅 analyze：两次 list_files（第一次 recursive=true 正确被分类阶段拒绝）、两次 emit_analysis 被拒，无 accepted analysis。read_file、resume、当前查询、export、extract/finalize、终稿/交付链接全部 NOT_TRIGGERED。

可定位过程问题：路由 `current_source=required` 及 `source_explanation` 与采集文件调用位置所求冲突；模型将“导出为 capture-review”维度声明 `role=diagram, required=true`，系统接受该声明而要求补 diagram_hint，随后参与者引用不精确又重试。用户未要求图，原路由 diagram_required=false。这是结构化维度/义务与教学待审，不是渲染库故障，更不授权扫描“导出”等原文词语强制改路由。尚无终稿，不能推断最终错误图。

## B：report-writeback

完整主日志 `report-writeback/logs/codrax-20261015-201310-000-10160.log`，1220 行/270832 bytes，SHA256 `bca89eddc07e949faa8a4288082b28284cc27f63394f5b768205b84a0d83a130`；`run.out` 54 行/11836 bytes，SHA256 `bbc2d1a43707f8b4f5093125791e3a747ac602255ccecce7baab75cd21266f43`。

20:13:10.710 首次请求至 20:14:37.133 explorer 第 4 轮请求在途，共 11 request/10 response。第三次 emit_analysis 成功进入 explore；首次两个拒绝为 bounded_fact_set 缺 fact_families、非法枚举。报告在日志 36 行明确 armed，不等于已发布。

实际完整 read_file 137 行 checkpoint，附结构化导航、历史计数、next_call 及历史成功需本轮重查边界；自然自动预读分支未触发。resume 首次混入 catalog_id 被拒，第二次去掉后成功，这是 resume 参数自然修复，不冒充查询失败重试。完整 6765-byte 结果：

`report-writeback/workspace/.codrax/blob/20261015-201310-000-10160/tool-call_function_qewnotqr4ly4_1-trace_catalog-result-c6de701e.txt`

精确当前清单：

`report-writeback/workspace/.codrax/blob/20261015-201310-000-10160/trace-catalog/catalog_60e0a3d931bce153af8e6b9434458d4bc769e250bfc470fbf88b945c12c95ba9.json`

其中 2 artifacts、4 current plans 全 not_executed、0 current attempts；历史 1 success/1 attempt 独立保存。四个 query_calls 完整在场。当前 trace_query=0、export=0，无 extract/finalize/emit_answer_document/MD/HTML/primary/principal。不可变引用复用、多分支交付、最终链接、报告原子替换全部 NOT_TRIGGERED。

过程问题与限制：

- accepted analysis 仍把运行时调用位置设 source_location，系统合成 function/method/type 的 source_inventory，探索义务仍 source required/block。归原 16.4/18.4，不因本次中断抹去，也不补推最终源码门结果。
- 模型在 resume 成功前称历史“还需 3 个”，随后系统回交当前 4 个未执行。下一模型响应缺失，不能声称最后仍漏查。
- 同 dispatch 先 read_file 成功，后 emit_evidence 的历史摘要却标 no read_file history available。保来源凭证口径调查线索；它未成为受信运行时事实，不由此放松证据门或宣称根因。
- 不连续原文“内存释放及调用位置”的 required dimension 正常被丢弃，模型重试未恢复完整所求维度。保过程风险，不伪称最终释放信息已遗漏。

## 输入、旧报告与结论

| 输入 | A SHA256 | B SHA256 |
| --- | --- | --- |
| checkpoint.json | 7436304595de5f4ef4ecc7c9e395a51b1d1496d8464c502d4c38faa8137f6a4f | b1b20a66400013607b4cd1cd7c55cf8d68ee69caf0de17cbbbb2e27827c2a298 |
| captures/capture.data | a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5 | 同 A |
| captures/nested/capture.data | f912c23433825e0f25188ffef226c296d3515a2550d6a9be56cbc87880e4f03e | 同 A |

所有值与原 preparation 相同；两 workspace 针对这三份已跟踪原件的 git diff exit 0，不以此宣称包括 .gitignore 的整个工作区 clean。B 旧报告仍 172 bytes，SHA256 `547d72acef79fc795f0fb810596716c1f20d8e0e684e842a00597eff51368e9f`，仍为 operator 的 Prior capture review。旧报告保全不等于自然发布故障注入或成功回写。

当前仅可签真实运行已启动、B 完整读取/导航/自然修复 resume 与 4 项当前恢复。终稿及物理交付未触发。完整能力新增 0、23 已交付/56 开放不变；确定性实现另计，不复用本批中断或正确附表抵销 §250 人工 FAIL。下一批优先有限自然交付验收和共享来源/交付义务接缝，不追第三例、不改旧原件或答案。
