## 统计值与 CommandMeasurement 证据链路

统计结果为 389 个非测试 Go 文件。递归口径为 `find internal/tool -name '*.go' | grep -v '_test.go' | wc -l`，覆盖 internal/tool 整个目录树的所有子目录，排除所有文件名含 `_test.go` 的文件。

### CommandMeasurement 证据链路

当 `ExecCommand.Execute` 成功执行完上述 `find` 命令后，会调用 `execCommandMeasurement(command, output)` (`internal/tool/builtin.go:677`)，将原始 stdout 传入。该函数内部调用 `DeterministicCountProofInteger(output)` (`internal/tool/builtin.go:1458`)，该函数通过正则表达式从 `wc -l` 输出中提取确定性的整数计数。提取成功后，构造 `types.ToolCommandMeasurement{Value: 389, Kind: types.ToolCommandMeasurementKindCount, Origin: types.AnswerEvidenceOriginCommandMeasurement, ProofSource: "exec_command"}` (`internal/types/context.go:7795`)，测量结果直接写入 `ToolResult.CommandMeasurement` 字段 (`internal/tool/builtin.go:692`)，而不是经过 Summary 渲染路径。

随后在 prescan 阶段，`appendPrescanCommandMeasurementCorpus` 将 `ToolCommandMeasurement` 中的 kind/value/origin/proof_source/command/history 字段注入预填语料 (`internal/types/context.go:5996`)，使确定性计数在答案合成链路的前置阶段即可直接使用，无需解析用户可见的 Summary 文本。

### 边界说明

统计范围仅限于 `internal/tool` 目录树，不包含该目录本身以外的文件。口径通过 `grep -v '_test.go'` 排除了所有测试文件，仅统计以 `.go` 结尾的非测试源码文件。命令测量结果 `389` 由 `wc -l` 直接输出，经 `DeterministicCountProofInteger` 解析为整数值后进入答案链路，测量原点为 `command_measurement`。

`389`

**CommandMeasurement 证据链路节点**

1. **ExecCommand.Execute -> execCommandMeasurement** — 命令执行成功后，在 builtin.go:677 调用 execCommandMeasurement(command, output)，将原始 stdout 传入。 (`internal/tool/builtin.go:677` (in `ExecCommand.Execute`) — measurement := execCommandMeasurement(command, output))
2. **execCommandMeasurement -> DeterministicCountProofInteger** — execCommandMeasurement 内部在 builtin.go:1458 调用 DeterministicCountProofInteger(output)，通过正则表达式从 wc -l 输出中提取整数 389。 (`internal/tool/builtin.go:1458` (in `execCommandMeasurement`) — if value, ok := types.DeterministicCountProofInteger(output); ok {)
3. **ToolCommandMeasurement 构造** — 提取成功后，在 builtin.go:1449 构造 types.ToolCommandMeasurement{Value:389, Kind:types.ToolCommandMeasurementKindCount, Origin:types.AnswerEvidenceOriginCommandMeasurement, ProofSource:"exec_command"}。
4. **ToolResult.CommandMeasurement 赋值** — 测量结果在 builtin.go:692 直接写入 ToolResult.CommandMeasurement 字段，不经 Summary 渲染路径。
5. **appendPrescanCommandMeasurementCorpus 注入预填语料** — 在 prescan 阶段，context.go:5996 将 ToolCommandMeasurement 的 kind/value/origin/proof_source/command/history 字段注入预填语料，作为答案合成链路的前置证据。

> 统计范围仅限 internal/tool 目录树，包含所有子目录中的 .go 文件，排除 _test.go。测量结果 389 来自命令输出，不属于 repo 源码文件:line，不进入 citations[] 池。