# r1072 Go 写模式 / Rust 跨模块读模式人工审计

- date: 2026-09-15T02:13:46Z
- sweep_start_ts: 20260914-191345
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

机器结果原样保留；以下人工结论来自真实源码、工具上下文、最终答案及已交付树，不以字符串 oracle 或进程退出码代替内容审计。构建385bcfc3ebfb（2026-09-15T02:13:26Z），CAP5/PARALLEL2/TIMEOUT1200，各一次；无追跑。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260914-191346 | answer_regex | none | 124s | 41 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 主调用路径保留，但匹配能力误述；另确证整题被scalar正规化污染，见下文 |
| 1 | patch_go_typo | FAIL | eval/results/patch_go_typo-20260914-191346 | write_apply,write_patch_oracle,answer_contains | none | 156s | 28 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass（本请求交付） | 精确一行patch+真实布局probe+原生测试通过；机FAIL因末尾proof-only计划无patch，B1634c复现 |

## Rust：答案、关系与模型上下文

- 原答案：`.codrax/output/20260914-191548.901-5746.{md,html}`。outer124s、case122s，不混写。三个源码文件完整读取，repo_map一次；1次完整成文+3次patch、2次成文拒绝，无JSON畸形、超时或跨阶段重试。最大上下文81046/200000（41%）。本题图非必需，实际无Mermaid，renderer检查 `20260914-r1072-rust-mermaid.json` 为diagram_count=0，记N/A，不记渲染通过或缺图失败。
- 正确部分：main→run；run分别调用collect_files和index_file；index_file逐行动态分派is_match；walker只负责收集路径，返回后由run逐文件处理。最终4条结构化关系全部是当前证据支持的边。递归与具体实现写在正文，不把collect_files→index_file的先后顺序误作直接调用。
- 人工失败：RegexLikeMatcher被说成支持`*`/`?`通配符，源码实际只按字面`".*"`切分、在剩余字符串中顺序find各字面片段。此说法第一稿即由模型添加，工具完整读过正确算法，不能归为工具给错源码。walker过滤对所有目录项生效，正文仅称隐藏目录/target目录，范围不完整；trait应称实现而不是类继承。未证明这些自由说明错误由下述错提示直接导致，不扫描答案词句作新硬门或系统改写。
- **B1691/P1确定系统合同冲突**：原emit_analysis日志626明确call_chain/call、is_scalar_answer=false、required relation_path+function_or_purpose两维度；模型错误角色profile=[agent]被 `normalizeRoleBindingScalarShape` 升成整题scalar/role-locate（628），继而清cross_component（642）及改scalar scenario（644）。探索1225和成文2434–2442实发“单个源字面值，不要walkthrough”，而相邻2424–2429仍要求caller→callee细节。应从整题typed形状限制可选角色profile的正规化权限，不添加词扫描或新拒绝；保留真正“哪个函数/配置/类型负责”的单值自动补齐。下一批公共RED优先。
- B1690新教学在2548准确说明list/table读者标签与技术身份不同域；2579允许非图完整块修补。模型选择replace_blocks，4条关系首patch已过；第二次拒绝仅因遗漏显式删除多余summary，后续完成。未证明B1690局部add分支本轮被真实选择，不能将整例签成其所有分支生产闭环。
- 被删的LiteralMatcher→line.contains、RegexLikeMatcher→rest.find初始锚均缺身份；真实pool已提供LiteralMatcher.is_match→line.contains，类型名不等于方法主体。没有“完整正确typed边被系统拒删”的证据。模型误把同endpoint局部候选缺失当成完全无证据，保作关系表达观察项，不用当前样例硬补。
- 后继供给候选：`emit_evidence.go:4224`相同caller/callee过滤可能丢walk→walk；日志1318–1322/1994将`rest = &rest[i + t.len()..],`发布成returns（实际为分支赋值）。两项须分别追producer并公共先红后绿，尚未签确切全链原因/实现，不能与B1691混销。

## Go：真实交付与证明

- 原patch计划`plan-1789438471299304000-5751`只改main.go:25的retrun→return。最终证明计划`plan-1789438559872893000-5885`的delivery.primary_source_plan_id仍指原计划；applied提交/durable ref/worktree均为`bbe6c3b94ad9e5cb2ed7df3ea70d537452203aa2`，applied-tree字节一致。其余测试、go.mod、README与原fixtureSHA不变，未合入客户主仓。
- c1的后继`TestReturnSpelling`是真同包Go overlay执行，读取当前main.go第25行并检查有return、无retrun，正式receipt exit0/903ms；该证明只支持本例文件布局事实，不冒充greet运行行为。原TestGreet另真实运行；它是1具名测试、3输入。全流程3次项目测试+1次probe，末代report2条结果，不能把profile.probe_count=6置信记录说成6次执行。
- 必需c1未删除/降格。原虚构PTO仍missing、旧行引用仍advisory，后继同c1 probe补覆盖；最终10covered+1advisory。本例不重现旧编译wrapper假证明，不据此关闭通用逐合同probe强度边界B1561。原生runner失败修复B1122未触发，N/A。
- 独立在真实交付树运行`go test -count=1 -json ./...`通过0.699s，收据`.codrax/tmp/20260914-r1072-go-applied-posthoc.log`；未回填正式报告。outer156s、case154s。机FAIL仅因`eval/run.sh`仍用末尾proof-only计划检patch-kind，B1634c再现；case/oracle不改，原FAIL不追跑。
- 初始PTO伪测试身份、中文描述式hard-contains和提前finish尝试分别留为模型/证明表达边界，系统最终如实保留missing并补了真实证明；不因最终绿色倒称起始无问题。

## 保真及后续

`20260914-r1072-{cases-before,fixtures-before,rust-originals,receipts-originals}.sha`保留；正式case、fixture、MD/HTML、计划/报告/日志不改。600/300/600s配置在真实请求中可见，无活跃流4ms/旧4m降级。此对无Trace运行，不冒称新Trace生产回放；B1689靠已交付公共/全仓回归验收。优先B1691源头形状冲突；完整occurrence来源库存A/B、B1634c及新供给候选独立留账。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
