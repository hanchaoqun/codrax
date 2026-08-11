# Codrax Windows 环境安装教程（小白版）

这份教程用于在另一台 Windows 电脑上下载、编译和运行 Codrax。

本文对应的功能分支是：

```text
agent/trace-root-cause-clustering
```

仓库地址：

<https://github.com/hanchaoqun/codrax>

---

## 1. 先选择你要做什么

### 方案 A：只运行 Codrax

如果别人已经把 `codrax.exe` 发给你，你只需要：

```text
codrax.exe
providers.yaml
codrax.yaml（可选）
```

这种方式不需要安装 Go、GCC、Make 或 MSYS2。

直接跳到本文的“第 10 步：配置模型”和“第 11 步：运行 Codrax”。

### 方案 B：下载源码并自己编译

如果你需要：

- 修改代码；
- 运行新功能测试；
- 自己生成 `codrax.exe`；
- 在最新功能分支上继续开发；

就需要完成下面的全部步骤。

---

## 2. 电脑需要满足什么条件

建议使用：

- Windows 10 或 Windows 11；
- 64 位系统；
- 16 GB 或更多内存；
- 至少 10 GB 可用磁盘空间；
- 能够访问 GitHub 和 Go 依赖下载服务。

建议把源码放在短的纯英文路径：

```text
C:\src\codrax
```

尽量不要使用过长路径、中文路径或带空格的路径。这样可以减少编译工具的兼容问题。

---

## 3. 安装 Git

打开 PowerShell，执行：

```powershell
winget install --id Git.Git -e --source winget
```

也可以从官方网站下载安装：

<https://git-scm.com/install/windows>

安装完成后，关闭 PowerShell，再重新打开。

检查：

```powershell
git --version
```

如果输出类似下面内容，说明安装成功：

```text
git version 2.x.x.windows.x
```

---

## 4. 安装 Go 1.25 或更高版本

项目的 `go.mod` 指定：

```text
go 1.25.0
```

执行：

```powershell
winget install --id GoLang.Go -e --source winget
```

也可以从 Go 官方网站下载 Windows MSI 安装包：

<https://go.dev/doc/install>

安装完成后，关闭 PowerShell，再重新打开。

检查：

```powershell
go version
```

版本应当是 Go 1.25.0 或更高，例如：

```text
go version go1.25.0 windows/amd64
```

---

## 5. 安装 MSYS2、GCC 和 Make

Codrax 使用了 tree-sitter。它需要 CGO，所以 Windows 编译时还需要 GCC。

### 5.1 安装 MSYS2

从官方网站下载安装：

<https://www.msys2.org/docs/installer/>

建议使用默认目录：

```text
C:\msys64
```

### 5.2 更新 MSYS2

安装后，从开始菜单打开：

```text
MSYS2 UCRT64
```

执行：

```bash
pacman -Syu
```

如果提示关闭窗口，就关闭它，再重新打开 `MSYS2 UCRT64`，继续执行：

```bash
pacman -Su
```

### 5.3 安装 GCC 和 Make

在 `MSYS2 UCRT64` 窗口执行：

```bash
pacman -S --needed mingw-w64-ucrt-x86_64-gcc make
```

如果还想运行更多测试，可以安装 Python：

```bash
pacman -S --needed python
```

---

## 6. 让 PowerShell 能找到 GCC 和 Make

重新打开普通 PowerShell，执行：

```powershell
$env:Path = "C:\msys64\ucrt64\bin;C:\msys64\usr\bin;$env:Path"
```

这条命令只对当前 PowerShell 窗口有效。

检查四个工具：

```powershell
git --version
go version
gcc --version
make --version
```

如果四条命令都有版本输出，说明基础环境已经准备好。

### 永久加入 PATH

在 Windows 中打开：

```text
设置
→ 系统
→ 系统信息
→ 高级系统设置
→ 环境变量
→ 用户变量中的 Path
```

加入：

```text
C:\msys64\ucrt64\bin
C:\msys64\usr\bin
```

保存后重新打开 PowerShell。

---

## 7. 下载 Codrax 新功能分支

在 PowerShell 中执行：

```powershell
New-Item -ItemType Directory -Force C:\src
Set-Location C:\src

git clone --branch agent/trace-root-cause-clustering --single-branch https://github.com/hanchaoqun/codrax.git
Set-Location C:\src\codrax
```

检查当前分支：

```powershell
git status -sb
```

应该看到：

```text
agent/trace-root-cause-clustering
```

