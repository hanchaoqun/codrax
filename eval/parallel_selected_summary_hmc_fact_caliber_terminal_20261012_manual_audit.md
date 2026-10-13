# §232 人工审计：事实口径与恢复终态

冻结 `e93c5040619b`，固定两例并行各一次。主审读取最终正文/表格、完整运行过程和结构化产物，独立复审分别核对两例。没有第三例追绿。

## 压缩二进制卡顿清单：FAIL（取数与表格通过）

原件：`eval/results/hmc_fact_caliber_terminal_20261012/trace_gzip_sqlite_jank_inventory-20261012-183405/`；工具载荷run ID `20261012-183408-000-99425`。

唯一附件是1100字节`capture.transport`，实际完整解包为49152字节SQLite，经快照/export生成tracebundle；manifest明确原数据库未改变、未调用外部转换器，数据库和decoded SHA同为`f7fa073b…77a6`。不是后缀判别或converter stub；但仍为合成可核验SQLite，不代表其它二进制平台/实机BRBE/SPE已验。

公开`trace_query event_search`使用1..1.2窗口和`jank_frames >= 2`，结构化payload `b4ea4df3`返回两条、matched_total=emitted=2、enumeration_complete=true，保原trace时钟。参数`dbcaff3f`另加native start/end全包含条件；本fixture结果相同，不证明跨边界事件上等价，不作为已泛化通过。

最终primary/principal/transcript一致，无隐藏图或互相矛盾的附表。两条都为appid27599：2帧、1020000000→1040000000ns、20ms；4帧、1100000000→1160000000ns、60ms。完整条数、原生值、单位和表格均正确，没有把OpenDocument/UploadTexture当根因，也未虚构调度边。

仍未通过的正文与过程：

- 正文称marker内嵌字段为`start_ts_ns/end_ts_ns`，实际原marker为`start_ts/end_ts`，前者是工具规范化字段。耗时公式省略`/1e6`，虽然表内20/60ms正确。
- accepted analysis `e58df0ec`把“至少2帧”填为`declared_count=2`，完整性required=false；本例恰好两条，不能掩盖阈值与结果数量的语义混淆。原生查询完整回执未丢。
- investigation `a09afc67`成员、值和引用正确，但reason把全局event_count15当窗内15，实际observed_count8，并误称“每帧耗时”。prestage `5fce635a`给相邻业务填trigger_span，最终没有升为原因。

归01.3/16.4/18.4留账：源事实供给充足而成文误述不追加关键词门、不把一次差错未经证实就称模型波动；范围/数量交接需在typed意图与原生总体边界上另验。不以本轮替代完整尾部、REPL、sample-only和采集完整性验证。

## 只读登记恢复终态：FAIL

原件：`eval/results/hmc_fact_caliber_terminal_20261012/native_registration_restored_terminal/`；[正式失败日志原字节归档](results/hmc_fact_caliber_terminal_20261012/verification-receipts/codrax-hmc232-registration-live-v1.log)。这是实际默认planner阶段，不是完整CLI/真实controller模型验收。

模型工具顺序合法：日志276/277分别读取`value.py`和`test_value.py`；324发射`changes=[]`，以真实读取产生的`assertion_ref`登记`increment-result`，无源码/测试/配置修改请求。系统补齐suite/assertion，独立登记凭据、来源plan/commit和持久化均存在。source-verified-workflow的来源batch为`verified/tests_passed/verify_attempt`，来源真实测试执行不是夹具直接注入完成标记。

新进程完成行为证明后，helper报`proof=verified reasons=[]`，但工作流`unverified/localization_observed_without_owner`，正式exit1、没有`receipt.json`，故不得签PASS。已保留的planner-observations把对`value.py`的只读查看记为observed production anchor，workflow层据此开启新的owner定位义务；读取不是owner证明，也不是新的源码修改任务。这是确定性系统职责/交接接缝，不是模型调用错误。

失败发生在helper终态断言，后续新invocation相异性/最终文件与HEAD不变的完整断言未全部走完；父进程的final-plan/final-report/workflow可能仍是恢复前镜像，不能冒充失败子进程的完整最终快照。原日志确认失败原因，但不据此补造丢失的终态产物。后续回归应完善失败留档，不能覆盖此轮原件。

归18.5留账。补救必须隔离认证只读验证与来源源码义务，仍保来源缺终态、错误delivery/代次/注册回执、普通空changes/verify-only的否定控制；不得把读取抬成owner或普遍跳过源码义务。原始FAIL与后续确定性GREEN分开记录。

## 同批确定性补救（不是重新评测）

末版`b8f2ffabc2a4`让独立凭据、精确已验证单一来源及完整历史匹配的只读登记不产生新的源码归属义务；35格公开资格矩阵、真实工具读→登记→独立进程验证，以及原缺来源终态负控均通过。实际执行前仍重验物理HEAD/字节，读取证据不升为owner。相关/race四包PASS。

新增失败留档负控刻意令来源owner未验证，确认子进程仍失败、原生证明虽verified也不代签工作流，并能保留实际plan/report/workflow、进程身份、新invocation和只读核验；父进程原样复制。这是今后失败产物保留能力，不补造本轮live未落盘的子进程产物。不追加第三live，不改变上述两项FAIL。正式末版全仓与发布凭据统一见[账本§232](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
