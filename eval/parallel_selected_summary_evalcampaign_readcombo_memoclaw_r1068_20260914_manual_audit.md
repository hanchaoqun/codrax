# r1068 人工审计：图关系未闭环与项目检查能力误铸

- date: 2026-09-14T07:23:24Z
- sweep_start_ts: 20260914-002324
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线清洁 `40c1ec82617e`；按243例库存的影响、风险、异构、最近覆盖及本机可执行性，严格并行2例、各1次。原case/oracle/答案/测试不修改，未追加第三例。机器2/2通过；人工两路均不能签完整正确：读缺要求的关系图，写补丁正确但系统验证能力被高估。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260914-002324 | log_regex,write_apply,write_patch_oracle | none | 156s | 28 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail: proof authority; patch pass | 同语言AST检查被授target_behavior，独立8格后验通过不回填产品proof |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260914-002324 | answer_regex,answer_contains | none | 423s | 39 | read=27,repo_map=2,list=0,trace=0,source_lens=1 | midloop=27,inv=6/0,fin_reject=7,unavail=0,prune=0 | fail: requested relation flow absent | 1 full+9 patch，最终仅两个孤立节点；解析成功不等于关系正确 |

## 1. 读模式：字面量正确，关系交付不完整

正式产物：`.codrax/output/20260914-003025.105-13880.md`（4545字节）及同名HTML。日志为read结果目录的`run-1.logs/codrax-20260914-002326-000-13880.log`，以下行号固定指本次原文件。

- md11/18/31：两个Name字面量正确；12/33保留full用于首次与完整重写。
- md21–25：图只有两个独立工具节点、零边，未呈现要求的finalizer流程。md27诚实声明未证关系；末尾“部分边标注不足”是宽泛残余披露，不能将零边算作有用关系图。
- 原Markdown经项目实际`internal/preview/assets/mermaid.min.js`离线Chrome parse+render通过，SVG6832字节。记录`.codrax/tmp/20260914-r1068-read-mermaid-render.json`，输入未改；仅为语法正证。过程10927–28同一repair_hash记为一个格式事件，不能算两次不同修复或关系恢复。
- md36把patch调度概括为emitSwitchToPatchSignal后由模型切换，遗漏首次局部拒稿即可preferPatchNext的通道；没有明确说“必须两次”，不能夸成它断言硬性≥2门。md34多加了Description首句比较，并把常量名称当真实字符串，属于模型事实错误。

### 过程、上下文与所有权

1. 首稿5441画抽象箭头却误用precedence且无实际操作证据，5448拒。模型5568自行删五边，5571精确合并稿暂存为下一轮base；5614处理orphan后仍因证据ID不在可引用集而拒，5700模型删ID并整块替换时自己漏掉section标题/正文，5714首次接收。系统没有删除该段。
2. 5755补表后接受，5816因要求的图零边回到explore。第一轮只读evaluator头80行和测试头40行，未供给真实schema选择路径。第二轮8459读FilterToolSchemas顶注附近50行，恰止于body起点；后来收进提示字符串写入call及guard，却未补足真正选择/执行/结果路径。WriteString旁路不能替代用户问的工具流程，不能为过门补画连接。
3. 旧`finalizer_switch_to_patch_test.go`头的≥2次特殊nudge与对应函数一致，本身不是错。真正陈旧的是FilterToolSchemas顶注把它列为通用前提、漏preferPatchNext；源码注释和有限补读助长了不完整概括。不能全归随机模型波动，也尚未证明新执行硬门自相矛盾。
4. 第二轮10924再尝试无证control_flow/call组合，关系门正确拒绝；模型随后选择删边/保独立边界。11203最后patch接受，11236零边合同仍未满足，11237已有diagram_fidelity补证预算1/1耗尽后带caveat交付。共1 full、9 patch、7拒、3接受、2次finalizer dispatch。不是畸形JSON、空答案、旧稿尽力恢复或流超时。
5. B1676自然正证只覆盖拒稿可作base、暂存事务及最终接受；无base报错、四base优先级、实际完整重写调用未自然命中。B1675其它选择兼容形及Trace旁路N/A，不能用此例销账。