检查当前提交：

```powershell
git log -1 --oneline
```

首次上传这项功能时的提交是：

```text
295d35054 add typed trace finding and exact clustering
```

如果以后分支增加了新提交，显示的提交号可能不同，这是正常的。

---

## 8. 下载 Go 依赖

确认当前目录是：

```text
C:\src\codrax
```

执行：

```powershell
go mod download
```

第一次下载可能需要几分钟。

如果下载失败，先检查网络：

```powershell
go env GOPROXY
```

公司网络环境中，应优先使用公司允许的 Go Proxy，不要随意使用不可信代理。

---

## 9. 编译 Codrax

先查看环境：

```powershell
make info
```

检查 CGO：

```powershell
go env CGO_ENABLED
go env GOOS
go env GOARCH
```

通常应当看到：

```text
1
windows
amd64
```

开始编译：

```powershell
make
```

编译成功后会生成：

```text
C:\src\codrax\codrax.exe
```

检查文件：

```powershell
Get-Item .\codrax.exe
```

检查程序能否启动：

```powershell
.\codrax.exe --help
```

### 如果提示找不到 GCC

常见错误：

```text
C compiler "gcc" not found
```

解决方法：

```powershell
$env:Path = "C:\msys64\ucrt64\bin;C:\msys64\usr\bin;$env:Path"
gcc --version
make
```

### 如果提示内存不足

常见错误包含：

```text
VirtualAlloc
errno=1455
out of memory
```

先使用低内存编译：

```powershell
make lowmem
```

仍然失败时执行：

```powershell
$env:GOMEMLIMIT = "2GiB"
make lowmem
```

还可以把 Windows 虚拟内存设置成“系统管理”，或者把页面文件设置为至少 16 GB。

---

## 10. 运行新功能测试

先运行最重要的测试：

```powershell
go test ./internal/analysis/tracefinding ./internal/analysis/tracecluster
```

成功时会看到类似：

```text
ok  github.com/hanchaoqun/codrax/internal/analysis/tracefinding
ok  github.com/hanchaoqun/codrax/internal/analysis/tracecluster
```

继续运行 Schema 测试：

```powershell
go test ./internal/tool -run TestTraceFindingSchemaIsOptIn
```

运行原子保存测试：

```powershell
go test ./internal/types -run TestFinalAnswerArtifactsAtomicRoundTrip
```

完整测试命令是：

```powershell
make test
```

注意：完整测试包含部分偏 Linux 的环境测试。Windows 上可能因为文件权限、`sh` 或 Python 环境失败。判断本次功能是否正常时，优先看上面的定向测试。

---

## 11. 配置模型

只编译和运行单元测试时，不需要 API Key。

真正使用 Codrax 分析代码时，需要 `providers.yaml`。

复制示例：

```powershell
Copy-Item .\providers.yaml.example .\providers.yaml
notepad .\providers.yaml
```

最小配置示例：

```yaml
llm:
  default:
    provider: openai
    api_key: "替换成你的 API Key"
    model: "替换成模型名称"
    base_url: "https://替换成模型服务地址/v1"
```

`providers.yaml` 应放在 `codrax.exe` 同一目录。

重要提醒：

- 不要把真实 API Key 提交到 Git；
- 不要把自己的 `providers.yaml` 发给其他人；
- 仓库中只提交 `providers.yaml.example`。

`codrax.yaml` 是可选配置。需要时执行：

```powershell
Copy-Item .\codrax.yaml.example .\codrax.yaml
```

---

## 12. 运行 Codrax

假设你要分析的项目位于：

```text
C:\work\my-project
```

可以执行：

```powershell
Set-Location C:\work\my-project
C:\src\codrax\codrax.exe
```

也可以执行单次问题：

```powershell
C:\src\codrax\codrax.exe --repo C:\work\my-project --request "帮我分析这个项目的入口和调用链"
```

Codrax 默认把启动时所在目录当作要分析的项目。

运行过程中，它会在目标项目下建立：

```text
.codrax
```

这个目录用于保存日志、缓存和运行状态。

---

## 13. 最终检查表

按顺序执行：

```powershell
git --version
go version
gcc --version
make --version

Set-Location C:\src\codrax
git status -sb
make info
make
go test ./internal/analysis/tracefinding ./internal/analysis/tracecluster
.\codrax.exe --help
```

如果这些命令都成功，说明另一台 Windows 电脑已经完成 Codrax 开发和编译环境安装。