## 2. 写模式：补丁正确，正式验证未执行目标方法

过程是write结果目录的两份日志（002326规划、002520应用）；formal报告`plan-1789370720642288000-13915.report.json`及`.final.json`，最终面为`run-1.out`。

1. API明确POST `/v1/search`，JSON为query/limit与可选namespace。applied `0edc77eddc`相对Python seed `beb8c1318d`只改client.py：sync/async均改POST+JSON，签名、默认10、原namespace判断、await和返回路径保留。API blob`f8bb6e89`、Makefile`7ee2ffd4`、原测试`a3b18f75`前后相同；API/TS兄弟仓未改。无新增测试/probe，未弱化原测试。
2. Make只运行`python3 tests/check_search_client.py`；其6–40行仅read_text/ast和源码检查，不调用目标方法。report7行正确是aggregate、16–28仅一条Make命令；42行却给client.py `target_behavior`，final399/405/412进一步是strong/一个行为路径/verified。这是B1678/P1能力生产者缺口，不能称原生动态正证。
3. 根源：`run_tests_changed_path_coverage.go:228–233`先授所有成功非probe命令target_behavior，262–265只在driver与target语言不同才降source_static。同语言AST检查漏过。精确路径已检查与目标已执行必须分轴；保留exact declared roster，但同语言不是执行凭证。此为旧B47-CAPCAL1假设的收窄，不是补提示、禁Make或重复已有逐合同B1561/B1575工作。
4. 三条原request约束与九条缺evidence_ref的模型文件布局约束自始至终为0硬/0软/12规划项；七条自然语言清单不是七个执行断言。模型内部曾误说12项covered，但程序未晋升required或12份proof。out140正确显示“1条验证结果”（B1673自然正证），144保留非逐项独立证明说明；这些正面不能抵销能力误铸。
5. write_analyzer一次expected_outcomes数组形状错误（规划日志861–865）被明确拒绝、模型重发；未见JSON教学矛盾或系统造内容，不为一个误发继续堆prompt。
6. 独立后验脚本`.codrax/tmp/20260913-r1068-memoclaw-native-audit.py`真实调用sync/await async，通过记录transport检查POST/URL/JSON/返回对象。两模式×默认/中文namespace/空query零limit/显式None共8格：基线8失败，applied8通过（`.codrax/tmp/20260914-r1068-memoclaw-native.json`）。只是有限输入观察，**不回填formal proof**，不宣称所有客户输入均已验证。

## 3. 排期与边界

| 优先级 | 任务 | 状态 |
|---|---|---|
| P1 | B1677 被消费工具包装的归属歧义 | 已只读定位与冻结通用方案；下一片公共RED和统一入口修复，正文/可选选择分流，禁last-wins代选。 |
| P1 | B1678 项目聚合检查执行能力来源 | 本次真实确认；需runner-owned目标执行/性质凭证，未知保未知，静态不升行为；跨语言、Make、框架统一审计，不靠命令/正文关键词判断。 |
| P2 | B1679 重试协议源码注释陈旧 | 后置仅改四处注释：reserved full、首次preferPatch与streak、base不要求先成功、nudge观察patch才停；运行逻辑和模型答案不变。 |
| 调查 | 图操作供给与主流程表达 | 仍未闭；先定位真正选择/执行路径的导航与消费，不能为本例加最少边数/术语门或系统编图。未确认新的确定性误拒者不预判为模型波动。 |

读49次、写18次模型请求均attempt1，无provider重试或超时。默认首响应600s、真实静默300s、非流600s原样；4ms/旧4m无可见正文不能触发降级。自然10分钟首响应/5分钟静默边界N/A，不以423s整例时长冒充极限正证。

两例无Trace附件，显式窗、根因旁路、两轴和自动补齐的自然回放N/A；B1674公共回归及三批联合86包全仓收据见统一文档§123.1792。原机审和原产物保留，后置修复不倒签人工全绿。
